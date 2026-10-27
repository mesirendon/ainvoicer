package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ap-agent/internal/config"
	"ap-agent/internal/db"
	"ap-agent/internal/healthcheck"
	"ap-agent/internal/httpapi"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck.Probe(cfg.APIPort); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(cfg); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)

	pool, err := db.Connect(ctx, cfg.DatabaseURL, &db.Limits{
		LockTimeoutMS:      cfg.DBLockTimeoutMS,
		StatementTimeoutMS: cfg.DBStatementTimeoutMS,
		IdleInTxTimeoutMS:  cfg.DBIdleInTxTimeoutMS,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           httpapi.NewRouter(pool),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("api starting", "fixture_mode", cfg.FixtureMode())
	return httpapi.Serve(ctx, server)
}
