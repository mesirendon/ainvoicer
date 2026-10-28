package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Reconciler struct {
	Store       Store
	Gateway     Gateway
	BatchSize   int
	CallTimeout time.Duration
}

func (r *Reconciler) Tick(ctx context.Context) error {
	stale, err := r.Store.ClaimStalePayments(ctx, r.BatchSize)
	if err != nil {
		return fmt.Errorf("claim stale payments: %w", err)
	}
	var errs []error
	for _, p := range stale {
		onNotFound := Settlement{Status: StatusFailed, Error: "the provider has no record of the payment"}
		if p.PreviousStatus == StatusProcessing {
			onNotFound = Settlement{Status: StatusScheduled}
		}
		found, lookupErr := lookup(context.WithoutCancel(ctx), r.Gateway, r.CallTimeout, p.ID)
		s := settlementFor(found, lookupErr, onNotFound)
		slog.InfoContext(ctx, "payment reconciled", "payment_id", p.ID, "was", p.PreviousStatus, "now", s.Status)
		if err := r.Store.SettlePayment(context.WithoutCancel(ctx), p.ID, s); err != nil {
			errs = append(errs, fmt.Errorf("payment %s: %w", p.ID, err))
		}
	}
	return errors.Join(errs...)
}
