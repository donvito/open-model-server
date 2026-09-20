// Package config loads server configuration from defaults, a YAML file,
// environment variables and (via Overrides) CLI flags, in increasing precedence.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const EnvPrefix = "MODELSERVER_"

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Models   ModelsConfig   `yaml:"models"`
	LlamaCpp LlamaCppConfig `yaml:"llamacpp"`
	ONNX     ONNXConfig     `yaml:"onnx"`
	Auth     AuthConfig     `yaml:"auth"`
	Logs     LogsConfig     `yaml:"logs"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// UI controls whether the embedded web frontend is served. Headless mode sets it to false.
	UI bool `yaml:"ui"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type ModelsConfig struct {
	// Directory is used to resolve relative model paths.
	Directory string `yaml:"directory"`
}

type LlamaCppConfig struct {
	// Binary is the llama-server executable. If not absolute it is looked up in PATH.
	Binary string `yaml:"binary"`
	// PortRange is the internal port range handed to llama-server processes.
	PortRangeStart int `yaml:"port_range_start"`
	PortRangeEnd   int `yaml:"port_range_end"`
	// StartupTimeoutSeconds bounds how long a model may take to become healthy.
	StartupTimeoutSeconds int `yaml:"startup_timeout_seconds"`
}

type ONNXConfig struct {
	// Library is the path to the ONNX Runtime shared library (libonnxruntime.so / .dylib / .dll).
	Library string `yaml:"library"`
}

type AuthConfig struct {
	// APIKeys enables bearer auth when non-empty.
	APIKeys []string `yaml:"api_keys"`
}

type LogsConfig struct {
	// LinesPerModel is the size of the in-memory ring buffer per model.
	LinesPerModel int `yaml:"lines_per_model"`
}

func Default() Config {
	return Config{
		Server:   ServerConfig{Host: "127.0.0.1", Port: 9090, UI: true},
		Database: DatabaseConfig{Path: "./data/modelserver.db"},
		Models:   ModelsConfig{Directory: "./models"},
		LlamaCpp: LlamaCppConfig{Binary: "llama-server", PortRangeStart: 12000, PortRangeEnd: 12999, StartupTimeoutSeconds: 600},
		ONNX:     ONNXConfig{Library: ""},
		Logs:     LogsConfig{LinesPerModel: 2000},
	}
}

// Overrides carries explicitly-set CLI values. Nil pointers mean "not set".
type Overrides struct {
	Host         *string
	Port         *int
	UI           *bool
	DatabasePath *string
	ModelsDir    *string
	LlamaBinary  *string
	ONNXLibrary  *string
	APIKeys      *[]string
}

// Load builds the effective configuration. configPath may be empty, in which
// case MODELSERVER_CONFIG and then ./modelserver.yaml are tried; a missing file
// is not an error unless it was named explicitly.
func Load(configPath string, ov Overrides) (Config, error) {
	cfg := Default()

	explicit := configPath != ""
	if configPath == "" {
		configPath = os.Getenv(EnvPrefix + "CONFIG")
		explicit = configPath != ""
	}
	if configPath == "" {
		configPath = "modelserver.yaml"
	}
	if err := loadFile(&cfg, configPath, explicit); err != nil {
		return cfg, err
	}
	if err := applyEnv(&cfg, os.LookupEnv); err != nil {
		return cfg, err
	}
	applyOverrides(&cfg, ov)
	return cfg, cfg.Validate()
}

func loadFile(cfg *Config, path string, mustExist bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !mustExist {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	return nil
}

type lookupFunc func(string) (string, bool)

func applyEnv(cfg *Config, lookup lookupFunc) error {
	str := func(key string, dst *string) {
		if v, ok := lookup(EnvPrefix + key); ok {
			*dst = v
		}
	}
	var firstErr error
	integer := func(key string, dst *int) {
		v, ok := lookup(EnvPrefix + key)
		if !ok {
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s%s: %w", EnvPrefix, key, err)
			}
			return
		}
		*dst = n
	}
	boolean := func(key string, dst *bool) {
		v, ok := lookup(EnvPrefix + key)
		if !ok {
			return
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s%s: %w", EnvPrefix, key, err)
			}
			return
		}
		*dst = b
	}

	str("HOST", &cfg.Server.Host)
	integer("PORT", &cfg.Server.Port)
	boolean("UI", &cfg.Server.UI)
	if v, ok := lookup(EnvPrefix + "HEADLESS"); ok {
		if b, err := strconv.ParseBool(v); err == nil && b {
			cfg.Server.UI = false
		}
	}
	str("DATABASE_PATH", &cfg.Database.Path)
	str("MODELS_DIRECTORY", &cfg.Models.Directory)
	str("LLAMA_CPP_BINARY", &cfg.LlamaCpp.Binary)
	integer("LLAMA_CPP_PORT_RANGE_START", &cfg.LlamaCpp.PortRangeStart)
	integer("LLAMA_CPP_PORT_RANGE_END", &cfg.LlamaCpp.PortRangeEnd)
	integer("LLAMA_CPP_STARTUP_TIMEOUT_SECONDS", &cfg.LlamaCpp.StartupTimeoutSeconds)
	str("ONNX_LIBRARY", &cfg.ONNX.Library)
	integer("LOGS_LINES_PER_MODEL", &cfg.Logs.LinesPerModel)
	if v, ok := lookup(EnvPrefix + "API_KEYS"); ok {
		cfg.Auth.APIKeys = splitKeys(v)
	}
	return firstErr
}

func splitKeys(v string) []string {
	var out []string
	for _, k := range strings.Split(v, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func applyOverrides(cfg *Config, ov Overrides) {
	if ov.Host != nil {
		cfg.Server.Host = *ov.Host
	}
	if ov.Port != nil {
		cfg.Server.Port = *ov.Port
	}
	if ov.UI != nil {
		cfg.Server.UI = *ov.UI
	}
	if ov.DatabasePath != nil {
		cfg.Database.Path = *ov.DatabasePath
	}
	if ov.ModelsDir != nil {
		cfg.Models.Directory = *ov.ModelsDir
	}
	if ov.LlamaBinary != nil {
		cfg.LlamaCpp.Binary = *ov.LlamaBinary
	}
	if ov.ONNXLibrary != nil {
		cfg.ONNX.Library = *ov.ONNXLibrary
	}
	if ov.APIKeys != nil {
		cfg.Auth.APIKeys = *ov.APIKeys
	}
}

func (c Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range", c.Server.Port)
	}
	if c.LlamaCpp.PortRangeStart <= 0 || c.LlamaCpp.PortRangeEnd > 65535 || c.LlamaCpp.PortRangeStart > c.LlamaCpp.PortRangeEnd {
		return fmt.Errorf("llamacpp port range %d-%d is invalid", c.LlamaCpp.PortRangeStart, c.LlamaCpp.PortRangeEnd)
	}
	if c.Logs.LinesPerModel <= 0 {
		return errors.New("logs.lines_per_model must be positive")
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		return errors.New("database.path is required")
	}
	return nil
}

// Addr returns the host:port the HTTP server binds to.
func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// AuthEnabled reports whether bearer authentication is active.
func (c Config) AuthEnabled() bool { return len(c.Auth.APIKeys) > 0 }

// ResolveModelPath makes a model path absolute, resolving relative paths
// against the configured models directory first and then the working directory.
func (c Config) ResolveModelPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	if c.Models.Directory != "" {
		candidate := filepath.Join(c.Models.Directory, p)
		if _, err := os.Stat(candidate); err == nil {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
