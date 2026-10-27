package main

import (
	"context"
	"log/slog"
	"os"

	"ap-agent/internal/config"
	"ap-agent/internal/db"
	"ap-agent/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(context.Background()); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL, nil)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, migrations.Files); err != nil {
		return err
	}
	slog.InfoContext(ctx, "migrations up to date")
	return nil
}
