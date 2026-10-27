package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func seed(ctx context.Context, tx pgx.Tx) error {
	for _, s := range suppliers {
		_, err := tx.Exec(ctx, `
			INSERT INTO suppliers (id, name, tax_id, country, currency, bank_account, payment_net_days, early_discount_bps, early_discount_days)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO NOTHING`,
			s.id, s.name, s.taxID, s.country, s.currency, s.bankAccount, s.netDays, s.discountBps, s.discountDays)
		if err != nil {
			return fmt.Errorf("insert supplier %s: %w", s.id, err)
		}
	}
	for _, w := range warehouses {
		_, err := tx.Exec(ctx, `
			INSERT INTO warehouses (id, supplier_id, name, city, country, location)
			VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($6, $7), 4326)::geography)
			ON CONFLICT (id) DO NOTHING`,
			w.id, w.supplierID, w.name, w.city, w.country, w.lng, w.lat)
		if err != nil {
			return fmt.Errorf("insert warehouse %s: %w", w.id, err)
		}
	}
	for _, po := range purchaseOrders {
		_, err := tx.Exec(ctx, `
			INSERT INTO purchase_orders (id, po_number, supplier_id, currency, status)
			VALUES ($1, $2, $3, $4, 'OPEN')
			ON CONFLICT (id) DO NOTHING`,
			po.id, po.number, po.supplierID, po.currency)
		if err != nil {
			return fmt.Errorf("insert purchase order %s: %w", po.number, err)
		}
		for _, l := range po.lines {
			_, err := tx.Exec(ctx, `
				INSERT INTO po_lines (po_id, line_no, sku, description, quantity, unit_price_minor)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (po_id, line_no) DO NOTHING`,
				po.id, l.lineNo, l.sku, l.description, l.quantity, l.unitPriceMinor)
			if err != nil {
				return fmt.Errorf("insert po line %s/%d: %w", po.number, l.lineNo, err)
			}
		}
	}
	for _, r := range receipts {
		_, err := tx.Exec(ctx, `
			INSERT INTO goods_receipts (id, po_id, warehouse_id, received_on)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO NOTHING`,
			r.id, r.poID, r.warehouseID, r.receivedOn)
		if err != nil {
			return fmt.Errorf("insert receipt %s: %w", r.id, err)
		}
		for _, l := range r.lines {
			_, err := tx.Exec(ctx, `
				INSERT INTO receipt_lines (receipt_id, sku, quantity_received)
				VALUES ($1, $2, $3)
				ON CONFLICT (receipt_id, sku) DO NOTHING`,
				r.id, l.sku, l.quantity)
			if err != nil {
				return fmt.Errorf("insert receipt line %s/%s: %w", r.id, l.sku, err)
			}
		}
	}
	for _, f := range fxRates {
		_, err := tx.Exec(ctx, `
			INSERT INTO fx_rates (base, quote, rate_e8, spread_bps)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (base, quote) DO NOTHING`,
			f.base, f.quote, f.rateE8, f.spreadBps)
		if err != nil {
			return fmt.Errorf("insert fx rate %s/%s: %w", f.base, f.quote, err)
		}
	}
	return nil
}
