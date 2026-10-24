package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/valpere/slovnyk/internal/ops"
	"github.com/valpere/slovnyk/internal/storage/postgres"
)

func main() {
	os.Exit(mainCode())
}

func mainCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	output := io.Writer(os.Stderr)
	if len(os.Args) > 1 && os.Args[1] == "import" {
		output = os.Stdout
	}
	return run(ctx, os.Args[1:], os.Getenv, output)
}

func run(ctx context.Context, args []string, getenv func(string) string, output io.Writer) int {
	if len(args) > 0 && args[0] == "import" {
		return runImport(ctx, args[1:], getenv, output)
	}
	logger := slog.New(slog.NewJSONHandler(output, nil))
	if len(args) == 1 && args[0] == "migrate" {
		if getenv("SLOVNYK_MIGRATION_DSN") == "" {
			logger.Error("SLOVNYK_MIGRATION_DSN is required", "event", "invalid_config")
			return 2
		}
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		conn, err := postgres.Connect(ctx, getenv("SLOVNYK_MIGRATION_DSN"))
		if err != nil {
			logger.Error("migration connection failed", "event", "migration_failed")
			if errors.Is(err, postgres.ErrConfig) {
				return 2
			}
			return 1
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = conn.Close(cleanup)
		}()
		if err := postgres.Migrate(ctx, conn); err != nil {
			logger.Error("migration failed", "event", "migration_failed")
			return 1
		}
		logger.Info("schema migrated", "event", "migration_completed", "schema_version", postgres.SchemaVersion)
		return 0
	}
	if len(args) != 1 || args[0] != "serve" {
		logger.Error("usage: slovnyk serve|migrate|import", "event", "invalid_command")
		return 2
	}
	cfg, err := ops.LoadConfig(getenv)
	if err != nil {
		logger.Error("configuration rejected", "event", "invalid_config", "reason", err.Error())
		return 2
	}
	logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := ops.Run(ctx, cfg, logger); err != nil {
		logger.Error("HTTP process failed", "event", "http_failed", "reason", err.Error())
		return 1
	}
	return 0
}
