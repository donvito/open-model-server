// Package llamacpp serves GGUF models by managing llama-server child processes.
package llamacpp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/donvito/modelserver/internal/models"
)

// Config is the per-model llama.cpp configuration stored in Model.Config.
type Config struct {
	ContextLength int `json:"context_length,omitempty"`
	// GPULayers is the number of layers to offload; -1 means all. nil leaves llama-server's default.
	GPULayers *int `json:"gpu_layers,omitempty"`
	Threads   int  `json:"threads,omitempty"`
	BatchSize int  `json:"batch_size,omitempty"`
	// Host llama-server binds to; defaults to 127.0.0.1.
	Host string `json:"host,omitempty"`
	// Port is the internal llama-server port; 0 allocates one from the pool.
	Port int `json:"port,omitempty"`
	// ExtraArgs are appended verbatim to the llama-server command line.
	ExtraArgs []string `json:"extra_args,omitempty"`
}

func ParseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) == 0 {
		return c, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid llamacpp config: %w", err)
	}
	if c.ContextLength < 0 || c.Threads < 0 || c.BatchSize < 0 || c.Port < 0 || c.Port > 65535 {
		return c, fmt.Errorf("invalid llamacpp config: numeric values must be non-negative")
	}
	for _, a := range c.ExtraArgs {
		if strings.ContainsAny(a, "\n\r\x00") {
			return c, fmt.Errorf("invalid llamacpp config: extra_args may not contain control characters")
		}
	}
	return c, nil
}

// BuildArgs constructs the llama-server command line for a model.
func BuildArgs(m models.Model, c Config, host string, port int) []string {
	args := []string{
		"-m", m.ModelPath,
		"--host", host,
		"--port", strconv.Itoa(port),
		"--alias", m.Name,
		"--no-webui",
	}
	if c.ContextLength > 0 {
		args = append(args, "-c", strconv.Itoa(c.ContextLength))
	}
	if c.GPULayers != nil {
		if *c.GPULayers < 0 {
			args = append(args, "-ngl", "all")
		} else {
			args = append(args, "-ngl", strconv.Itoa(*c.GPULayers))
		}
	}
	if c.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(c.Threads))
	}
	if c.BatchSize > 0 {
		args = append(args, "-b", strconv.Itoa(c.BatchSize))
	}
	if m.Task == models.TaskEmbedding {
		args = append(args, "--embeddings")
	}
	if m.Task == models.TaskReranking {
		args = append(args, "--reranking")
	}
	args = append(args, c.ExtraArgs...)
	return args
}
