package llamacpp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvito/modelserver/internal/models"
)

func TestParseConfig(t *testing.T) {
	c, err := ParseConfig(json.RawMessage(`{"context_length":4096,"gpu_layers":-1,"threads":4,"extra_args":["--flash-attn"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.ContextLength != 4096 || c.GPULayers == nil || *c.GPULayers != -1 || c.Threads != 4 || len(c.ExtraArgs) != 1 {
		t.Fatalf("parsed = %+v", c)
	}
	if _, err := ParseConfig(nil); err != nil {
		t.Fatalf("empty config should be valid: %v", err)
	}

	bad := []string{
		`{"unknown_key":1}`,
		`{"context_length":-1}`,
		`{"port":70000}`,
		`{"extra_args":["ok","bad\nline"]}`,
		`{"context_length":"big"}`,
	}
	for _, raw := range bad {
		if _, err := ParseConfig(json.RawMessage(raw)); err == nil {
			t.Errorf("%s: expected error", raw)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	gpu := 12
	m := models.Model{Name: "gemma", ModelPath: "/models/gemma.gguf", Task: models.TaskChat}
	c := Config{ContextLength: 2048, GPULayers: &gpu, Threads: 8, BatchSize: 256, ExtraArgs: []string{"--flash-attn"}}
	got := strings.Join(BuildArgs(m, c, "127.0.0.1", 12000), " ")
	want := "-m /models/gemma.gguf --host 127.0.0.1 --port 12000 --alias gemma --no-webui -c 2048 -ngl 12 -t 8 -b 256 --flash-attn"
	if got != want {
		t.Fatalf("args:\n got %s\nwant %s", got, want)
	}

	all := -1
	got = strings.Join(BuildArgs(models.Model{Name: "e", ModelPath: "/e.gguf", Task: models.TaskEmbedding}, Config{GPULayers: &all}, "127.0.0.1", 1), " ")
	if !strings.Contains(got, "-ngl all") || !strings.HasSuffix(got, "--embeddings") {
		t.Fatalf("embedding args = %s", got)
	}
	got = strings.Join(BuildArgs(models.Model{Name: "r", ModelPath: "/r.gguf", Task: models.TaskReranking}, Config{}, "127.0.0.1", 1), " ")
	if !strings.HasSuffix(got, "--reranking") {
		t.Fatalf("reranking args = %s", got)
	}
}
