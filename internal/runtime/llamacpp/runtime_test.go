package llamacpp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/process"
	"github.com/donvito/modelserver/internal/runtime"
)

// fakeHandle simulates a llama-server child: it serves /health on the port
// requested in the command line and exits when terminated or crashed.
type fakeHandle struct {
	pid  int
	done chan struct{}
	once sync.Once
	err  error
	srv  *http.Server
}

func (h *fakeHandle) PID() int              { return h.pid }
func (h *fakeHandle) Done() <-chan struct{} { return h.done }
func (h *fakeHandle) Err() error            { return h.err }
func (h *fakeHandle) Terminate(time.Duration) error {
	h.exit(nil)
	return nil
}
func (h *fakeHandle) exit(err error) {
	h.once.Do(func() {
		h.err = err
		if h.srv != nil {
			h.srv.Close()
		}
		close(h.done)
	})
}

type fakeRunner struct {
	mu      sync.Mutex
	started []*fakeHandle
	// crashWith, when non-empty, makes the child print this line and exit 1
	// instead of becoming healthy.
	crashWith string
	specs     []process.Spec
}

func (r *fakeRunner) Start(ctx context.Context, spec process.Spec) (process.Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs = append(r.specs, spec)
	h := &fakeHandle{pid: 1000 + len(r.started), done: make(chan struct{})}
	r.started = append(r.started, h)
	if r.crashWith != "" {
		go func() {
			spec.OnLine("stderr", r.crashWith)
			h.exit(errors.New("exit status 1"))
		}()
		return h, nil
	}
	var port string
	for i, a := range spec.Args {
		if a == "--port" && i+1 < len(spec.Args) {
			port = spec.Args[i+1]
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"object":"list","data":[]}`) })
	h.srv = &http.Server{Handler: mux}
	go h.srv.Serve(ln)
	spec.OnLine("stderr", "llama_server: model loaded")
	return h, nil
}

func newTestRuntime(t *testing.T, runner *fakeRunner) (*Runtime, models.Model) {
	t.Helper()
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "tiny.gguf")
	os.WriteFile(modelPath, []byte("GGUF"), 0o644)
	bin := filepath.Join(dir, "llama-server")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)

	ports := process.NewPortAllocator(31000, 31020)
	rt := New(Options{
		Binary:         bin,
		StartupTimeout: 5 * time.Second,
		HealthInterval: 20 * time.Millisecond,
		StopTimeout:    time.Second,
	}, runner, ports, logs.NewStore(100))
	m := models.Model{ID: "m1", Name: "tiny", Runtime: models.RuntimeLlamaCpp, Task: models.TaskChat, ModelPath: modelPath}
	return rt, m
}

func TestRuntimeLifecycle(t *testing.T) {
	runner := &fakeRunner{}
	rt, m := newTestRuntime(t, runner)
	ctx := context.Background()

	st, _ := rt.Status(ctx, m.ID)
	if st.State != models.StatusStopped {
		t.Fatalf("initial state = %s", st.State)
	}

	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("load: %v", err)
	}
	st, _ = rt.Status(ctx, m.ID)
	if st.State != models.StatusRunning || st.Port < 31000 || st.PID == 0 {
		t.Fatalf("status after load = %+v", st)
	}
	if !strings.Contains(strings.Join(runner.specs[0].Args, " "), "--alias tiny") {
		t.Fatalf("unexpected args: %v", runner.specs[0].Args)
	}
	if err := rt.Load(ctx, m); !errors.Is(err, runtime.ErrAlreadyLoaded) {
		t.Fatalf("second load should be ErrAlreadyLoaded, got %v", err)
	}

	h, err := rt.ProxyHandler(m.ID)
	if err != nil || h == nil {
		t.Fatalf("proxy handler: %v", err)
	}

	if err := rt.Unload(ctx, m.ID); err != nil {
		t.Fatalf("unload: %v", err)
	}
	st, _ = rt.Status(ctx, m.ID)
	if st.State != models.StatusStopped {
		t.Fatalf("state after unload = %s", st.State)
	}
	if err := rt.Unload(ctx, m.ID); !errors.Is(err, runtime.ErrNotLoaded) {
		t.Fatalf("unload when stopped should be ErrNotLoaded, got %v", err)
	}
	// Port is returned to the pool and the model can be reloaded.
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("reload: %v", err)
	}
	rt.Shutdown(ctx)
	st, _ = rt.Status(ctx, m.ID)
	if st.State != models.StatusStopped {
		t.Fatalf("state after shutdown = %s", st.State)
	}
}

func TestRuntimeCrashDuringStartup(t *testing.T) {
	runner := &fakeRunner{crashWith: "error: invalid argument: --bogus"}
	rt, m := newTestRuntime(t, runner)
	err := rt.Load(context.Background(), m)
	if err == nil || !strings.Contains(err.Error(), "--bogus") {
		t.Fatalf("expected error mentioning the llama-server output, got %v", err)
	}
	st, _ := rt.Status(context.Background(), m.ID)
	if st.State != models.StatusFailed || !strings.Contains(st.Error, "exited unexpectedly") {
		t.Fatalf("status = %+v", st)
	}
	if len(runner.started) != 1 {
		t.Fatalf("crash must not trigger a restart loop; started %d processes", len(runner.started))
	}
}

func TestRuntimeCrashWhileRunning(t *testing.T) {
	runner := &fakeRunner{}
	rt, m := newTestRuntime(t, runner)
	ctx := context.Background()
	if err := rt.Load(ctx, m); err != nil {
		t.Fatal(err)
	}
	runner.started[0].exit(errors.New("signal: killed"))
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, _ := rt.Status(ctx, m.ID)
		if st.State == models.StatusFailed {
			if !strings.Contains(st.Error, "exited unexpectedly") {
				t.Fatalf("error = %q", st.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("state never became failed: %s", st.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(runner.started) != 1 {
		t.Fatal("no automatic restart expected")
	}
	// Reload after failure works and reuses the released port.
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("reload after crash: %v", err)
	}
	rt.Shutdown(ctx)
}

func TestRuntimeValidateAndMissingBinary(t *testing.T) {
	rt, m := newTestRuntime(t, &fakeRunner{})
	ctx := context.Background()

	missing := m
	missing.ModelPath = filepath.Join(t.TempDir(), "nope.gguf")
	if err := rt.Validate(ctx, missing); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
	wrongTask := m
	wrongTask.Task = models.TaskClassification
	if err := rt.Validate(ctx, wrongTask); !errors.Is(err, runtime.ErrUnsupportedTask) {
		t.Fatalf("expected ErrUnsupportedTask, got %v", err)
	}

	nobin := New(Options{Binary: "llama-server-that-does-not-exist", LookPath: func(string) (string, error) {
		return "", errors.New("not found")
	}}, &fakeRunner{}, process.NewPortAllocator(31000, 31001), logs.NewStore(10))
	info := nobin.Info(ctx)
	if info.Available || info.Error == "" {
		t.Fatalf("info = %+v", info)
	}
	if err := nobin.Load(ctx, m); err == nil {
		t.Fatal("load without binary should fail")
	}
}
