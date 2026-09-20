// Package app wires configuration, storage, runtimes and the service layer.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/donvito/modelserver/internal/config"
	"github.com/donvito/modelserver/internal/database"
	"github.com/donvito/modelserver/internal/logs"
	"github.com/donvito/modelserver/internal/process"
	"github.com/donvito/modelserver/internal/registry"
	rt "github.com/donvito/modelserver/internal/runtime"
	"github.com/donvito/modelserver/internal/runtime/llamacpp"
	"github.com/donvito/modelserver/internal/runtime/onnx"
	"github.com/donvito/modelserver/internal/service"
)

type App struct {
	Config   config.Config
	DB       *sql.DB
	Service  *service.Service
	Runtimes *rt.Registry
	Logger   *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	logStore := logs.NewStore(cfg.Logs.LinesPerModel)
	ports := process.NewPortAllocator(cfg.LlamaCpp.PortRangeStart, cfg.LlamaCpp.PortRangeEnd)
	llama := llamacpp.New(llamacpp.Options{
		Binary:         cfg.LlamaCpp.Binary,
		StartupTimeout: time.Duration(cfg.LlamaCpp.StartupTimeoutSeconds) * time.Second,
	}, process.ExecRunner{}, ports, logStore)
	ort := onnx.New(onnx.Options{Library: cfg.ONNX.Library}, logStore)

	runtimes := rt.NewRegistry(llama, ort)
	svc := service.New(cfg, registry.New(db), runtimes, logStore)
	return &App{Config: cfg, DB: db, Service: svc, Runtimes: runtimes, Logger: logger}, nil
}

// Close shuts down runtimes and the database.
func (a *App) Close(ctx context.Context) {
	a.Service.Shutdown(ctx)
	a.DB.Close()
}
