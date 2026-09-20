package onnx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/runtime"
)

func TestParseConfig(t *testing.T) {
	c, err := ParseConfig(json.RawMessage(`{"max_length":128,"pooling":"cls","labels":["neg","pos"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.maxLength() != 128 || c.pooling() != "cls" || !c.normalize() || len(c.Labels) != 2 {
		t.Fatalf("parsed = %+v", c)
	}
	def, _ := ParseConfig(nil)
	if def.maxLength() != 512 || def.pooling() != "mean" || !def.normalize() {
		t.Fatalf("defaults = %+v", def)
	}
	for _, raw := range []string{`{"pooling":"max"}`, `{"max_length":-5}`, `{"nope":1}`} {
		if _, err := ParseConfig(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: expected error", raw)
		}
	}
}

func writeFakeModelDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "model_quantized.onnx"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "tokenizer.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"id2label":{"0":"NEGATIVE","1":"POSITIVE"},"pad_token_id":7}`), 0o644)
	return dir
}

func TestResolveLayout(t *testing.T) {
	dir := writeFakeModelDir(t)

	l, err := resolveLayout(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(l.ModelFile) != "model_quantized.onnx" || l.Tokenizer == "" || l.PadID != 7 {
		t.Fatalf("layout = %+v", l)
	}
	if len(l.Labels) != 2 || l.Labels[0] != "NEGATIVE" || l.Labels[1] != "POSITIVE" {
		t.Fatalf("labels from config.json = %v", l.Labels)
	}

	l, err = resolveLayout(dir, Config{Labels: []string{"a", "b"}})
	if err != nil || l.Labels[0] != "a" {
		t.Fatalf("explicit labels should win: %v %v", l.Labels, err)
	}

	l, err = resolveLayout(filepath.Join(dir, "model_quantized.onnx"), Config{})
	if err != nil || l.Dir != dir || l.Tokenizer == "" {
		t.Fatalf("file path layout = %+v, %v", l, err)
	}

	if _, err := resolveLayout(filepath.Join(dir, "missing"), Config{}); err == nil {
		t.Fatal("missing path should fail")
	}
	if _, err := resolveLayout(filepath.Join(dir, "tokenizer.json"), Config{}); err == nil || !strings.Contains(err.Error(), ".onnx") {
		t.Fatalf("non-onnx file should fail: %v", err)
	}
	if _, err := resolveLayout(dir, Config{ModelFile: "other.onnx"}); err == nil {
		t.Fatal("explicit missing model_file should fail")
	}
	if _, err := resolveLayout(t.TempDir(), Config{}); err == nil {
		t.Fatal("directory without .onnx should fail")
	}
}

func TestValidate(t *testing.T) {
	rt := New(Options{}, logs.NewStore(10))
	dir := writeFakeModelDir(t)
	ctx := context.Background()

	m := models.Model{Name: "c", Runtime: models.RuntimeONNX, Task: models.TaskClassification, ModelPath: dir}
	if err := rt.Validate(ctx, m); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
	m.Task = models.TaskChat
	if err := rt.Validate(ctx, m); !errors.Is(err, runtime.ErrUnsupportedTask) {
		t.Fatalf("expected ErrUnsupportedTask, got %v", err)
	}

	os.Remove(filepath.Join(dir, "tokenizer.json"))
	m.Task = models.TaskEmbedding
	if err := rt.Validate(ctx, m); err == nil || !strings.Contains(err.Error(), "tokenizer") {
		t.Fatalf("text task without tokenizer should fail: %v", err)
	}
	m.Task = models.TaskCustom
	if err := rt.Validate(ctx, m); err != nil {
		t.Fatalf("custom task should not need a tokenizer: %v", err)
	}
}

func TestStatusWhenNotLoaded(t *testing.T) {
	rt := New(Options{}, logs.NewStore(10))
	st, err := rt.Status(context.Background(), "nope")
	if err != nil || st.State != models.StatusStopped {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if err := rt.Unload(context.Background(), "nope"); !errors.Is(err, runtime.ErrNotLoaded) {
		t.Fatalf("expected ErrNotLoaded, got %v", err)
	}
}

// TestRealClassification runs the full ONNX path when the test assets are
// available (set MODELSERVER_TEST_ONNX_LIBRARY and MODELSERVER_TEST_ONNX_CLASSIFIER).
func TestRealClassification(t *testing.T) {
	lib := os.Getenv("MODELSERVER_TEST_ONNX_LIBRARY")
	dir := os.Getenv("MODELSERVER_TEST_ONNX_CLASSIFIER")
	if lib == "" || dir == "" {
		t.Skip("ONNX test assets not configured")
	}
	rt := New(Options{Library: lib}, logs.NewStore(10))
	ctx := context.Background()
	if info := rt.Info(ctx); !info.Available {
		t.Fatalf("runtime unavailable: %s", info.Error)
	}
	m := models.Model{ID: "sst2", Name: "sst2", Runtime: models.RuntimeONNX, Task: models.TaskClassification, ModelPath: dir}
	if err := rt.Load(ctx, m); err != nil {
		t.Fatal(err)
	}
	defer rt.Shutdown(ctx)
	st, _ := rt.Status(ctx, m.ID)
	if st.State != models.StatusRunning {
		t.Fatalf("state = %s", st.State)
	}
	resp, err := rt.Infer(ctx, m, runtime.InferenceRequest{Input: "I absolutely loved this movie"})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := resp.Output.(map[string]any)
	if !ok || out["label"] != "POSITIVE" {
		t.Fatalf("output = %#v", resp.Output)
	}
	if err := rt.Unload(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Infer(ctx, m, runtime.InferenceRequest{Input: "x"}); !errors.Is(err, runtime.ErrNotLoaded) {
		t.Fatalf("infer after unload should be ErrNotLoaded, got %v", err)
	}
}
