// Package service is the application layer shared by the HTTP API and CLI.
// It coordinates the persistent registry with the in-memory runtimes.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime" //nolint:depguard // Go stdlib runtime for host info
	"strings"
	"sync"
	"time"

	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/models"
	"github.com/donvito/modelserver/internal/registry"
	rt "github.com/donvito/modelserver/internal/runtime"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	ErrModelBusy      = errors.New("model is currently starting or stopping")
	ErrTaskNotServing = errors.New("model task does not support this endpoint")
)

type Service struct {
	cfg      config.Config
	registry *registry.Registry
	runtimes *rt.Registry
	logs     *logs.Store
	started  time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per-model lifecycle lock
}

func New(cfg config.Config, reg *registry.Registry, runtimes *rt.Registry, logStore *logs.Store) *Service {
	return &Service{cfg: cfg, registry: reg, runtimes: runtimes, logs: logStore, started: time.Now(), locks: map[string]*sync.Mutex{}}
}

func (s *Service) Config() config.Config { return s.cfg }
func (s *Service) Logs() *logs.Store    { return s.logs }
func (s *Service) Runtimes() *rt.Registry {
	return s.runtimes
}

func (s *Service) lock(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[id]
	if !ok {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	return l
}

// ModelView is a model joined with its live runtime status.
type ModelView struct {
	models.Model
	Live rt.Status `json:"live"`
}

// CreateInput is the shape accepted by POST /api/models and `models add`.
type CreateInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Runtime     string          `json:"runtime"`
	Task        string          `json:"task"`
	ModelPath   string          `json:"model_path"`
	Config      json.RawMessage `json:"config"`
}

// UpdateInput is a partial update; nil fields are left unchanged.
type UpdateInput struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	Runtime     *string          `json:"runtime"`
	Task        *string          `json:"task"`
	ModelPath   *string          `json:"model_path"`
	Config      *json.RawMessage `json:"config"`
}

// InferRuntime guesses a runtime from a model path.
func InferRuntime(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".gguf"):
		return models.RuntimeLlamaCpp
	case strings.HasSuffix(lower, ".onnx"):
		return models.RuntimeONNX
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if strings.HasSuffix(strings.ToLower(e.Name()), ".onnx") {
				return models.RuntimeONNX
			}
			if strings.HasSuffix(strings.ToLower(e.Name()), ".gguf") {
				return models.RuntimeLlamaCpp
			}
		}
	}
	return ""
}

// DefaultName derives a registry-friendly name from a path.
func DefaultName(path string) string {
	base := filepath.Base(strings.TrimRight(path, "/\\"))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), ".-_")
	if name == "" {
		name = "model"
	}
	if len(name) > 128 {
		name = name[:128]
	}
	return name
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*models.Model, error) {
	if in.ModelPath == "" {
		return nil, fmt.Errorf("%w: model_path is required", models.ErrValidation)
	}
	path := s.cfg.ResolveModelPath(in.ModelPath)
	if in.Runtime == "" {
		in.Runtime = InferRuntime(path)
		if in.Runtime == "" {
			return nil, fmt.Errorf("%w: could not infer runtime from %q; specify runtime", models.ErrValidation, in.ModelPath)
		}
	}
	if in.Name == "" {
		in.Name = DefaultName(path)
	}
	if in.Task == "" {
		in.Task = defaultTask(in.Runtime)
	}
	m := &models.Model{
		Name:        in.Name,
		Description: in.Description,
		Runtime:     in.Runtime,
		Task:        in.Task,
		ModelPath:   path,
		Config:      in.Config,
		Status:      models.StatusStopped,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	r, err := s.runtimes.For(*m)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if err := r.Validate(ctx, *m); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if err := s.registry.Create(ctx, m); err != nil {
		return nil, err
	}
	s.logs.Get(m.ID).Systemf("model %s registered (%s, %s)", m.Name, m.Runtime, m.Task)
	return m, nil
}

func defaultTask(runtimeName string) string {
	switch runtimeName {
	case models.RuntimeLlamaCpp:
		return models.TaskChat
	case models.RuntimeONNX:
		return models.TaskClassification
	}
	return models.TaskCustom
}

func (s *Service) Update(ctx context.Context, ref string, in UpdateInput) (*models.Model, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	l := s.lock(m.ID)
	l.Lock()
	defer l.Unlock()

	if in.Name != nil {
		m.Name = *in.Name
	}
	if in.Description != nil {
		m.Description = *in.Description
	}
	if in.Runtime != nil {
		m.Runtime = *in.Runtime
	}
	if in.Task != nil {
		m.Task = *in.Task
	}
	if in.ModelPath != nil {
		m.ModelPath = s.cfg.ResolveModelPath(*in.ModelPath)
	}
	if in.Config != nil {
		m.Config = *in.Config
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	r, err := s.runtimes.For(*m)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if err := r.Validate(ctx, *m); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrValidation, err)
	}
	if err := s.registry.Update(ctx, m); err != nil {
		return nil, err
	}
	s.logs.Get(m.ID).Systemf("configuration updated (restart to apply)")
	return m, nil
}

func (s *Service) Delete(ctx context.Context, ref string) error {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	l := s.lock(m.ID)
	l.Lock()
	defer l.Unlock()
	if r, err := s.runtimes.For(*m); err == nil {
		if err := r.Unload(ctx, m.ID); err != nil && !errors.Is(err, rt.ErrNotLoaded) {
			return err
		}
	}
	if err := s.registry.Delete(ctx, m.ID); err != nil {
		return err
	}
	s.logs.Remove(m.ID)
	s.mu.Lock()
	delete(s.locks, m.ID)
	s.mu.Unlock()
	return nil
}

func (s *Service) Get(ctx context.Context, ref string) (*ModelView, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, m), nil
}

func (s *Service) view(ctx context.Context, m *models.Model) *ModelView {
	v := &ModelView{Model: *m, Live: rt.Stopped()}
	if r, err := s.runtimes.For(*m); err == nil {
		if st, err := r.Status(ctx, m.ID); err == nil {
			v.Live = st
		}
	}
	v.Status = v.Live.State
	return v
}

func (s *Service) List(ctx context.Context) ([]*ModelView, error) {
	ms, err := s.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ModelView, 0, len(ms))
	for _, m := range ms {
		out = append(out, s.view(ctx, m))
	}
	return out, nil
}

// Load starts a model and blocks until it is running or failed.
func (s *Service) Load(ctx context.Context, ref string) (*ModelView, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	l := s.lock(m.ID)
	if !l.TryLock() {
		return nil, ErrModelBusy
	}
	defer l.Unlock()
	return s.loadLocked(ctx, m)
}

func (s *Service) loadLocked(ctx context.Context, m *models.Model) (*ModelView, error) {
	r, err := s.runtimes.For(*m)
	if err != nil {
		return nil, err
	}
	_ = s.registry.SetStatus(ctx, m.ID, models.StatusStarting)
	loadErr := r.Load(ctx, *m)
	v := s.view(ctx, m)
	if loadErr != nil {
		if errors.Is(loadErr, rt.ErrAlreadyLoaded) {
			return v, loadErr
		}
		v.Status = models.StatusFailed
		v.Live.State = models.StatusFailed
		if v.Live.Error == "" {
			v.Live.Error = loadErr.Error()
		}
		_ = s.registry.SetStatus(ctx, m.ID, models.StatusFailed)
		return v, loadErr
	}
	_ = s.registry.SetStatus(ctx, m.ID, models.StatusRunning)
	return v, nil
}

func (s *Service) Unload(ctx context.Context, ref string) (*ModelView, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	l := s.lock(m.ID)
	if !l.TryLock() {
		return nil, ErrModelBusy
	}
	defer l.Unlock()
	if err := s.unloadLocked(ctx, m); err != nil {
		return nil, err
	}
	return s.view(ctx, m), nil
}

func (s *Service) unloadLocked(ctx context.Context, m *models.Model) error {
	r, err := s.runtimes.For(*m)
	if err != nil {
		return err
	}
	_ = s.registry.SetStatus(ctx, m.ID, models.StatusStopping)
	if err := r.Unload(ctx, m.ID); err != nil && !errors.Is(err, rt.ErrNotLoaded) {
		_ = s.registry.SetStatus(ctx, m.ID, models.StatusFailed)
		return err
	}
	_ = s.registry.SetStatus(ctx, m.ID, models.StatusStopped)
	return nil
}

func (s *Service) Restart(ctx context.Context, ref string) (*ModelView, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}
	l := s.lock(m.ID)
	if !l.TryLock() {
		return nil, ErrModelBusy
	}
	defer l.Unlock()
	if err := s.unloadLocked(ctx, m); err != nil {
		return nil, err
	}
	return s.loadLocked(ctx, m)
}

func (s *Service) Status(ctx context.Context, ref string) (*ModelView, error) { return s.Get(ctx, ref) }

// Running returns the resolved model and its runtime if it is serving.
func (s *Service) Running(ctx context.Context, ref string) (*models.Model, rt.Runtime, error) {
	m, err := s.registry.Resolve(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	r, err := s.runtimes.For(*m)
	if err != nil {
		return nil, nil, err
	}
	st, err := r.Status(ctx, m.ID)
	if err != nil {
		return nil, nil, err
	}
	if st.State != models.StatusRunning {
		return m, r, fmt.Errorf("%w: model %q is %s", rt.ErrNotLoaded, m.Name, st.State)
	}
	return m, r, nil
}

// ProxyHandler returns the HTTP handler of a running generative model.
func (s *Service) ProxyHandler(ctx context.Context, ref string) (*models.Model, http.Handler, error) {
	m, r, err := s.Running(ctx, ref)
	if err != nil {
		return m, nil, err
	}
	p, ok := r.(rt.HTTPProxier)
	if !ok {
		return m, nil, fmt.Errorf("%w: runtime %s has no OpenAI-compatible endpoint", ErrTaskNotServing, r.Name())
	}
	h, err := p.ProxyHandler(m.ID)
	return m, h, err
}

func (s *Service) Predict(ctx context.Context, ref string, req rt.InferenceRequest) (*models.Model, rt.InferenceResponse, error) {
	m, r, err := s.Running(ctx, ref)
	if err != nil {
		return m, rt.InferenceResponse{}, err
	}
	resp, err := r.Infer(ctx, *m, req)
	return m, resp, err
}

// Shutdown stops every runtime and marks models stopped.
func (s *Service) Shutdown(ctx context.Context) {
	for _, r := range s.runtimes.All() {
		_ = r.Shutdown(ctx)
	}
	if ms, err := s.registry.List(ctx); err == nil {
		for _, m := range ms {
			if m.Status != models.StatusStopped {
				_ = s.registry.SetStatus(ctx, m.ID, models.StatusStopped)
			}
		}
	}
}

// ResetStatuses marks every model stopped; called at startup because runtime
// state does not survive restarts.
func (s *Service) ResetStatuses(ctx context.Context) error {
	ms, err := s.registry.List(ctx)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.Status != models.StatusStopped {
			if err := s.registry.SetStatus(ctx, m.ID, models.StatusStopped); err != nil {
				return err
			}
		}
	}
	return nil
}

// SystemInfo is returned by GET /api/system.
type SystemInfo struct {
	Version       string            `json:"version"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	CPUs          int               `json:"cpus"`
	Hostname      string            `json:"hostname"`
	GoVersion     string            `json:"go_version"`
	Memory        MemoryInfo        `json:"memory"`
	UptimeSeconds float64           `json:"uptime_seconds"`
	Runtimes      []rt.Info         `json:"runtimes"`
	Models        ModelCounts       `json:"models"`
	Server        map[string]any    `json:"server"`
	Paths         map[string]string `json:"paths"`
}

type MemoryInfo struct {
	TotalBytes     uint64 `json:"total_bytes,omitempty"`
	AvailableBytes uint64 `json:"available_bytes,omitempty"`
}

type ModelCounts struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Failed  int `json:"failed"`
}

func (s *Service) System(ctx context.Context) SystemInfo {
	host, _ := os.Hostname()
	info := SystemInfo{
		Version:       Version,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CPUs:          runtime.NumCPU(),
		Hostname:      host,
		GoVersion:     runtime.Version(),
		Memory:        readMemory(),
		UptimeSeconds: time.Since(s.started).Seconds(),
		Server: map[string]any{
			"host":         s.cfg.Server.Host,
			"port":         s.cfg.Server.Port,
			"ui_enabled":   s.cfg.Server.UI,
			"auth_enabled": s.cfg.AuthEnabled(),
		},
		Paths: map[string]string{
			"database":         s.cfg.Database.Path,
			"models_directory": s.cfg.Models.Directory,
		},
	}
	for _, r := range s.runtimes.All() {
		info.Runtimes = append(info.Runtimes, r.Info(ctx))
	}
	if ms, err := s.List(ctx); err == nil {
		info.Models.Total = len(ms)
		for _, m := range ms {
			switch m.Live.State {
			case models.StatusRunning:
				info.Models.Running++
			case models.StatusFailed:
				info.Models.Failed++
			}
		}
	}
	return info
}

// Health is a cheap liveness check.
func (s *Service) Health(ctx context.Context) (map[string]any, error) {
	if _, err := s.registry.List(ctx); err != nil {
		return map[string]any{"status": "error", "error": err.Error()}, err
	}
	return map[string]any{"status": "ok", "version": Version}, nil
}
