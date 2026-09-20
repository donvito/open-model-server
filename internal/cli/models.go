package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/donvito/modelserver/internal/app"
	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/service"
)

func newModelsCommand(g *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "models",
		Aliases: []string{"model"},
		Short:   "Manage registered models",
	}
	cmd.AddCommand(
		newModelsList(g),
		newModelsAdd(g),
		newModelsRemove(g),
		newModelsLoad(g),
		newModelsUnload(g),
		newModelsRestart(g),
		newModelsStatus(g),
		newModelsLogs(g),
	)
	return cmd
}

// backend abstracts "talk to the running server" vs "open the database directly".
type backend struct {
	client *client
	app    *app.App
}

func (b *backend) close() {
	if b.app != nil {
		b.app.Close(context.Background())
	}
}

// open prefers a running server; when none is reachable it falls back to the
// database directly if allowLocal is set (registry-only commands).
func open(ctx context.Context, cfg config.Config, allowLocal bool) (*backend, error) {
	c := newClient(cfg)
	if c.reachable(ctx) {
		return &backend{client: c}, nil
	}
	if !allowLocal {
		return nil, fmt.Errorf("no modelserver running at %s; start one with `modelserver serve`", cfg.Addr())
	}
	a, err := app.New(cfg, newLogger(io.Discard))
	if err != nil {
		return nil, err
	}
	return &backend{app: a}, nil
}

func (b *backend) list(ctx context.Context) ([]*service.ModelView, error) {
	if b.client != nil {
		var out struct {
			Models []*service.ModelView `json:"models"`
		}
		err := b.client.do(ctx, "GET", "/api/models", nil, &out)
		return out.Models, err
	}
	return b.app.Service.List(ctx)
}

func (b *backend) get(ctx context.Context, ref string) (*service.ModelView, error) {
	if b.client != nil {
		var out service.ModelView
		err := b.client.do(ctx, "GET", modelPath(ref), nil, &out)
		return &out, err
	}
	return b.app.Service.Get(ctx, ref)
}

func (b *backend) create(ctx context.Context, in service.CreateInput) (*service.ModelView, error) {
	if b.client != nil {
		var out service.ModelView
		err := b.client.do(ctx, "POST", "/api/models", in, &out)
		return &out, err
	}
	m, err := b.app.Service.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return b.app.Service.Get(ctx, m.ID)
}

func (b *backend) remove(ctx context.Context, ref string) error {
	if b.client != nil {
		return b.client.do(ctx, "DELETE", modelPath(ref), nil, nil)
	}
	return b.app.Service.Delete(ctx, ref)
}

func (b *backend) action(ctx context.Context, ref, action string) (*service.ModelView, error) {
	var out service.ModelView
	err := b.client.do(ctx, "POST", modelPath(ref)+"/"+action, nil, &out)
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Code == "load_failed" {
			return nil, err
		}
		return nil, err
	}
	return &out, nil
}

// --- output helpers ---

func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printModels(cmd *cobra.Command, ms []*service.ModelView) {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tRUNTIME\tTASK\tSTATUS\tPATH")
	for _, m := range ms {
		st := m.Live.State
		if m.Live.Error != "" {
			st += " (" + truncate(m.Live.Error, 60) + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Name, m.Runtime, m.Task, st, m.ModelPath)
	}
	tw.Flush()
}

func printModel(cmd *cobra.Command, m *service.ModelView) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Name:     %s\n", m.Name)
	fmt.Fprintf(w, "ID:       %s\n", m.ID)
	fmt.Fprintf(w, "Runtime:  %s\n", m.Runtime)
	fmt.Fprintf(w, "Task:     %s\n", m.Task)
	fmt.Fprintf(w, "Path:     %s\n", m.ModelPath)
	fmt.Fprintf(w, "Status:   %s\n", m.Live.State)
	if m.Live.Error != "" {
		fmt.Fprintf(w, "Error:    %s\n", m.Live.Error)
	}
	if m.Live.PID != 0 {
		fmt.Fprintf(w, "PID:      %d\n", m.Live.PID)
	}
	if m.Live.Port != 0 {
		fmt.Fprintf(w, "Port:     %d\n", m.Live.Port)
	}
	if m.Live.StartedAt != nil {
		fmt.Fprintf(w, "Uptime:   %s\n", time.Since(*m.Live.StartedAt).Round(time.Second))
	}
	if len(m.Config) > 0 && string(m.Config) != "{}" {
		fmt.Fprintf(w, "Config:   %s\n", string(m.Config))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// --- subcommands ---

func newModelsList(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered models and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			b, err := open(cmd.Context(), cfg, true)
			if err != nil {
				return err
			}
			defer b.close()
			ms, err := b.list(cmd.Context())
			if err != nil {
				return err
			}
			if g.jsonOut {
				return printJSON(cmd, ms)
			}
			if len(ms) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No models registered. Add one with: modelserver models add ./model.gguf")
				return nil
			}
			printModels(cmd, ms)
			return nil
		},
	}
}

func newModelsAdd(g *globalFlags) *cobra.Command {
	var name, runtime, task, desc, configJSON string
	var load bool
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register a local model file or directory",
		Example: `  modelserver models add ./gemma-2b.gguf
  modelserver models add ./sst2-onnx --runtime onnx --task classification
  modelserver models add ./minilm --runtime onnx --task embedding --name minilm --load`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			path := args[0]
			if abs, err := filepath.Abs(path); err == nil {
				if _, statErr := os.Stat(abs); statErr == nil {
					path = abs
				}
			}
			in := service.CreateInput{Name: name, Runtime: runtime, Task: task, Description: desc, ModelPath: path}
			if configJSON != "" {
				if !json.Valid([]byte(configJSON)) {
					return fmt.Errorf("--config must be a JSON object")
				}
				in.Config = json.RawMessage(configJSON)
			}
			b, err := open(cmd.Context(), cfg, !load)
			if err != nil {
				return err
			}
			defer b.close()
			m, err := b.create(cmd.Context(), in)
			if err != nil {
				return err
			}
			if load {
				m, err = b.action(cmd.Context(), m.ID, "load")
				if err != nil {
					return err
				}
			}
			if g.jsonOut {
				return printJSON(cmd, m)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered %s (%s, %s) -> %s\n", m.Name, m.Runtime, m.Task, m.ModelPath)
			if load {
				fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", m.Live.State)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "registry name (default: derived from the file name)")
	cmd.Flags().StringVar(&runtime, "runtime", "", "runtime: llamacpp or onnx (default: inferred from the path)")
	cmd.Flags().StringVar(&task, "task", "", "task: chat, completion, embedding, reranking, classification, custom")
	cmd.Flags().StringVar(&desc, "description", "", "free-form description")
	cmd.Flags().StringVar(&configJSON, "config", "", "runtime-specific JSON config, e.g. '{\"context_length\":4096}'")
	cmd.Flags().BoolVar(&load, "load", false, "load the model right away (requires a running server)")
	return cmd
}

func newModelsRemove(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name|id>",
		Aliases: []string{"rm", "delete"},
		Short:   "Unregister a model (stops it first if running)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			b, err := open(cmd.Context(), cfg, true)
			if err != nil {
				return err
			}
			defer b.close()
			if err := b.remove(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])
			return nil
		},
	}
}

func actionCommand(g *globalFlags, use, action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <name|id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			b, err := open(cmd.Context(), cfg, false)
			if err != nil {
				return err
			}
			m, err := b.action(cmd.Context(), args[0], action)
			if err != nil {
				return err
			}
			if g.jsonOut {
				return printJSON(cmd, m)
			}
			printModel(cmd, m)
			return nil
		},
	}
}

func newModelsLoad(g *globalFlags) *cobra.Command {
	return actionCommand(g, "load", "load", "Load (start) a model; blocks until it is ready")
}

func newModelsUnload(g *globalFlags) *cobra.Command {
	return actionCommand(g, "unload", "unload", "Unload (stop) a model")
}

func newModelsRestart(g *globalFlags) *cobra.Command {
	return actionCommand(g, "restart", "restart", "Restart a model with its current configuration")
}

func newModelsStatus(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status <name|id>",
		Short: "Show a model's live status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			b, err := open(cmd.Context(), cfg, true)
			if err != nil {
				return err
			}
			defer b.close()
			m, err := b.get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if g.jsonOut {
				return printJSON(cmd, m)
			}
			printModel(cmd, m)
			if b.client == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "(no server running; live status unavailable)")
			}
			return nil
		},
	}
}

func newModelsLogs(g *globalFlags) *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "logs <name|id>",
		Short: "Show recent runtime logs for a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			b, err := open(cmd.Context(), cfg, false)
			if err != nil {
				return err
			}
			var out struct {
				Lines []logs.Line `json:"lines"`
			}
			if err := b.client.do(cmd.Context(), "GET", modelPath(args[0])+fmt.Sprintf("/logs?lines=%d", lines), nil, &out); err != nil {
				return err
			}
			if g.jsonOut {
				return printJSON(cmd, out.Lines)
			}
			for _, l := range out.Lines {
				fmt.Fprintf(cmd.OutOrStdout(), "%s [%s] %s\n", l.Time.Format(time.TimeOnly), strings.ToUpper(l.Source), l.Text)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of lines")
	return cmd
}
