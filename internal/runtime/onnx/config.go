// Package onnx serves ONNX models in-process through ONNX Runtime.
package onnx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config is the per-model ONNX configuration stored in Model.Config.
type Config struct {
	// ModelFile is the .onnx file, relative to the model directory when
	// ModelPath is a directory. Auto-detected when empty.
	ModelFile string `json:"model_file,omitempty"`
	// TokenizerFile is a HuggingFace tokenizer.json; defaults to
	// <dir>/tokenizer.json. Required for text tasks.
	TokenizerFile string `json:"tokenizer_file,omitempty"`
	// MaxLength truncates tokenized input; defaults to 512.
	MaxLength int `json:"max_length,omitempty"`
	// Labels overrides the id2label mapping read from config.json.
	Labels []string `json:"labels,omitempty"`
	// Pooling for embeddings: "mean" (default), "cls" or "none".
	Pooling string `json:"pooling,omitempty"`
	// Normalize L2-normalizes embeddings; defaults to true.
	Normalize *bool `json:"normalize,omitempty"`
	// Threads sets intra-op parallelism; 0 leaves ONNX Runtime's default.
	Threads int `json:"threads,omitempty"`
}

func ParseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) == 0 {
		return c, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid onnx config: %w", err)
	}
	if c.MaxLength < 0 || c.Threads < 0 {
		return c, fmt.Errorf("invalid onnx config: numeric values must be non-negative")
	}
	switch c.Pooling {
	case "", "mean", "cls", "none":
	default:
		return c, fmt.Errorf("invalid onnx config: pooling must be mean, cls or none")
	}
	return c, nil
}

func (c Config) maxLength() int {
	if c.MaxLength > 0 {
		return c.MaxLength
	}
	return 512
}

func (c Config) pooling() string {
	if c.Pooling == "" {
		return "mean"
	}
	return c.Pooling
}

func (c Config) normalize() bool { return c.Normalize == nil || *c.Normalize }
