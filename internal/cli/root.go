// Package cli implements the modelserver command line.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/internal/service"
)

type globalFlags struct {
	configPath string
	host       string
	port       int
	dbPath     string
	modelsDir  string
	llamaBin   string
	onnxLib    string
	apiKeys    []string
	jsonOut    bool
}

func (g *globalFlags) overrides(cmd *cobra.Command) config.Overrides {
	var ov config.Overrides
	f := cmd.Flags()
	if f.Changed("host") {
		ov.Host = &g.host
	}
	if f.Changed("port") {
		ov.Port = &g.port
	}
	if f.Changed("db") {
		ov.DatabasePath = &g.dbPath
	}
	if f.Changed("models-dir") {
		ov.ModelsDir = &g.modelsDir
	}
	if f.Changed("llama-binary") {
		ov.LlamaBinary = &g.llamaBin
	}
	if f.Changed("onnx-library") {
		ov.ONNXLibrary = &g.onnxLib
	}
	if f.Changed("api-key") {
		ov.APIKeys = &g.apiKeys
	}
	return ov
}

func (g *globalFlags) load(cmd *cobra.Command) (config.Config, error) {
	return config.Load(g.configPath, g.overrides(cmd))
}

// NewRootCommand builds the CLI tree.
func NewRootCommand(stdout, stderr io.Writer) *cobra.Command {
	g := &globalFlags{}
	root := &cobra.Command{
		Use:           "modelserver",
		Short:         "Self-hosted small model server (llama.cpp + ONNX Runtime)",
		Long:          "modelserver registers local GGUF and ONNX models and serves them through OpenAI-compatible and prediction APIs with an optional web dashboard.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       service.Version,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	pf := root.PersistentFlags()
	pf.StringVarP(&g.configPath, "config", "c", "", "path to modelserver.yaml (default ./modelserver.yaml if present)")
	pf.StringVar(&g.host, "host", "", "server bind host (default 127.0.0.1)")
	pf.IntVar(&g.port, "port", 0, "server port (default 9090)")
	pf.StringVar(&g.dbPath, "db", "", "SQLite database path (default ./data/modelserver.db)")
	pf.StringVar(&g.modelsDir, "models-dir", "", "directory relative model paths resolve against (default ./models)")
	pf.StringVar(&g.llamaBin, "llama-binary", "", "llama-server binary path or name (default llama-server)")
	pf.StringVar(&g.onnxLib, "onnx-library", "", "path to the ONNX Runtime shared library")
	pf.StringSliceVar(&g.apiKeys, "api-key", nil, "API key(s) required as Bearer tokens; repeatable")
	pf.BoolVar(&g.jsonOut, "json", false, "print machine-readable JSON")

	root.AddCommand(newServeCommand(g), newModelsCommand(g), newSystemCommand(g))
	return root
}

// Execute runs the CLI and returns an exit code.
func Execute(args []string) int {
	root := NewRootCommand(os.Stdout, os.Stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
