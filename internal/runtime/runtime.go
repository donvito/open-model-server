// Package runtime defines the abstraction between the application layer and
// concrete model runtimes (llama.cpp, ONNX Runtime, ...).
package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/donvito/modelserver/internal/models"
)

var (
	ErrNotLoaded       = errors.New("model is not loaded")
	ErrAlreadyLoaded   = errors.New("model is already loaded")
	ErrUnknownRuntime  = errors.New("unknown runtime")
	ErrUnsupportedTask = errors.New("unsupported task for this runtime")
	ErrUnavailable     = errors.New("runtime is unavailable")
)

// Status is the live, in-memory state of a loaded model.
type Status struct {
	State     string         `json:"state"`
	Error     string         `json:"error,omitempty"`
	PID       int            `json:"pid,omitempty"`
	Port      int            `json:"port,omitempty"`
	StartedAt *time.Time     `json:"started_at,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Stopped is the status of a model with no live state.
func Stopped() Status { return Status{State: models.StatusStopped} }

// InferenceRequest is the runtime-agnostic prediction input.
type InferenceRequest struct {
	// Input is a string, a list of strings, or a runtime-specific object.
	Input any `json:"input"`
	// Params carries task-specific options (e.g. max_tokens, top_k).
	Params map[string]any `json:"params,omitempty"`
}

type Timing struct {
	LatencyMS       float64 `json:"latency_ms"`
	TokensPerSecond float64 `json:"tokens_per_second,omitempty"`
	Tokens          int     `json:"tokens,omitempty"`
}

type InferenceResponse struct {
	Output any    `json:"output"`
	Timing Timing `json:"timing"`
}

// Info describes a runtime's availability on this host.
type Info struct {
	Name      string         `json:"name"`
	Available bool           `json:"available"`
	Error     string         `json:"error,omitempty"`
	Version   string         `json:"version,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Runtime is implemented by every model backend.
type Runtime interface {
	Name() string
	// Info reports whether the runtime can be used (binary/library present).
	Info(ctx context.Context) Info
	// Validate checks the model's path and runtime config without loading it.
	Validate(ctx context.Context, model models.Model) error
	// Load makes the model ready to serve. It blocks until the model is
	// running or has failed.
	Load(ctx context.Context, model models.Model) error
	Unload(ctx context.Context, modelID string) error
	Status(ctx context.Context, modelID string) (Status, error)
	Infer(ctx context.Context, model models.Model, request InferenceRequest) (InferenceResponse, error)
	// Shutdown unloads everything; called when the server exits.
	Shutdown(ctx context.Context) error
}

// HTTPProxier is implemented by runtimes that expose an HTTP API for a loaded
// model (e.g. llama-server's OpenAI-compatible endpoints).
type HTTPProxier interface {
	ProxyHandler(modelID string) (http.Handler, error)
}

// Registry maps runtime names to implementations.
type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]Runtime
}

func NewRegistry(rts ...Runtime) *Registry {
	r := &Registry{runtimes: map[string]Runtime{}}
	for _, rt := range rts {
		r.Register(rt)
	}
	return r
}

func (r *Registry) Register(rt Runtime) {
	r.mu.Lock()
	r.runtimes[rt.Name()] = rt
	r.mu.Unlock()
}

func (r *Registry) Get(name string) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownRuntime, name)
	}
	return rt, nil
}

// For resolves the runtime that serves m.
func (r *Registry) For(m models.Model) (Runtime, error) { return r.Get(m.Runtime) }

func (r *Registry) All() []Runtime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Runtime, 0, len(r.runtimes))
	for _, rt := range r.runtimes {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
