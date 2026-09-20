package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/donvito/modelserver/internal/app"
	"github.com/donvito/modelserver/internal/service"
)

func newSystemCommand(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "system",
		Short: "Show host, runtime availability and server information",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			var info service.SystemInfo
			c := newClient(cfg)
			if c.reachable(cmd.Context()) {
				if err := c.do(cmd.Context(), "GET", "/api/system", nil, &info); err != nil {
					return err
				}
			} else {
				a, err := app.New(cfg, newLogger(io.Discard))
				if err != nil {
					return err
				}
				info = a.Service.System(cmd.Context())
				a.Close(cmd.Context())
				info.Server["running"] = false
			}
			if g.jsonOut {
				return printJSON(cmd, info)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "modelserver %s on %s/%s (%d CPUs)\n", info.Version, info.OS, info.Arch, info.CPUs)
			if info.Memory.TotalBytes > 0 {
				fmt.Fprintf(w, "Memory:   %.1f GiB total, %.1f GiB available\n", gib(info.Memory.TotalBytes), gib(info.Memory.AvailableBytes))
			}
			fmt.Fprintf(w, "Server:   http://%s:%v (ui=%v auth=%v)\n", info.Server["host"], info.Server["port"], info.Server["ui_enabled"], info.Server["auth_enabled"])
			fmt.Fprintf(w, "Database: %s\n", info.Paths["database"])
			fmt.Fprintf(w, "Models:   %d registered, %d running, %d failed\n", info.Models.Total, info.Models.Running, info.Models.Failed)
			fmt.Fprintln(w, "Runtimes:")
			for _, r := range info.Runtimes {
				if r.Available {
					fmt.Fprintf(w, "  %-9s available  %s  %v\n", r.Name, r.Version, r.Details)
				} else {
					fmt.Fprintf(w, "  %-9s unavailable: %s\n", r.Name, r.Error)
				}
			}
			return nil
		},
	}
}

func gib(b uint64) float64 { return float64(b) / (1024 * 1024 * 1024) }
