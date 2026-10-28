package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ap-agent/internal/clock"
	"ap-agent/internal/db"
)

func (r *Repo) DemoClock(ctx context.Context) (clock.Clock, error) {
	return readClock(ctx, r.pool)
}

func (r *Repo) AdvanceDemoClock(ctx context.Context, days int) (clock.Clock, error) {
	var c clock.Clock
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE demo_clock SET offset_days = offset_days + $1 WHERE id = 1`, days); err != nil {
			return fmt.Errorf("advance demo clock: %w", err)
		}
		var err error
		c, err = readClock(ctx, tx)
		return err
	})
	return c, err
}

func (r *Repo) ResetDemo(ctx context.Context) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `TRUNCATE audit_log, findings, invoice_lines, payments, invoices`); err != nil {
			return fmt.Errorf("truncate demo data: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE demo_clock SET offset_days = 0 WHERE id = 1`); err != nil {
			return fmt.Errorf("reset demo clock: %w", err)
		}
		return nil
	})
}

func readClock(ctx context.Context, q db.Querier) (clock.Clock, error) {
	var c clock.Clock
	if err := q.QueryRow(ctx, `SELECT offset_days FROM demo_clock WHERE id = 1`).Scan(&c.OffsetDays); err != nil {
		return clock.Clock{}, fmt.Errorf("read demo clock: %w", err)
	}
	today, err := demoToday(ctx, q)
	if err != nil {
		return clock.Clock{}, err
	}
	c.Today = today.Format(time.DateOnly)
	return c, nil
}
