package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ap-agent/internal/clock"
	"ap-agent/internal/config"
	"ap-agent/internal/db"
	"ap-agent/internal/geocode"
	"ap-agent/internal/healthcheck"
	"ap-agent/internal/httpapi"
	"ap-agent/internal/invoices"
	"ap-agent/internal/matching"
	"ap-agent/internal/payments"
	"ap-agent/internal/repo"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "extract-check" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: api extract-check <file.pdf>")
			os.Exit(2)
		}
		if err := extractCheck(cfg, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
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

	store := repo.New(pool, cfg.LeaseSeconds, cfg.LeaseSeconds+cfg.ExtractTimeoutMS/1000)
	service := &invoices.Service{
		Store:     store,
		Extractor: newExtractor(cfg),
		Geocoder:  geocode.TableGeocoder{},
		Rules: invoices.Rules{
			Tolerances:       matching.Tolerances{PriceBps: cfg.PriceToleranceBps, OriginKM: cfg.OriginToleranceKM},
			AutoApproveClean: cfg.AutoApproveClean,
			FundingCurrency:  cfg.BuyerBaseCurrency,
		},
		ExtractTimeout: time.Duration(cfg.ExtractTimeoutMS) * time.Millisecond,
		GeocodeTimeout: time.Duration(cfg.GeocoderTimeoutMS) * time.Millisecond,
		SamplesDir:     cfg.SamplesDir,
	}
	gateway := &payments.HTTPGateway{BaseURL: cfg.PaymentMockURL, Client: &http.Client{}}
	executor := &payments.Executor{
		Store:       store,
		Gateway:     gateway,
		BatchSize:   10,
		CallTimeout: 3 * time.Second,
		MaxAttempts: 3,
		Backoff:     500 * time.Millisecond,
	}
	reconciler := &payments.Reconciler{Store: store, Gateway: gateway, BatchSize: 10, CallTimeout: 3 * time.Second}

	router := httpapi.NewRouter(httpapi.Deps{
		Health:         pool,
		Invoices:       service,
		Samples:        service,
		PurchaseOrders: store,
		Payments:       store,
		Executor:       executor,
		Demo:           &clock.Service{Store: store},
		Meta: httpapi.Meta{
			FixtureMode:     cfg.FixtureMode(),
			Model:           cfg.AnthropicModel,
			BuyerName:       cfg.BuyerName,
			BuyerCountry:    cfg.BuyerCountry,
			FundingCurrency: cfg.BuyerBaseCurrency,
		},
		MaxUploadBytes: int64(cfg.ExtractMaxFileMB) << 20,
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	var jobs sync.WaitGroup
	startJob(ctx, &jobs, "executor", time.Duration(cfg.ExecuteIntervalMS)*time.Millisecond, executor.Tick)
	startJob(ctx, &jobs, "reconciler", time.Duration(cfg.ReconcileIntervalMS)*time.Millisecond, reconciler.Tick)

	slog.Info("api starting", "fixture_mode", cfg.FixtureMode())
	serveErr := httpapi.Serve(ctx, server)
	stop()
	jobs.Wait()
	slog.Info("background jobs stopped")
	return serveErr
}
