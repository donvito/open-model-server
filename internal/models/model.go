// Package models defines the application's runtime-agnostic domain types.
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Runtime identifiers.
const (
	RuntimeLlamaCpp = "llamacpp"
	RuntimeONNX     = "onnx"
)

// Task identifiers. These are intentionally loose; runtimes interpret them.
const (
	TaskChat           = "chat"
	TaskCompletion     = "completion"
	TaskClassification = "classification"
	TaskEmbedding      = "embedding"
	TaskReranking      = "reranking"
	TaskCustom         = "custom"
)

// Lifecycle states of a model. Persisted state records the last requested
// state; the live runtime is the source of truth while the server runs.
const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusFailed   = "failed"
	StatusStopping = "stopping"
)

var KnownRuntimes = []string{RuntimeLlamaCpp, RuntimeONNX}
var KnownTasks = []string{TaskChat, TaskCompletion, TaskClassification, TaskEmbedding, TaskReranking, TaskCustom}

// Model is the persistent registry entry for a servable model. Runtime
// specific settings live opaquely in Config and are interpreted by the runtime
// package named by Runtime.
type Model struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Runtime     string          `json:"runtime"`
	Task        string          `json:"task"`
	ModelPath   string          `json:"model_path"`
	Config      json.RawMessage `json:"config"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

var (
	ErrNotFound   = errors.New("model not found")
	ErrNameTaken  = errors.New("a model with that name already exists")
	ErrValidation = errors.New("validation error")
)

// ValidateName checks that a model name is URL and shell friendly.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: name %q must match %s", ErrValidation, name, nameRe.String())
	}
	return nil
}

// Validate checks runtime-agnostic invariants.
func (m *Model) Validate() error {
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	if !contains(KnownRuntimes, m.Runtime) {
		return fmt.Errorf("%w: unknown runtime %q (want one of %s)", ErrValidation, m.Runtime, strings.Join(KnownRuntimes, ", "))
	}
	if m.Task == "" {
		m.Task = TaskCustom
	}
	if !contains(KnownTasks, m.Task) {
		return fmt.Errorf("%w: unknown task %q (want one of %s)", ErrValidation, m.Task, strings.Join(KnownTasks, ", "))
	}
	if strings.TrimSpace(m.ModelPath) == "" {
		return fmt.Errorf("%w: model_path is required", ErrValidation)
	}
	if len(m.Config) == 0 {
		m.Config = json.RawMessage(`{}`)
	}
	if !json.Valid(m.Config) {
		return fmt.Errorf("%w: config must be valid JSON", ErrValidation)
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
