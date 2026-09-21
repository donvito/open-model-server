package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/internal/database"
	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/registry"
	rt "github.com/donvito/modelserver/internal/runtime"
	"github.com/donvito/modelserver/internal/service"
)

// fakeRuntime is an in-memory runtime that flips models between stopped and
// running and answers predictions by echoing the input.
type fakeRuntime struct {
	name    string
	mu      sync.Mutex
	running map[string]bool
	failed  map[string]string
	failOn  string // model name whose Load fails
}

func newFakeRuntime(name string) *fakeRuntime {
	return &fakeRuntime{name: name, running: map[string]bool{}, failed: map[string]string{}}
}

func (f *fakeRuntime) Name() string { return f.name }
func (f *fakeRuntime) Info(context.Context) rt.Info {
	return rt.Info{Name: f.name, Available: true, Version: "test"}
}
func (f *fakeRuntime) Validate(_ context.Context, m models.Model) error {
	if m.Task == models.TaskReranking {
		return fmt.Errorf("%w: %s", rt.ErrUnsupportedTask, m.Task)
	}
	return nil
}
func (f *fakeRuntime) Load(_ context.Context, m models.Model) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m.Name == f.failOn {
		err := fmt.Errorf("model file does not exist: %s", m.ModelPath)
		f.failed[m.ID] = err.Error()
		return err
	}
	if f.running[m.ID] {
		return rt.ErrAlreadyLoaded
	}
	delete(f.failed, m.ID)
	f.running[m.ID] = true
	return nil
}
func (f *fakeRuntime) Unload(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, wasFailed := f.failed[id]; wasFailed {
		delete(f.failed, id)
		return nil
	}
	if !f.running[id] {
		return rt.ErrNotLoaded
	}
	delete(f.running, id)
	return nil
}
func (f *fakeRuntime) Status(_ context.Context, id string) (rt.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running[id] {
		return rt.Status{State: models.StatusRunning, PID: 42}, nil
	}
	if msg, ok := f.failed[id]; ok {
		return rt.Status{State: models.StatusFailed, Error: msg}, nil
	}
	return rt.Stopped(), nil
}
func (f *fakeRuntime) Infer(_ context.Context, m models.Model, req rt.InferenceRequest) (rt.InferenceResponse, error) {
	return rt.InferenceResponse{Output: map[string]any{"echo": req.Input, "model": m.Name}}, nil
}
func (f *fakeRuntime) Shutdown(context.Context) error { return nil }

func (f *fakeRuntime) ProxyHandler(id string) (http.Handler, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.running[id] {
		return nil, rt.ErrNotLoaded
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proxied-Model", id)
		fmt.Fprint(w, `{"object":"chat.completion","choices":[]}`)
	}), nil
}

type env struct {
	h        http.Handler
	llama    *fakeRuntime
	onnx     *fakeRuntime
	ggufPath string
	onnxDir  string
}

func newEnv(t *testing.T, apiKeys ...string) *env {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	gguf := filepath.Join(dir, "tiny.gguf")
	os.WriteFile(gguf, []byte("GGUF"), 0o644)
	onnxDir := filepath.Join(dir, "clf")
	os.MkdirAll(onnxDir, 0o755)
	os.WriteFile(filepath.Join(onnxDir, "model.onnx"), []byte("x"), 0o644)

	cfg := config.Default()
	cfg.Database.Path = filepath.Join(dir, "t.db")
	cfg.Models.Directory = dir
	cfg.Auth.APIKeys = apiKeys
	llama := newFakeRuntime(models.RuntimeLlamaCpp)
	onnx := newFakeRuntime(models.RuntimeONNX)
	svc := service.New(cfg, registry.New(db), rt.NewRegistry(llama, onnx), logs.NewStore(50))
	srv := New(svc, Options{APIKeys: apiKeys})
	return &env{h: srv.Handler(), llama: llama, onnx: onnx, ggufPath: gguf, onnxDir: onnxDir}
}

func (e *env) do(t *testing.T, method, path string, body any, headers ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 && strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		var v any
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err == nil {
			out, _ = v.(map[string]any)
		}
	}
	return rec, out
}

func errCode(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestModelLifecycleViaAPI(t *testing.T) {
	e := newEnv(t)

	rec, out := e.do(t, "POST", "/api/models", map[string]any{"model_path": e.ggufPath})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if out["runtime"] != models.RuntimeLlamaCpp || out["task"] != models.TaskChat || out["name"] != "tiny" {
		t.Fatalf("runtime/task/name should be inferred from .gguf: %v", out)
	}
	id := out["id"].(string)

	rec, out = e.do(t, "POST", "/api/models", map[string]any{"model_path": e.ggufPath, "name": "tiny"})
	if rec.Code != http.StatusConflict || errCode(out) != "name_taken" {
		t.Fatalf("duplicate name: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/api/models", map[string]any{"name": "x"})
	if rec.Code != http.StatusBadRequest || errCode(out) != "validation_error" {
		t.Fatalf("missing path: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/api/models", map[string]any{"name": "rr", "model_path": e.ggufPath, "task": models.TaskReranking})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unsupported task") {
		t.Fatalf("runtime validation should surface as 400: %d %s", rec.Code, rec.Body)
	}

	rec, _ = e.do(t, "GET", "/api/models", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"tiny"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "GET", "/api/models/tiny", nil)
	if rec.Code != 200 || out["id"] != id {
		t.Fatalf("get by name: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "GET", "/api/models/nope", nil)
	if rec.Code != http.StatusNotFound || errCode(out) != "model_not_found" {
		t.Fatalf("get missing: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "POST", "/api/models/"+id+"/load", nil)
	if rec.Code != 200 || out["status"] != models.StatusRunning {
		t.Fatalf("load: %d %s", rec.Code, rec.Body)
	}
	live := out["live"].(map[string]any)
	if live["state"] != models.StatusRunning || live["pid"].(float64) != 42 {
		t.Fatalf("live status = %v", live)
	}
	rec, out = e.do(t, "POST", "/api/models/"+id+"/load", nil)
	if rec.Code != http.StatusConflict || errCode(out) != "already_loaded" {
		t.Fatalf("double load: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "GET", "/api/models/"+id+"/status", nil)
	if rec.Code != 200 || out["live"].(map[string]any)["state"] != models.StatusRunning {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}

	rec, _ = e.do(t, "GET", "/v1/models", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Fatalf("openai models: %d %s", rec.Code, rec.Body)
	}

	rec, _ = e.do(t, "POST", "/v1/chat/completions", map[string]any{"model": "tiny", "messages": []any{}})
	if rec.Code != 200 || rec.Header().Get("X-Proxied-Model") != id {
		t.Fatalf("proxy: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/v1/chat/completions", map[string]any{"messages": []any{}})
	if rec.Code != http.StatusBadRequest || errCode(out) != "invalid_request_error" {
		t.Fatalf("proxy without model: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "POST", "/v1/models/tiny/predict", map[string]any{"input": "hello"})
	if rec.Code != 200 || out["output"].(map[string]any)["echo"] != "hello" || out["task"] != models.TaskChat {
		t.Fatalf("predict: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/v1/models/tiny/predict", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("predict without input: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "POST", "/api/models/"+id+"/restart", nil)
	if rec.Code != 200 || out["status"] != models.StatusRunning {
		t.Fatalf("restart: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "PATCH", "/api/models/"+id, map[string]any{"description": "d", "config": map[string]any{"context_length": 1024}})
	if rec.Code != 200 || out["description"] != "d" {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}

	rec, _ = e.do(t, "GET", "/api/models/"+id+"/logs?n=5", nil)
	if rec.Code != 200 {
		t.Fatalf("logs: %d %s", rec.Code, rec.Body)
	}

	rec, out = e.do(t, "POST", "/api/models/"+id+"/unload", nil)
	if rec.Code != 200 || out["status"] != models.StatusStopped {
		t.Fatalf("unload: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/v1/models/tiny/predict", map[string]any{"input": "hello"})
	if rec.Code != http.StatusConflict || errCode(out) != "model_not_loaded" {
		t.Fatalf("predict on stopped model: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "POST", "/v1/chat/completions", map[string]any{"model": "tiny"})
	if rec.Code != http.StatusConflict || errCode(out) != "model_not_loaded" {
		t.Fatalf("proxy on stopped model: %d %s", rec.Code, rec.Body)
	}

	rec, _ = e.do(t, "DELETE", "/api/models/"+id, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(e.ggufPath); err != nil {
		t.Fatal("delete must not remove the model file")
	}
	rec, _ = e.do(t, "GET", "/api/models/"+id, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", rec.Code)
	}
}

func TestLoadFailureIsReported(t *testing.T) {
	e := newEnv(t)
	e.llama.failOn = "broken"
	rec, out := e.do(t, "POST", "/api/models", map[string]any{"name": "broken", "model_path": e.ggufPath})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	id := out["id"].(string)
	rec, out = e.do(t, "POST", "/api/models/"+id+"/load", nil)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "does not exist") {
		t.Fatalf("load failure: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "GET", "/api/models/"+id, nil)
	if out["status"] != models.StatusFailed || !strings.Contains(out["live"].(map[string]any)["error"].(string), "does not exist") {
		t.Fatalf("status after failure = %s", rec.Body)
	}
	rec, out = e.do(t, "POST", "/api/models/"+id+"/unload", nil)
	if rec.Code != 200 || out["status"] != models.StatusStopped {
		t.Fatalf("unload of failed model should reset it: %d %s", rec.Code, rec.Body)
	}
}

func TestONNXPredictAndRuntimeSelection(t *testing.T) {
	e := newEnv(t)
	rec, out := e.do(t, "POST", "/api/models", map[string]any{"model_path": e.onnxDir, "task": models.TaskClassification, "name": "clf"})
	if rec.Code != http.StatusCreated || out["runtime"] != models.RuntimeONNX {
		t.Fatalf("onnx dir should infer onnx runtime: %d %s", rec.Code, rec.Body)
	}
	id := out["id"].(string)
	e.do(t, "POST", "/api/models/"+id+"/load", nil)
	rec, out = e.do(t, "POST", "/v1/models/clf/predict", map[string]any{"inputs": []string{"a", "b"}})
	if rec.Code != 200 || out["runtime"] != models.RuntimeONNX {
		t.Fatalf("predict: %d %s", rec.Code, rec.Body)
	}
	if !e.onnx.running[id] || e.llama.running[id] {
		t.Fatal("model should have been loaded by the onnx runtime")
	}
	rec, out = e.do(t, "POST", "/api/models", map[string]any{"model_path": e.onnxDir, "name": "bad", "runtime": "vllm"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown runtime: %d %s", rec.Code, rec.Body)
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t, "secret")
	rec, _ := e.do(t, "GET", "/api/system/health", nil)
	if rec.Code != 200 {
		t.Fatalf("health must be public: %d", rec.Code)
	}
	rec, out := e.do(t, "GET", "/api/models", nil)
	if rec.Code != http.StatusUnauthorized || errCode(out) != "unauthorized" {
		t.Fatalf("no key: %d %s", rec.Code, rec.Body)
	}
	rec, _ = e.do(t, "GET", "/api/models", nil, "Authorization", "Bearer wrong")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	rec, _ = e.do(t, "GET", "/api/models", nil, "Authorization", "Bearer secret")
	if rec.Code != 200 {
		t.Fatalf("bearer: %d", rec.Code)
	}
	rec, _ = e.do(t, "GET", "/v1/models", nil, "X-API-Key", "secret")
	if rec.Code != 200 {
		t.Fatalf("x-api-key: %d", rec.Code)
	}
	rec, _ = e.do(t, "GET", "/api/models/x/logs/stream?api_key=secret", nil)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("query api_key should be accepted for SSE")
	}
}

func TestRuntimeLogsAPI(t *testing.T) {
	e := newEnv(t, "secret")
	auth := []string{"Authorization", "Bearer secret"}
	path := "/api/runtimes/llamacpp/logs"
	if rec, _ := e.do(t, "GET", path, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("runtime logs must require authentication: %d", rec.Code)
	}
	if rec, out := e.do(t, "GET", path, nil, auth...); rec.Code != 200 || len(out["lines"].([]any)) != 0 {
		t.Fatalf("expected empty runtime logs: %d %s", rec.Code, rec.Body)
	}
	e.llama.failOn = "broken"
	rec, out := e.do(t, "POST", "/api/models", map[string]any{"name": "broken", "model_path": e.ggufPath}, auth...)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	e.do(t, "POST", "/api/models/"+out["id"].(string)+"/load", nil, auth...)
	rec, out = e.do(t, "GET", path+"?lines=1", nil, auth...)
	lines := out["lines"].([]any)
	if rec.Code != 200 || len(lines) != 1 || lines[0].(map[string]any)["model"] != "broken" || !strings.Contains(lines[0].(map[string]any)["text"].(string), "load failed") {
		t.Fatalf("missing attributed failure: %d %s", rec.Code, rec.Body)
	}
	if rec, out := e.do(t, "GET", "/api/runtimes/onnx/logs", nil, auth...); rec.Code != 200 || len(out["lines"].([]any)) != 0 {
		t.Fatalf("logs leaked to another runtime: %d %s", rec.Code, rec.Body)
	}
	for _, query := range []string{"0", "-1", "2001", "invalid"} {
		if rec, _ := e.do(t, "GET", path+"?lines="+query, nil, auth...); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid line count %q: %d", query, rec.Code)
		}
	}
	if rec, _ := e.do(t, "GET", "/api/runtimes/unknown/logs", nil, auth...); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown runtime: %d", rec.Code)
	}
}

func TestSystemAndHeadlessRoot(t *testing.T) {
	e := newEnv(t)
	rec, out := e.do(t, "GET", "/api/system", nil)
	if rec.Code != 200 || out["os"] == "" || out["runtimes"] == nil {
		t.Fatalf("system: %d %s", rec.Code, rec.Body)
	}
	rec, _ = e.do(t, "GET", "/api/runtimes", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), models.RuntimeONNX) {
		t.Fatalf("runtimes: %d %s", rec.Code, rec.Body)
	}
	rec, out = e.do(t, "GET", "/", nil)
	if rec.Code != 200 || out["ui"] != false {
		t.Fatalf("headless root: %d %s", rec.Code, rec.Body)
	}
	rec, _ = e.do(t, "GET", "/models", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("headless SPA route should 404: %d", rec.Code)
	}
}
