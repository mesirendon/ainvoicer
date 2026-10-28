package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Executor struct {
	Store       Store
	Gateway     Gateway
	BatchSize   int
	CallTimeout time.Duration
	MaxAttempts int
	Backoff     time.Duration
}

func (e *Executor) Tick(ctx context.Context) error {
	var errs []error
	for range e.BatchSize {
		if ctx.Err() != nil {
			break
		}
		due, err := e.Store.ClaimDuePayments(ctx, 1)
		if err != nil {
			return errors.Join(append(errs, fmt.Errorf("claim due payment: %w", err))...)
		}
		if len(due) == 0 {
			break
		}
		p := due[0]
		if err := e.execute(context.WithoutCancel(ctx), p); err != nil {
			errs = append(errs, fmt.Errorf("payment %s: %w", p.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (e *Executor) execute(ctx context.Context, p Payment) error {
	result, err := e.pay(ctx, p)
	var s Settlement
	switch {
	case err == nil && result.Status == ProviderSucceeded:
		s = Settlement{Status: StatusPaid, ProviderPaymentID: result.ProviderPaymentID}
	case err == nil && result.Status == ProviderDeclined:
		s = Settlement{Status: StatusFailed, ProviderPaymentID: result.ProviderPaymentID, Error: "declined by the destination bank"}
	default:
		reason := "unexpected provider status " + result.Status
		if err != nil {
			reason = err.Error()
		}
		found, lookupErr := lookup(ctx, e.Gateway, e.CallTimeout, p.ID)
		s = settlementFor(found, lookupErr, Settlement{
			Status: StatusFailed,
			Error:  "the provider has no record of the payment: " + reason,
		})
	}
	slog.InfoContext(ctx, "payment executed", "payment_id", p.ID, "destination_last4", last4(p.DestinationAccount), "status", s.Status)
	return e.Store.SettlePayment(ctx, p.ID, s)
}

func (e *Executor) pay(ctx context.Context, p Payment) (ProviderResult, error) {
	req := PayRequest{
		IdempotencyKey:     p.ID,
		DestinationAccount: p.DestinationAccount,
		AmountMinor:        p.AmountMinor,
		Currency:           p.Currency,
		Description:        "Invoice " + p.InvoiceNumber,
	}
	for attempt := 1; ; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, e.CallTimeout)
		result, err := e.Gateway.Pay(callCtx, req)
		cancel()
		if err == nil || !errors.Is(err, ErrTransient) || attempt >= e.MaxAttempts {
			return result, err
		}
		slog.WarnContext(ctx, "payment attempt failed, retrying", "payment_id", p.ID, "attempt", attempt, "error", err)
		time.Sleep(e.Backoff * time.Duration(attempt))
	}
}

func lookup(ctx context.Context, gw Gateway, timeout time.Duration, id string) (ProviderResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return gw.Lookup(callCtx, id)
}

func settlementFor(result ProviderResult, lookupErr error, onNotFound Settlement) Settlement {
	if lookupErr != nil {
		return Settlement{Status: StatusUnknown, Error: "payment status unknown, provider lookup failed: " + lookupErr.Error()}
	}
	switch result.Status {
	case ProviderSucceeded:
		return Settlement{Status: StatusPaid, ProviderPaymentID: result.ProviderPaymentID}
	case ProviderDeclined:
		return Settlement{Status: StatusFailed, ProviderPaymentID: result.ProviderPaymentID, Error: "declined by the destination bank"}
	case ProviderNotFound:
		return onNotFound
	default:
		return Settlement{Status: StatusUnknown, Error: "unexpected provider status " + result.Status}
	}
}
