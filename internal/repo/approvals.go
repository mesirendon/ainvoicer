package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ap-agent/internal/db"
	"ap-agent/internal/fx"
	"ap-agent/internal/invoices"
)

var (
	approvable  = []string{"MATCHED", "MATCHED_WITH_WARNINGS"}
	overridable = []string{"BLOCKED"}
	rejectable  = []string{"MATCHED", "MATCHED_WITH_WARNINGS", "BLOCKED", "EXTRACTION_FAILED"}
)

func (r *Repo) Approve(ctx context.Context, id, actor string, rules invoices.Rules) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := transition(ctx, tx, id, approvable, invoices.StatusApproved); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, id, actor, "approved", map[string]string{}); err != nil {
			return err
		}
		plan, err := planPayment(ctx, tx, id, rules)
		if err != nil {
			return err
		}
		return schedulePayment(ctx, tx, id, plan)
	})
}

func (r *Repo) Override(ctx context.Context, id, actor, reason string, rules invoices.Rules) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var duplicate bool
		err := tx.QueryRow(ctx, `SELECT duplicate_of IS NOT NULL FROM invoices WHERE id = $1 FOR UPDATE`, id).Scan(&duplicate)
		if errors.Is(err, pgx.ErrNoRows) {
			return invoices.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock invoice: %w", err)
		}
		if duplicate {
			return fmt.Errorf("%w: a duplicate invoice can only be rejected, overriding it would pay the supplier twice", invoices.ErrConflict)
		}
		if err := transition(ctx, tx, id, overridable, invoices.StatusApproved); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, id, actor, "overridden", map[string]string{"reason": reason}); err != nil {
			return err
		}
		plan, err := planPayment(ctx, tx, id, rules)
		if err != nil {
			return err
		}
		return schedulePayment(ctx, tx, id, plan)
	})
}

func (r *Repo) Reject(ctx context.Context, id, actor, reason string) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := transition(ctx, tx, id, rejectable, invoices.StatusRejected); err != nil {
			return err
		}
		return insertAudit(ctx, tx, id, actor, "rejected", map[string]string{"reason": reason})
	})
}

func transition(ctx context.Context, q db.Querier, id string, from []string, to string) error {
	tag, err := q.Exec(ctx, `
		UPDATE invoices SET status = $3, updated_at = now()
		WHERE id = $1 AND status = ANY($2)`,
		id, from, to)
	if err != nil {
		return fmt.Errorf("move invoice to %s: %w", to, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var current string
	err = q.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read invoice status: %w", err)
	}
	return fmt.Errorf("%w: invoice is %s, expected one of %s", invoices.ErrConflict, current, strings.Join(from, ", "))
}

func planPayment(ctx context.Context, q db.Querier, id string, rules invoices.Rules) (invoices.PaymentPlan, error) {
	var in invoices.PaymentInput
	var issue, due *time.Time
	err := q.QueryRow(ctx, `
		SELECT i.total_minor, i.currency, i.issue_date, i.due_date,
		       s.payment_net_days, s.early_discount_bps, s.early_discount_days
		FROM invoices i
		JOIN suppliers s ON s.id = i.supplier_id
		WHERE i.id = $1`, id).Scan(
		&in.InvoiceAmountMinor, &in.Currency, &issue, &due,
		&in.Terms.NetDays, &in.Terms.EarlyDiscountBps, &in.Terms.EarlyDiscountDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.PaymentPlan{}, fmt.Errorf("%w: the invoice has no known supplier to pay", invoices.ErrConflict)
	}
	if err != nil {
		return invoices.PaymentPlan{}, fmt.Errorf("load payment inputs: %w", err)
	}
	if in.Today, err = demoToday(ctx, q); err != nil {
		return invoices.PaymentPlan{}, err
	}
	if issue != nil {
		in.IssueDate = *issue
	}
	if due != nil {
		in.DueDate = *due
	}

	var rate fx.Rate
	err = q.QueryRow(ctx, `
		SELECT base, quote, rate_e8, spread_bps FROM fx_rates
		WHERE base = $1 AND quote = $2`,
		in.Currency, rules.FundingCurrency).Scan(&rate.Base, &rate.Quote, &rate.RateE8, &rate.SpreadBps)
	switch {
	case err == nil:
		in.Rate = &rate
	case !errors.Is(err, pgx.ErrNoRows):
		return invoices.PaymentPlan{}, fmt.Errorf("load fx rate: %w", err)
	}
	return rules.PlanPayment(in)
}

func demoToday(ctx context.Context, q db.Querier) (time.Time, error) {
	var today time.Time
	err := q.QueryRow(ctx, `
		SELECT (now() AT TIME ZONE 'UTC')::date + offset_days
		FROM demo_clock WHERE id = 1`).Scan(&today)
	if err != nil {
		return time.Time{}, fmt.Errorf("read demo clock: %w", err)
	}
	return today, nil
}

func schedulePayment(ctx context.Context, q db.Querier, id string, plan invoices.PaymentPlan) error {
	s, c := plan.Schedule, plan.Conversion
	_, err := q.Exec(ctx, `
		INSERT INTO payments (id, invoice_id, status, amount_minor, currency, discount_minor, scheduled_for,
		                      funding_currency, fx_rate_e8, fx_spread_bps, funding_amount_minor)
		VALUES ($1, $2, 'SCHEDULED', $3, $4, $5, $6, $7, $8, $9, $10)`,
		uuid.NewString(), id, s.AmountMinor, plan.Currency, s.DiscountMinor, s.PayOn,
		c.FundingCurrency, c.RateE8, c.SpreadBps, c.FundingAmountMinor)
	if err != nil {
		return fmt.Errorf("insert payment: %w", err)
	}
	if err := transition(ctx, q, id, []string{invoices.StatusApproved}, invoices.StatusPaymentScheduled); err != nil {
		return err
	}
	return insertAudit(ctx, q, id, "agent", "payment_scheduled", map[string]any{
		"scheduledFor":       s.PayOn.Format(time.DateOnly),
		"amountMinor":        s.AmountMinor,
		"discountMinor":      s.DiscountMinor,
		"reason":             s.Reason,
		"fundingCurrency":    c.FundingCurrency,
		"fxRateE8":           c.RateE8,
		"fxSpreadBps":        c.SpreadBps,
		"fundingAmountMinor": c.FundingAmountMinor,
	})
}
