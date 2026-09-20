package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if cfg.Server.Port != 9090 || !cfg.Server.UI {
		t.Fatalf("unexpected defaults: %+v", cfg.Server)
	}
	if cfg.Addr() != "127.0.0.1:9090" {
		t.Fatalf("addr = %q", cfg.Addr())
	}
}

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "modelserver.yaml")
	os.WriteFile(file, []byte("server:\n  port: 8000\n  host: 0.0.0.0\ndatabase:\n  path: file.db\nllamacpp:\n  binary: /from/file\n"), 0o644)

	t.Setenv(EnvPrefix+"PORT", "8100")
	t.Setenv(EnvPrefix+"LLAMA_CPP_BINARY", "/from/env")
	t.Setenv(EnvPrefix+"API_KEYS", "a, b")
	t.Setenv(EnvPrefix+"HEADLESS", "true")

	port := 8200
	cfg, err := Load(file, Overrides{Port: &port})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8200 {
		t.Errorf("flag should win: port=%d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("file value should apply: host=%s", cfg.Server.Host)
	}
	if cfg.LlamaCpp.Binary != "/from/env" {
		t.Errorf("env should beat file: %s", cfg.LlamaCpp.Binary)
	}
	if cfg.Database.Path != "file.db" {
		t.Errorf("db path = %s", cfg.Database.Path)
	}
	if len(cfg.Auth.APIKeys) != 2 || cfg.Auth.APIKeys[1] != "b" || !cfg.AuthEnabled() {
		t.Errorf("api keys = %v", cfg.Auth.APIKeys)
	}
	if cfg.Server.UI {
		t.Error("HEADLESS env should disable UI")
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), Overrides{}); err == nil {
		t.Fatal("expected error for explicit missing file")
	}
}

func TestLoadMissingDefaultFileIsFine(t *testing.T) {
	cwd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(cwd)
	t.Setenv(EnvPrefix+"CONFIG", "")
	if _, err := Load("", Overrides{}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEnvBadInteger(t *testing.T) {
	cfg := Default()
	err := applyEnv(&cfg, func(k string) (string, bool) {
		if k == EnvPrefix+"PORT" {
			return "abc", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "PORT") {
		t.Fatalf("expected PORT parse error, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]func(*Config){
		"port":       func(c *Config) { c.Server.Port = 0 },
		"port range": func(c *Config) { c.LlamaCpp.PortRangeStart = 5000; c.LlamaCpp.PortRangeEnd = 4000 },
		"logs":       func(c *Config) { c.Logs.LinesPerModel = 0 },
		"db":         func(c *Config) { c.Database.Path = " " },
	}
	for name, mutate := range cases {
		cfg := Default()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestResolveModelPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "m.gguf"), nil, 0o644)
	cfg := Default()
	cfg.Models.Directory = dir
	if got := cfg.ResolveModelPath("m.gguf"); got != filepath.Join(dir, "m.gguf") {
		t.Errorf("relative path should resolve in models dir: %s", got)
	}
	abs := filepath.Join(dir, "elsewhere", "x.gguf")
	if got := cfg.ResolveModelPath(abs); got != abs {
		t.Errorf("absolute path should be untouched: %s", got)
	}
}
