package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ap-agent/internal/db"
	"ap-agent/internal/invoices"
	"ap-agent/internal/payments"
)

var invoiceStatusFor = map[string]string{
	payments.StatusScheduled: invoices.StatusPaymentScheduled,
	payments.StatusPaid:      "PAID",
	payments.StatusFailed:    "PAYMENT_FAILED",
	payments.StatusUnknown:   "PAYMENT_UNKNOWN",
}

func (r *Repo) ClaimDuePayments(ctx context.Context, limit int) ([]payments.Payment, error) {
	var claimed []payments.Payment
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		today, err := demoToday(ctx, tx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, status FROM payments
			WHERE status = 'SCHEDULED' AND scheduled_for <= $1
			ORDER BY scheduled_for, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED`, today, limit)
		if err != nil {
			return fmt.Errorf("select due payments: %w", err)
		}
		claimed, err = claim(ctx, tx, rows, true)
		return err
	})
	return claimed, err
}

func (r *Repo) ClaimStalePayments(ctx context.Context, limit int) ([]payments.Payment, error) {
	var claimed []payments.Payment
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, status FROM payments
			WHERE status IN ('UNKNOWN', 'PROCESSING')
			  AND updated_at < now() - make_interval(secs => $1)
			ORDER BY updated_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED`, r.leaseSeconds, limit)
		if err != nil {
			return fmt.Errorf("select stale payments: %w", err)
		}
		claimed, err = claim(ctx, tx, rows, false)
		return err
	})
	return claimed, err
}

func claim(ctx context.Context, tx pgx.Tx, rows pgx.Rows, audit bool) ([]payments.Payment, error) {
	defer rows.Close()
	previous := map[string]string{}
	var ids []string
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, fmt.Errorf("scan claimed payment: %w", err)
		}
		previous[id] = status
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select payments to claim: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	details, err := tx.Query(ctx, `
		UPDATE payments p
		SET status = 'PROCESSING', updated_at = now()
		FROM invoices i
		JOIN suppliers s ON s.id = i.supplier_id
		WHERE p.id = ANY($1) AND i.id = p.invoice_id
		RETURNING p.id, p.invoice_id, i.invoice_number, s.bank_account, p.amount_minor, p.currency`, ids)
	if err != nil {
		return nil, fmt.Errorf("mark payments processing: %w", err)
	}
	defer details.Close()
	var claimed []payments.Payment
	for details.Next() {
		var p payments.Payment
		if err := details.Scan(&p.ID, &p.InvoiceID, &p.InvoiceNumber, &p.DestinationAccount, &p.AmountMinor, &p.Currency); err != nil {
			return nil, fmt.Errorf("scan claimed payment: %w", err)
		}
		p.PreviousStatus = previous[p.ID]
		claimed = append(claimed, p)
	}
	if err := details.Err(); err != nil {
		return nil, fmt.Errorf("mark payments processing: %w", err)
	}
	if !audit {
		return claimed, nil
	}
	for _, p := range claimed {
		if err := insertAudit(ctx, tx, p.InvoiceID, "agent", "payment_processing", map[string]string{}); err != nil {
			return nil, err
		}
	}
	return claimed, nil
}

func (r *Repo) SettlePayment(ctx context.Context, id string, s payments.Settlement) error {
	invoiceStatus, ok := invoiceStatusFor[s.Status]
	if !ok {
		return fmt.Errorf("cannot settle payment as %q", s.Status)
	}
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var invoiceID string
		err := tx.QueryRow(ctx, `
			UPDATE payments
			SET status = $2,
			    processor_payment_id = COALESCE(NULLIF($3, ''), processor_payment_id),
			    error = $4,
			    updated_at = now()
			WHERE id = $1 AND status = 'PROCESSING'
			RETURNING invoice_id`,
			id, s.Status, s.ProviderPaymentID, s.Error).Scan(&invoiceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: payment %s is no longer PROCESSING", invoices.ErrConflict, id)
		}
		if err != nil {
			return fmt.Errorf("settle payment: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE invoices SET status = $2, updated_at = now()
			WHERE id = $1 AND status IN ('PAYMENT_SCHEDULED', 'PAYMENT_UNKNOWN')`,
			invoiceID, invoiceStatus)
		if err != nil {
			return fmt.Errorf("update invoice after payment: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: invoice %s is not waiting for this payment", invoices.ErrConflict, invoiceID)
		}
		return insertAudit(ctx, tx, invoiceID, "agent", "payment_"+strings.ToLower(s.Status), map[string]string{
			"providerPaymentId": s.ProviderPaymentID,
			"error":             s.Error,
		})
	})
}

func (r *Repo) ListPayments(ctx context.Context) ([]payments.Summary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.invoice_id, i.invoice_number, s.name, right(s.bank_account, 4), p.status,
		       p.amount_minor, p.discount_minor, p.currency, p.scheduled_for,
		       p.funding_currency, p.fx_rate_e8, p.fx_spread_bps, p.funding_amount_minor,
		       p.processor_payment_id, p.error, p.updated_at
		FROM payments p
		JOIN invoices i ON i.id = p.invoice_id
		JOIN suppliers s ON s.id = i.supplier_id
		ORDER BY p.scheduled_for, i.invoice_number`)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()
	list := []payments.Summary{}
	for rows.Next() {
		var p payments.Summary
		var scheduledFor time.Time
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.InvoiceNumber, &p.SupplierName, &p.DestinationLast4, &p.Status,
			&p.AmountMinor, &p.DiscountMinor, &p.Currency, &scheduledFor,
			&p.FundingCurrency, &p.FxRateE8, &p.FxSpreadBps, &p.FundingAmountMinor,
			&p.ProcessorPaymentID, &p.Error, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		p.ScheduledFor = scheduledFor.Format(time.DateOnly)
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	return list, nil
}
