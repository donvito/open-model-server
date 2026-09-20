package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/donvito/modelserver/internal/api"
	"github.com/donvito/modelserver/internal/app"
	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/web"
)

func newServeCommand(g *globalFlags) *cobra.Command {
	var headless, uiFlag bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the model server (web UI + APIs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := g.load(cmd)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("headless") && headless {
				cfg.Server.UI = false
			}
			if cmd.Flags().Changed("ui") {
				cfg.Server.UI = uiFlag
			}
			return runServe(cmd.Context(), cfg, g)
		},
	}
	cmd.Flags().BoolVar(&headless, "headless", false, "disable the web UI; keep the APIs")
	cmd.Flags().BoolVar(&uiFlag, "ui", true, "serve the web UI")
	return cmd
}

func runServe(parent context.Context, cfg config.Config, g *globalFlags) error {
	logger := newLogger(os.Stderr)
	a, err := app.New(cfg, logger)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Service.ResetStatuses(ctx); err != nil {
		return err
	}

	opts := api.Options{APIKeys: cfg.Auth.APIKeys, Logger: logger}
	uiMode := "headless"
	if cfg.Server.UI {
		if dist, ok := web.Dist(); ok {
			opts.UI = dist
			uiMode = "enabled"
		} else {
			uiMode = "unavailable (frontend not built; run `make web`)"
		}
	}

	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		a.Close(context.Background())
		return fmt.Errorf("listen on %s: %w", cfg.Addr(), err)
	}
	srv := &http.Server{Handler: api.New(a.Service, opts).Handler(), ReadHeaderTimeout: 30 * time.Second}

	logger.Info("modelserver starting", "addr", "http://"+ln.Addr().String(), "ui", uiMode, "auth", cfg.AuthEnabled(), "db", cfg.Database.Path)
	for _, rt := range a.Runtimes.All() {
		info := rt.Info(ctx)
		if info.Available {
			logger.Info("runtime available", "runtime", info.Name, "version", info.Version)
		} else {
			logger.Warn("runtime unavailable", "runtime", info.Name, "reason", info.Error)
		}
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		logger.Info("shutting down: stopping models and closing server")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.Close(context.Background())
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	a.Close(shutdownCtx)
	logger.Info("bye")
	return nil
}
