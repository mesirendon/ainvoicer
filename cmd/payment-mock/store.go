package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ap-agent/internal/db"
)

type payment struct {
	Key                string
	DestinationAccount string
	AmountMinor        int64
	Currency           string
	Status             string
}

var errNotFound = errors.New("payment not found")

func ensureSchema(ctx context.Context, q db.Querier) error {
	if _, err := q.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS payment_mock"); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	_, err := q.Exec(ctx, `CREATE TABLE IF NOT EXISTS payment_mock.payments (
		key text PRIMARY KEY,
		destination_account text NOT NULL,
		amount_minor bigint NOT NULL,
		currency text NOT NULL,
		status text NOT NULL CHECK (status IN ('SUCCEEDED', 'DECLINED')),
		created_at timestamptz NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return fmt.Errorf("create payments table: %w", err)
	}
	return nil
}

func recordPayment(ctx context.Context, q db.Querier, p payment) (payment, error) {
	_, err := q.Exec(ctx, `
		INSERT INTO payment_mock.payments (key, destination_account, amount_minor, currency, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key) DO NOTHING`,
		p.Key, p.DestinationAccount, p.AmountMinor, p.Currency, p.Status)
	if err != nil {
		return payment{}, fmt.Errorf("insert payment: %w", err)
	}
	return findPayment(ctx, q, p.Key)
}

func findPayment(ctx context.Context, q db.Querier, key string) (payment, error) {
	var p payment
	err := q.QueryRow(ctx, `
		SELECT key, destination_account, amount_minor, currency, status
		FROM payment_mock.payments
		WHERE key = $1`, key).
		Scan(&p.Key, &p.DestinationAccount, &p.AmountMinor, &p.Currency, &p.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment{}, errNotFound
	}
	if err != nil {
		return payment{}, fmt.Errorf("select payment: %w", err)
	}
	return p, nil
}
