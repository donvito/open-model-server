package onnx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// layout is the resolved set of files backing an ONNX model.
type layout struct {
	Dir       string
	ModelFile string
	Tokenizer string // empty when absent
	Labels    []string
	PadID     int
}

// resolveLayout locates the .onnx file, tokenizer and label metadata for a
// model path that is either a directory or a .onnx file.
func resolveLayout(modelPath string, cfg Config) (layout, error) {
	st, err := os.Stat(modelPath)
	if err != nil {
		return layout{}, fmt.Errorf("model path does not exist: %s", modelPath)
	}
	var l layout
	if st.IsDir() {
		l.Dir = modelPath
		if cfg.ModelFile != "" {
			l.ModelFile = joinIfRelative(l.Dir, cfg.ModelFile)
		} else {
			l.ModelFile, err = findONNX(l.Dir)
			if err != nil {
				return layout{}, err
			}
		}
	} else {
		l.Dir = filepath.Dir(modelPath)
		l.ModelFile = modelPath
		if !strings.EqualFold(filepath.Ext(modelPath), ".onnx") {
			return layout{}, fmt.Errorf("model path %s is not a .onnx file or directory", modelPath)
		}
	}
	if _, err := os.Stat(l.ModelFile); err != nil {
		return layout{}, fmt.Errorf("onnx model file does not exist: %s", l.ModelFile)
	}
	if cfg.TokenizerFile != "" {
		l.Tokenizer = joinIfRelative(l.Dir, cfg.TokenizerFile)
		if _, err := os.Stat(l.Tokenizer); err != nil {
			return layout{}, fmt.Errorf("tokenizer file does not exist: %s", l.Tokenizer)
		}
	} else if p := filepath.Join(l.Dir, "tokenizer.json"); fileExists(p) {
		l.Tokenizer = p
	}
	l.Labels = cfg.Labels
	l.PadID = 0
	if hf, err := readHFConfig(filepath.Join(l.Dir, "config.json")); err == nil {
		if len(l.Labels) == 0 {
			l.Labels = hf.labels()
		}
		if hf.PadTokenID != nil {
			l.PadID = *hf.PadTokenID
		}
	}
	return l, nil
}

func joinIfRelative(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// findONNX picks the model file in a directory, preferring conventional names.
func findONNX(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var candidates []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".onnx") {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		// HF optimum layout: <dir>/onnx/model.onnx
		if sub := filepath.Join(dir, "onnx"); fileExists(filepath.Join(sub, "model.onnx")) {
			return filepath.Join(sub, "model.onnx"), nil
		}
		return "", fmt.Errorf("no .onnx file found in %s (set config.model_file)", dir)
	}
	preferred := []string{"model.onnx", "model_quantized.onnx", "model_int8.onnx", "model_fp16.onnx"}
	for _, p := range preferred {
		for _, c := range candidates {
			if strings.EqualFold(c, p) {
				return filepath.Join(dir, c), nil
			}
		}
	}
	sort.Strings(candidates)
	return filepath.Join(dir, candidates[0]), nil
}

type hfConfig struct {
	ID2Label   map[string]string `json:"id2label"`
	PadTokenID *int              `json:"pad_token_id"`
}

func readHFConfig(path string) (hfConfig, error) {
	var c hfConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

func (c hfConfig) labels() []string {
	if len(c.ID2Label) == 0 {
		return nil
	}
	out := make([]string, len(c.ID2Label))
	for k, v := range c.ID2Label {
		i, err := strconv.Atoi(k)
		if err != nil || i < 0 || i >= len(out) {
			return nil
		}
		out[i] = v
	}
	return out
}
