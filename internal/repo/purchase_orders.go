package repo

import (
	"context"
	"fmt"
	"time"

	"ap-agent/internal/db"
	"ap-agent/internal/invoices"
)

func (r *Repo) ListPurchaseOrders(ctx context.Context) ([]invoices.PurchaseOrder, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM purchase_orders ORDER BY po_number`)
	if err != nil {
		return nil, fmt.Errorf("list purchase orders: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan purchase order id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list purchase orders: %w", err)
	}
	pos := make([]invoices.PurchaseOrder, 0, len(ids))
	for _, id := range ids {
		po, err := loadPurchaseOrder(ctx, r.pool, id)
		if err != nil {
			return nil, err
		}
		pos = append(pos, po)
	}
	return pos, nil
}

func loadPurchaseOrder(ctx context.Context, q db.Querier, id string) (invoices.PurchaseOrder, error) {
	var po invoices.PurchaseOrder
	err := q.QueryRow(ctx, `
		SELECT po.id, po.po_number, po.supplier_id, s.name, po.currency, po.status
		FROM purchase_orders po
		JOIN suppliers s ON s.id = po.supplier_id
		WHERE po.id = $1`, id).Scan(&po.ID, &po.Number, &po.SupplierID, &po.SupplierName, &po.Currency, &po.Status)
	if err != nil {
		return invoices.PurchaseOrder{}, fmt.Errorf("load purchase order: %w", err)
	}

	rows, err := q.Query(ctx, `
		SELECT line_no, sku, description, quantity, unit_price_minor
		FROM po_lines WHERE po_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return invoices.PurchaseOrder{}, fmt.Errorf("load po lines: %w", err)
	}
	defer rows.Close()
	po.Lines = []invoices.POLine{}
	for rows.Next() {
		var l invoices.POLine
		if err := rows.Scan(&l.LineNo, &l.SKU, &l.Description, &l.Quantity, &l.UnitPriceMinor); err != nil {
			return invoices.PurchaseOrder{}, fmt.Errorf("scan po line: %w", err)
		}
		po.Lines = append(po.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return invoices.PurchaseOrder{}, fmt.Errorf("load po lines: %w", err)
	}

	if po.Receipts, err = loadReceipts(ctx, q, id); err != nil {
		return invoices.PurchaseOrder{}, err
	}
	return po, nil
}

func loadReceipts(ctx context.Context, q db.Querier, poID string) ([]invoices.Receipt, error) {
	rows, err := q.Query(ctx, `
		SELECT gr.id, w.name, w.city, w.country, gr.received_on, rl.sku, rl.quantity_received
		FROM goods_receipts gr
		JOIN warehouses w ON w.id = gr.warehouse_id
		JOIN receipt_lines rl ON rl.receipt_id = gr.id
		WHERE gr.po_id = $1
		ORDER BY gr.received_on, gr.id, rl.sku`, poID)
	if err != nil {
		return nil, fmt.Errorf("load receipts: %w", err)
	}
	defer rows.Close()
	receipts := []invoices.Receipt{}
	for rows.Next() {
		var rc invoices.Receipt
		var receivedOn time.Time
		var line invoices.ReceiptLine
		if err := rows.Scan(&rc.ID, &rc.WarehouseName, &rc.WarehouseCity, &rc.WarehouseCountry, &receivedOn, &line.SKU, &line.Quantity); err != nil {
			return nil, fmt.Errorf("scan receipt line: %w", err)
		}
		if len(receipts) == 0 || receipts[len(receipts)-1].ID != rc.ID {
			rc.ReceivedOn = receivedOn.Format(time.DateOnly)
			receipts = append(receipts, rc)
		}
		last := &receipts[len(receipts)-1]
		last.Lines = append(last.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load receipts: %w", err)
	}
	return receipts, nil
}
