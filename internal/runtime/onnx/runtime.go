package onnx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/runtime"
)

// Options configures the runtime as a whole.
type Options struct {
	// Library is the path to the ONNX Runtime shared library. When empty,
	// well-known locations are searched.
	Library string
}

type Runtime struct {
	opts Options
	logs *logs.Store

	initOnce sync.Once
	initErr  error
	libPath  string

	mu       sync.Mutex
	sessions map[string]*session
}

func New(opts Options, logStore *logs.Store) *Runtime {
	return &Runtime{opts: opts, logs: logStore, sessions: map[string]*session{}}
}

func (r *Runtime) Name() string { return models.RuntimeONNX }

// LibraryPath returns the ONNX Runtime shared library that will be loaded.
func (r *Runtime) LibraryPath() (string, error) {
	if r.opts.Library != "" {
		if _, err := os.Stat(r.opts.Library); err != nil {
			return "", fmt.Errorf("onnx runtime library not found at %s", r.opts.Library)
		}
		return r.opts.Library, nil
	}
	if p := os.Getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	name := libraryFileName()
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "lib"))
	}
	dirs = append(dirs, ".", "lib", "/usr/local/lib", "/usr/lib", "/opt/onnxruntime/lib", "/opt/homebrew/lib")
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("ONNX Runtime shared library (%s) not found; set onnx.library or MODELSERVER_ONNX_LIBRARY", name)
}

func libraryFileName() string {
	switch goruntime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "windows":
		return "onnxruntime.dll"
	}
	return "libonnxruntime.so"
}

// ensureInit loads the shared library once per process.
func (r *Runtime) ensureInit() error {
	r.initOnce.Do(func() {
		p, err := r.LibraryPath()
		if err != nil {
			r.initErr = fmt.Errorf("%w: %v", runtime.ErrUnavailable, err)
			return
		}
		r.libPath = p
		if !ort.IsInitialized() {
			ort.SetSharedLibraryPath(p)
			if err := ort.InitializeEnvironment(); err != nil {
				r.initErr = fmt.Errorf("%w: initialize ONNX Runtime from %s: %v", runtime.ErrUnavailable, p, err)
				return
			}
		}
	})
	return r.initErr
}

func (r *Runtime) Info(ctx context.Context) runtime.Info {
	info := runtime.Info{Name: r.Name(), Details: map[string]any{}}
	if err := r.ensureInit(); err != nil {
		info.Error = errors.Unwrap(err).Error()
		if info.Error == "" {
			info.Error = err.Error()
		}
		return info
	}
	info.Available = true
	info.Version = ort.GetVersion()
	info.Details["library"] = r.libPath
	return info
}

func (r *Runtime) Validate(ctx context.Context, m models.Model) error {
	cfg, err := ParseConfig(m.Config)
	if err != nil {
		return err
	}
	switch m.Task {
	case models.TaskClassification, models.TaskEmbedding, models.TaskCustom:
	default:
		return fmt.Errorf("%w: %s (onnx supports classification, embedding, custom)", runtime.ErrUnsupportedTask, m.Task)
	}
	l, err := resolveLayout(m.ModelPath, cfg)
	if err != nil {
		return err
	}
	if m.Task != models.TaskCustom && l.Tokenizer == "" {
		return fmt.Errorf("task %s requires a tokenizer.json next to the model (or config.tokenizer_file)", m.Task)
	}
	return nil
}

func (r *Runtime) Load(ctx context.Context, m models.Model) error {
	if err := r.Validate(ctx, m); err != nil {
		return err
	}
	if err := r.ensureInit(); err != nil {
		return err
	}
	r.mu.Lock()
	if existing, ok := r.sessions[m.ID]; ok && existing.state() == models.StatusRunning {
		r.mu.Unlock()
		return runtime.ErrAlreadyLoaded
	}
	placeholder := &session{model: m}
	placeholder.setState(models.StatusStarting, "")
	r.sessions[m.ID] = placeholder
	r.mu.Unlock()

	log := r.logs.Get(m.ID)
	cfg, _ := ParseConfig(m.Config)
	l, _ := resolveLayout(m.ModelPath, cfg)
	log.Systemf("loading %s with ONNX Runtime %s", l.ModelFile, ort.GetVersion())
	start := time.Now()
	s, err := newSession(m, cfg, l)
	if err != nil {
		placeholder.setState(models.StatusFailed, err.Error())
		log.Systemf("failed: %v", err)
		return err
	}
	s.startedAt = time.Now()
	s.setState(models.StatusRunning, "")
	r.mu.Lock()
	r.sessions[m.ID] = s
	r.mu.Unlock()
	log.Systemf("model %s is running (loaded in %s; inputs=%v outputs=%v)", m.Name, time.Since(start).Round(time.Millisecond), s.inputNames, s.outputNames)
	return nil
}

func (r *Runtime) Unload(ctx context.Context, id string) error {
	r.mu.Lock()
	s, ok := r.sessions[id]
	delete(r.sessions, id)
	r.mu.Unlock()
	if !ok {
		return runtime.ErrNotLoaded
	}
	s.close()
	r.logs.Get(id).Systemf("session released")
	return nil
}

func (r *Runtime) Status(ctx context.Context, id string) (runtime.Status, error) {
	r.mu.Lock()
	s, ok := r.sessions[id]
	r.mu.Unlock()
	if !ok {
		return runtime.Stopped(), nil
	}
	return s.status(), nil
}

func (r *Runtime) Infer(ctx context.Context, m models.Model, req runtime.InferenceRequest) (runtime.InferenceResponse, error) {
	r.mu.Lock()
	s, ok := r.sessions[m.ID]
	r.mu.Unlock()
	if !ok || s.state() != models.StatusRunning {
		return runtime.InferenceResponse{}, runtime.ErrNotLoaded
	}
	return s.infer(ctx, req)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	ss := r.sessions
	r.sessions = map[string]*session{}
	r.mu.Unlock()
	for _, s := range ss {
		s.close()
	}
	return nil
}
