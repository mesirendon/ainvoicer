package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"ap-agent/internal/config"
	"ap-agent/internal/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(context.Background()); err != nil {
		slog.Error("seed failed", "error", err)
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
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return seed(ctx, tx)
	})
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "seed complete")
	return nil
}
