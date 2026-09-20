package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/process"
	"github.com/donvito/modelserver/internal/runtime"
)

// Options configures the runtime as a whole (not per model).
type Options struct {
	Binary         string
	StartupTimeout time.Duration
	// HealthInterval is how often /health is polled while starting.
	HealthInterval time.Duration
	// StopTimeout bounds graceful termination before SIGKILL.
	StopTimeout time.Duration
	// HealthClient can be swapped in tests.
	HealthClient *http.Client
	// LookPath resolves the binary; defaults to exec.LookPath.
	LookPath func(string) (string, error)
}

type Runtime struct {
	opts   Options
	runner process.Runner
	ports  *process.PortAllocator
	logs   *logs.Store

	mu        sync.Mutex
	instances map[string]*instance

	versionOnce sync.Once
	version     string
	versionErr  error
}

const errorTail = 12

type instance struct {
	mu        sync.Mutex
	model     models.Model
	state     string
	err       string
	pid       int
	host      string
	port      int
	startedAt time.Time
	handle    process.Handle
	proxy     *httputil.ReverseProxy
	base      *url.URL
	recent    []string // recent output lines, used to explain failures
}

func New(opts Options, runner process.Runner, ports *process.PortAllocator, logStore *logs.Store) *Runtime {
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = 10 * time.Minute
	}
	if opts.HealthInterval <= 0 {
		opts.HealthInterval = 250 * time.Millisecond
	}
	if opts.StopTimeout <= 0 {
		opts.StopTimeout = 10 * time.Second
	}
	if opts.HealthClient == nil {
		opts.HealthClient = &http.Client{Timeout: 2 * time.Second}
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if runner == nil {
		runner = process.ExecRunner{}
	}
	return &Runtime{opts: opts, runner: runner, ports: ports, logs: logStore, instances: map[string]*instance{}}
}

func (r *Runtime) Name() string { return models.RuntimeLlamaCpp }

// BinaryPath resolves the configured llama-server binary. A value that looks
// like a path is resolved on disk; a bare name is looked up in PATH and then
// next to the server executable, which is where a Windows install typically
// keeps llama-server.exe.
func (r *Runtime) BinaryPath() (string, error) {
	b := r.opts.Binary
	if b == "" {
		b = "llama-server"
	}
	if isPathLike(b) {
		abs, err := filepath.Abs(b)
		if err != nil {
			return "", err
		}
		resolved, ok := executableAt(abs)
		if !ok {
			return "", fmt.Errorf("llama-server binary not found at %s", abs)
		}
		return resolved, nil
	}
	if p, err := r.opts.LookPath(b); err == nil {
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		if p, ok := executableAt(filepath.Join(filepath.Dir(exe), b)); ok {
			return p, nil
		}
	}
	return "", fmt.Errorf("llama-server binary %q not found in PATH (set llamacpp.binary or MODELSERVER_LLAMA_CPP_BINARY)", b)
}

// isPathLike reports whether s names a location rather than a command to look
// up in PATH. Windows accepts either separator, and a leading volume ("C:bin")
// is a path too.
func isPathLike(s string) bool {
	return strings.ContainsAny(s, `/\`) || filepath.VolumeName(s) != ""
}

// executableAt returns the file at p, trying the platform's executable
// extensions so a configured "llama-server" also finds "llama-server.exe".
func executableAt(p string) (string, bool) {
	for _, cand := range append([]string{p}, withExecExtensions(p)...) {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, true
		}
	}
	return "", false
}

func (r *Runtime) Info(ctx context.Context) runtime.Info {
	info := runtime.Info{Name: r.Name(), Details: map[string]any{}}
	path, err := r.BinaryPath()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Available = true
	info.Details["binary"] = path
	if v, err := r.Version(ctx); err == nil {
		info.Version = v
	}
	return info
}

// Version runs `llama-server --version` once and caches the result.
func (r *Runtime) Version(ctx context.Context) (string, error) {
	r.versionOnce.Do(func() {
		path, err := r.BinaryPath()
		if err != nil {
			r.versionErr = err
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, path, "--version").CombinedOutput()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "version:") {
				r.version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
				return
			}
		}
		r.version = strings.TrimSpace(string(out))
	})
	return r.version, r.versionErr
}

func (r *Runtime) Validate(ctx context.Context, m models.Model) error {
	if _, err := ParseConfig(m.Config); err != nil {
		return err
	}
	st, err := os.Stat(m.ModelPath)
	if err != nil {
		return fmt.Errorf("model file does not exist: %s", m.ModelPath)
	}
	if st.IsDir() {
		return fmt.Errorf("model path %s is a directory; llama.cpp needs a .gguf file", m.ModelPath)
	}
	switch m.Task {
	case models.TaskChat, models.TaskCompletion, models.TaskEmbedding, models.TaskReranking, models.TaskCustom:
	default:
		return fmt.Errorf("%w: %s", runtime.ErrUnsupportedTask, m.Task)
	}
	return nil
}

func (r *Runtime) get(id string) (*instance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.instances[id]
	return in, ok
}

func (r *Runtime) Load(ctx context.Context, m models.Model) error {
	if err := r.Validate(ctx, m); err != nil {
		return err
	}
	binary, err := r.BinaryPath()
	if err != nil {
		return err
	}
	cfg, _ := ParseConfig(m.Config)
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}

	r.mu.Lock()
	if existing, ok := r.instances[m.ID]; ok {
		existing.mu.Lock()
		live := existing.state == models.StatusRunning || existing.state == models.StatusStarting
		existing.mu.Unlock()
		if live {
			r.mu.Unlock()
			return runtime.ErrAlreadyLoaded
		}
	}
	in := &instance{model: m, state: models.StatusStarting, host: host}
	r.instances[m.ID] = in
	r.mu.Unlock()

	log := r.logs.Get(m.ID)
	fail := func(err error) error {
		in.mu.Lock()
		in.state = models.StatusFailed
		in.err = err.Error()
		if in.port != 0 {
			r.ports.Release(in.port)
			in.port = 0
		}
		in.mu.Unlock()
		log.Systemf("failed: %v", err)
		return err
	}

	var port int
	if cfg.Port != 0 {
		if err := r.ports.Reserve(host, cfg.Port); err != nil {
			return fail(fmt.Errorf("port cannot be allocated: %w", err))
		}
		port = cfg.Port
	} else {
		port, err = r.ports.Allocate(host)
		if err != nil {
			return fail(fmt.Errorf("port cannot be allocated: %w", err))
		}
	}
	in.mu.Lock()
	in.port = port
	in.base = &url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprint(port))}
	in.mu.Unlock()

	args := BuildArgs(m, cfg, host, port)
	log.Systemf("starting: %s %s", binary, strings.Join(args, " "))
	handle, err := r.runner.Start(ctx, process.Spec{
		Path: binary,
		Args: args,
		OnLine: func(source, line string) {
			log.Append(source, line)
			in.mu.Lock()
			in.recent = append(in.recent, line)
			if len(in.recent) > errorTail {
				in.recent = in.recent[1:]
			}
			in.mu.Unlock()
		},
	})
	if err != nil {
		return fail(fmt.Errorf("could not start llama-server: %w", err))
	}
	in.mu.Lock()
	in.handle = handle
	in.pid = handle.PID()
	in.startedAt = time.Now()
	in.mu.Unlock()
	log.Systemf("llama-server started (pid %d) on %s:%d", handle.PID(), host, port)

	go r.monitor(m.ID, in, handle)

	if err := r.waitHealthy(ctx, in, handle); err != nil {
		in.mu.Lock()
		alreadyFailed := in.state == models.StatusFailed
		in.mu.Unlock()
		if !alreadyFailed {
			_ = handle.Terminate(r.opts.StopTimeout)
			return fail(err)
		}
		in.mu.Lock()
		msg := in.err
		in.mu.Unlock()
		return errors.New(msg)
	}

	in.mu.Lock()
	in.state = models.StatusRunning
	in.err = ""
	in.mu.Unlock()
	log.Systemf("model %s is running", m.Name)
	return nil
}

func (r *Runtime) waitHealthy(ctx context.Context, in *instance, handle process.Handle) error {
	deadline := time.NewTimer(r.opts.StartupTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(r.opts.HealthInterval)
	defer tick.Stop()
	in.mu.Lock()
	base := in.base
	in.mu.Unlock()
	healthURL := base.ResolveReference(&url.URL{Path: "/health"}).String()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-handle.Done():
			// monitor() records the failure reason; give it a moment to run.
			time.Sleep(10 * time.Millisecond)
			in.mu.Lock()
			msg := in.err
			in.mu.Unlock()
			if msg == "" {
				msg = "llama-server exited during startup"
			}
			return errors.New(msg)
		case <-deadline.C:
			return fmt.Errorf("model failed to load: llama-server did not become healthy within %s", r.opts.StartupTimeout)
		case <-tick.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
			resp, err := r.opts.HealthClient.Do(req)
			if err != nil {
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

// monitor watches for process exit and records a failure unless we asked it to stop.
func (r *Runtime) monitor(id string, in *instance, handle process.Handle) {
	<-handle.Done()
	exitErr := handle.Err()
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.handle != handle {
		return
	}
	if in.port != 0 {
		r.ports.Release(in.port)
	}
	log := r.logs.Get(id)
	if in.state == models.StatusStopping || in.state == models.StatusStopped {
		in.state = models.StatusStopped
		in.err = ""
		log.Systemf("llama-server stopped")
		return
	}
	reason := "process exited unexpectedly: " + process.ExitDescription(exitErr)
	if hint := lastError(in.recent); hint != "" {
		reason += " — " + hint
	}
	in.state = models.StatusFailed
	in.err = reason
	log.Systemf("failed: %s", reason)
}

// lastError picks the most informative recent line from llama-server output.
func lastError(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		low := strings.ToLower(l)
		if strings.Contains(low, "error") || strings.Contains(low, "failed") || strings.Contains(low, "invalid") || strings.Contains(low, "unable") {
			return l
		}
	}
	if len(lines) > 0 {
		return strings.TrimSpace(lines[len(lines)-1])
	}
	return ""
}

func (r *Runtime) Unload(ctx context.Context, id string) error {
	in, ok := r.get(id)
	if !ok {
		return runtime.ErrNotLoaded
	}
	in.mu.Lock()
	handle := in.handle
	if in.state == models.StatusStopped || in.state == models.StatusFailed || handle == nil {
		in.state = models.StatusStopped
		in.mu.Unlock()
		r.mu.Lock()
		delete(r.instances, id)
		r.mu.Unlock()
		return nil
	}
	in.state = models.StatusStopping
	in.mu.Unlock()
	r.logs.Get(id).Systemf("stopping llama-server (pid %d)", handle.PID())
	if err := handle.Terminate(r.opts.StopTimeout); err != nil {
		return fmt.Errorf("stop llama-server: %w", err)
	}
	r.mu.Lock()
	delete(r.instances, id)
	r.mu.Unlock()
	return nil
}

func (r *Runtime) Status(ctx context.Context, id string) (runtime.Status, error) {
	in, ok := r.get(id)
	if !ok {
		return runtime.Stopped(), nil
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	st := runtime.Status{State: in.state, Error: in.err, Details: map[string]any{}}
	if in.state == models.StatusRunning || in.state == models.StatusStarting || in.state == models.StatusStopping {
		st.PID = in.pid
		st.Port = in.port
		t := in.startedAt
		st.StartedAt = &t
		st.Details["endpoint"] = in.base.String()
	}
	if cfg, err := ParseConfig(in.model.Config); err == nil && cfg.ContextLength > 0 {
		st.Details["context_length"] = cfg.ContextLength
	}
	return st, nil
}

// ProxyHandler returns a reverse proxy to the model's llama-server.
func (r *Runtime) ProxyHandler(id string) (http.Handler, error) {
	in, ok := r.get(id)
	if !ok {
		return nil, runtime.ErrNotLoaded
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.state != models.StatusRunning {
		return nil, fmt.Errorf("%w: model is %s", runtime.ErrNotLoaded, in.state)
	}
	if in.proxy == nil {
		p := httputil.NewSingleHostReverseProxy(in.base)
		p.FlushInterval = -1 // stream SSE tokens immediately
		p.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "llama-server unreachable: " + err.Error(), "type": "upstream_error"}})
		}
		in.proxy = p
	}
	return in.proxy, nil
}

// Infer implements the generic prediction API by translating to llama-server's
// OpenAI-compatible endpoints.
func (r *Runtime) Infer(ctx context.Context, m models.Model, req runtime.InferenceRequest) (runtime.InferenceResponse, error) {
	in, ok := r.get(m.ID)
	if !ok {
		return runtime.InferenceResponse{}, runtime.ErrNotLoaded
	}
	in.mu.Lock()
	base := in.base
	state := in.state
	in.mu.Unlock()
	if state != models.StatusRunning {
		return runtime.InferenceResponse{}, fmt.Errorf("%w: model is %s", runtime.ErrNotLoaded, state)
	}

	body := map[string]any{"model": m.Name}
	for k, v := range req.Params {
		body[k] = v
	}
	var path string
	switch m.Task {
	case models.TaskEmbedding:
		path = "/v1/embeddings"
		body["input"] = req.Input
	case models.TaskChat:
		path = "/v1/chat/completions"
		switch v := req.Input.(type) {
		case string:
			body["messages"] = []map[string]any{{"role": "user", "content": v}}
		case []any:
			body["messages"] = v
		default:
			return runtime.InferenceResponse{}, fmt.Errorf("%w: chat input must be a string or a list of messages", models.ErrValidation)
		}
	default:
		path = "/v1/completions"
		body["prompt"] = req.Input
	}
	delete(body, "stream")

	payload, _ := json.Marshal(body)
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String()+path, bytes.NewReader(payload))
	if err != nil {
		return runtime.InferenceResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return runtime.InferenceResponse{}, fmt.Errorf("llama-server request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	latency := time.Since(start)
	if resp.StatusCode >= 400 {
		return runtime.InferenceResponse{}, fmt.Errorf("llama-server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return runtime.InferenceResponse{}, fmt.Errorf("decode llama-server response: %w", err)
	}
	out := runtime.InferenceResponse{Timing: runtime.Timing{LatencyMS: float64(latency.Microseconds()) / 1000}}
	if timings, ok := parsed["timings"].(map[string]any); ok {
		if tps, ok := timings["predicted_per_second"].(float64); ok {
			out.Timing.TokensPerSecond = tps
		}
		if n, ok := timings["predicted_n"].(float64); ok {
			out.Timing.Tokens = int(n)
		}
	}
	if usage, ok := parsed["usage"].(map[string]any); ok && out.Timing.Tokens == 0 {
		if n, ok := usage["completion_tokens"].(float64); ok {
			out.Timing.Tokens = int(n)
		}
	}
	switch m.Task {
	case models.TaskEmbedding:
		var vectors []any
		if data, ok := parsed["data"].([]any); ok {
			for _, d := range data {
				if dm, ok := d.(map[string]any); ok {
					vectors = append(vectors, dm["embedding"])
				}
			}
		}
		if _, single := req.Input.(string); single && len(vectors) == 1 {
			out.Output = map[string]any{"embedding": vectors[0]}
		} else {
			out.Output = map[string]any{"embeddings": vectors}
		}
	case models.TaskChat:
		text := ""
		if choices, ok := parsed["choices"].([]any); ok && len(choices) > 0 {
			if c, ok := choices[0].(map[string]any); ok {
				if msg, ok := c["message"].(map[string]any); ok {
					text, _ = msg["content"].(string)
				}
			}
		}
		out.Output = map[string]any{"text": text, "message": map[string]any{"role": "assistant", "content": text}}
	default:
		text := ""
		if choices, ok := parsed["choices"].([]any); ok && len(choices) > 0 {
			if c, ok := choices[0].(map[string]any); ok {
				text, _ = c["text"].(string)
			}
		}
		out.Output = map[string]any{"text": text}
	}
	return out, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	ids := make([]string, 0, len(r.instances))
	for id := range r.instances {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_ = r.Unload(ctx, id)
		}(id)
	}
	wg.Wait()
	return nil
}
