package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ap-agent/internal/db"
)

type Repo struct {
	pool                   *pgxpool.Pool
	leaseSeconds           int
	reclaimReceivedSeconds int
}

func New(pool *pgxpool.Pool, leaseSeconds, reclaimReceivedSeconds int) *Repo {
	return &Repo{pool: pool, leaseSeconds: leaseSeconds, reclaimReceivedSeconds: reclaimReceivedSeconds}
}

func insertAudit(ctx context.Context, q db.Querier, invoiceID, actor, action string, detail any) error {
	body, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO audit_log (invoice_id, actor, action, detail)
		VALUES ($1, $2, $3, $4)`,
		invoiceID, actor, action, body)
	if err != nil {
		return fmt.Errorf("insert audit %s: %w", action, err)
	}
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableDate(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nullableInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
