package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

func startJob(ctx context.Context, wg *sync.WaitGroup, name string, interval time.Duration, tick func(context.Context) error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("job stopped", "job", name)
				return
			case <-ticker.C:
				if err := tick(ctx); err != nil {
					slog.Error("job tick failed", "job", name, "error", err)
				}
			}
		}
	}()
}
