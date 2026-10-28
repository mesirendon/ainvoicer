package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ap-agent/internal/db"
	"ap-agent/internal/extraction"
	"ap-agent/internal/geocode"
	"ap-agent/internal/invoices"
	"ap-agent/internal/matching"
)

var errNotReceived = fmt.Errorf("%w: invoice is no longer in RECEIVED status", invoices.ErrConflict)

func (r *Repo) SaveReceivedInvoice(ctx context.Context, f invoices.NewFile) (invoices.Received, error) {
	var rec invoices.Received
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO invoices (id, file_name, file_sha256, file, status)
			VALUES ($1, $2, $3, $4, 'RECEIVED')
			ON CONFLICT (file_sha256) DO NOTHING
			RETURNING id`,
			uuid.NewString(), f.FileName, f.SHA256, f.Content).Scan(&rec.ID)
		if err == nil {
			rec.Created, rec.Claimed = true, true
			return insertAudit(ctx, tx, rec.ID, "system", "received", map[string]string{"fileName": f.FileName, "sha256": f.SHA256})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("insert invoice: %w", err)
		}

		err = tx.QueryRow(ctx, `
			UPDATE invoices
			SET status = 'RECEIVED', extraction_error = '', updated_at = now()
			WHERE file_sha256 = $1
			  AND (status = 'EXTRACTION_FAILED'
			       OR (status = 'RECEIVED' AND updated_at < now() - make_interval(secs => $2)))
			RETURNING id`,
			f.SHA256, r.reclaimReceivedSeconds).Scan(&rec.ID)
		if err == nil {
			rec.Claimed = true
			return insertAudit(ctx, tx, rec.ID, "system", "reprocessing", map[string]string{"fileName": f.FileName})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reclaim invoice: %w", err)
		}

		if err := tx.QueryRow(ctx, `SELECT id FROM invoices WHERE file_sha256 = $1`, f.SHA256).Scan(&rec.ID); err != nil {
			return fmt.Errorf("find existing invoice: %w", err)
		}
		return nil
	})
	return rec, err
}

func (r *Repo) MarkExtractionFailed(ctx context.Context, id, reason string) error {
	return db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE invoices
			SET status = 'EXTRACTION_FAILED', extraction_error = $2, updated_at = now()
			WHERE id = $1 AND status = 'RECEIVED'`,
			id, reason)
		if err != nil {
			return fmt.Errorf("mark extraction failed: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errNotReceived
		}
		return insertAudit(ctx, tx, id, "agent", "extraction_failed", map[string]string{"error": reason})
	})
}

func (r *Repo) ApplyVerdict(ctx context.Context, id string, inv extraction.ExtractedInvoice, shipFrom *geocode.Point,
	rules invoices.Rules,
) (string, error) {
	var status string
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
			return fmt.Errorf("lock invoice: %w", err)
		}
		if current != invoices.StatusReceived {
			return errNotReceived
		}

		supplier, err := lockSupplier(ctx, tx, inv.SupplierTaxID, inv.SupplierName)
		if err != nil {
			return err
		}
		po, err := lockPurchaseOrder(ctx, tx, inv.PONumber)
		if err != nil {
			return err
		}
		duplicateOf, err := findDuplicate(ctx, tx, supplier, inv.InvoiceNumber, id)
		if err != nil {
			return err
		}
		if err := writeHeader(ctx, tx, id, inv, supplier, po, duplicateOf, shipFrom); err != nil {
			return err
		}

		in := matching.Input{
			Invoice:     toMatchingInvoice(inv),
			Supplier:    supplier,
			PO:          po,
			DuplicateOf: duplicateOf,
		}
		if po != nil {
			if in.Receipts, err = loadReceiptsForMatching(ctx, tx, po.ID); err != nil {
				return err
			}
			if in.PreviouslyInvoiced, err = previouslyInvoiced(ctx, tx, po.ID, id); err != nil {
				return err
			}
			if in.Nearest, err = nearestReceivingWarehouse(ctx, tx, id, po.ID); err != nil {
				return err
			}
		}

		decision := rules.Decide(in)
		result := decision.Result
		status = string(result.Verdict)
		if err := insertLines(ctx, tx, id, inv.Lines, result.Lines); err != nil {
			return err
		}
		if err := insertFindings(ctx, tx, id, result.Findings); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE invoices SET status = $2, updated_at = now() WHERE id = $1`, id, status); err != nil {
			return fmt.Errorf("set verdict: %w", err)
		}
		if err := insertAudit(ctx, tx, id, "agent", "matched", map[string]any{"verdict": result.Verdict, "findings": len(result.Findings)}); err != nil {
			return err
		}
		if !decision.AutoApprove {
			return nil
		}

		plan, err := planPayment(ctx, tx, id, rules)
		if errors.Is(err, invoices.ErrCannotSchedule) {
			return insertAudit(ctx, tx, id, "agent", "auto_approval_skipped", map[string]string{"error": err.Error()})
		}
		if err != nil {
			return err
		}
		if err := transition(ctx, tx, id, []string{string(matching.VerdictMatched)}, invoices.StatusApproved); err != nil {
			return err
		}
		if err := insertAudit(ctx, tx, id, "agent", "approved", map[string]string{"reason": "clean 3-way match, auto-approved"}); err != nil {
			return err
		}
		status = invoices.StatusPaymentScheduled
		return schedulePayment(ctx, tx, id, plan)
	})
	return status, err
}

func lockSupplier(ctx context.Context, q db.Querier, taxID, name string) (*matching.Supplier, error) {
	var s matching.Supplier
	err := q.QueryRow(ctx, `
		SELECT id, name FROM suppliers
		WHERE ($1 <> '' AND tax_id = $1) OR lower(name) = lower($2)
		ORDER BY (tax_id = $1) DESC
		LIMIT 1
		FOR UPDATE`,
		taxID, name).Scan(&s.ID, &s.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock supplier: %w", err)
	}
	return &s, nil
}

func lockPurchaseOrder(ctx context.Context, q db.Querier, number string) (*matching.PO, error) {
	if number == "" {
		return nil, nil
	}
	var po matching.PO
	err := q.QueryRow(ctx, `
		SELECT po.id, po.po_number, po.supplier_id, s.name, po.currency
		FROM purchase_orders po
		JOIN suppliers s ON s.id = po.supplier_id
		WHERE upper(po.po_number) = upper($1)
		FOR UPDATE OF po`,
		number).Scan(&po.ID, &po.Number, &po.SupplierID, &po.SupplierName, &po.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock purchase order: %w", err)
	}

	rows, err := q.Query(ctx, `
		SELECT line_no, sku, description, quantity, unit_price_minor
		FROM po_lines WHERE po_id = $1 ORDER BY line_no`, po.ID)
	if err != nil {
		return nil, fmt.Errorf("load po lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l matching.POLine
		if err := rows.Scan(&l.LineNo, &l.SKU, &l.Description, &l.Quantity, &l.UnitPriceMinor); err != nil {
			return nil, fmt.Errorf("scan po line: %w", err)
		}
		po.Lines = append(po.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load po lines: %w", err)
	}
	return &po, nil
}

func findDuplicate(ctx context.Context, q db.Querier, supplier *matching.Supplier, number, id string) (string, error) {
	if supplier == nil || number == "" {
		return "", nil
	}
	var original string
	err := q.QueryRow(ctx, `
		SELECT id FROM invoices
		WHERE supplier_id = $1 AND invoice_number = $2 AND id <> $3 AND duplicate_of IS NULL
		ORDER BY created_at
		LIMIT 1`,
		supplier.ID, number, id).Scan(&original)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find duplicate: %w", err)
	}
	return original, nil
}

func writeHeader(ctx context.Context, q db.Querier, id string, inv extraction.ExtractedInvoice,
	supplier *matching.Supplier, po *matching.PO, duplicateOf string, shipFrom *geocode.Point,
) error {
	var supplierID, poID *string
	if supplier != nil {
		supplierID = &supplier.ID
	}
	if po != nil {
		poID = &po.ID
	}
	var lng, lat float64
	if shipFrom != nil {
		lng, lat = shipFrom.Lng, shipFrom.Lat
	}
	_, err := q.Exec(ctx, `
		UPDATE invoices SET
			supplier_id = $2, po_id = $3, duplicate_of = $4,
			invoice_number = $5, currency = $6,
			subtotal_minor = $7, tax_minor = $8, total_minor = $9,
			issue_date = $10, due_date = $11,
			ship_from_city = $12, ship_from_country = $13,
			ship_from_location = CASE WHEN $14 THEN ST_SetSRID(ST_MakePoint($15, $16), 4326)::geography END,
			extraction = $17, extraction_error = '', updated_at = now()
		WHERE id = $1`,
		id, supplierID, poID, nullable(duplicateOf),
		inv.InvoiceNumber, inv.Currency,
		inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor,
		nullableDate(inv.IssueDate), nullableDate(inv.DueDate),
		inv.ShipFrom.City, inv.ShipFrom.Country,
		shipFrom != nil, lng, lat,
		[]byte(inv.Raw))
	if err != nil {
		return fmt.Errorf("write invoice header: %w", err)
	}
	return nil
}

func loadReceiptsForMatching(ctx context.Context, q db.Querier, poID string) ([]matching.Receipt, error) {
	rows, err := q.Query(ctx, `
		SELECT gr.id, w.country, rl.sku, rl.quantity_received
		FROM goods_receipts gr
		JOIN warehouses w ON w.id = gr.warehouse_id
		JOIN receipt_lines rl ON rl.receipt_id = gr.id
		WHERE gr.po_id = $1
		ORDER BY gr.received_on, gr.id, rl.sku`, poID)
	if err != nil {
		return nil, fmt.Errorf("load receipts: %w", err)
	}
	defer rows.Close()
	var receipts []matching.Receipt
	lastID := ""
	for rows.Next() {
		var receiptID, country, sku string
		var qty int64
		if err := rows.Scan(&receiptID, &country, &sku, &qty); err != nil {
			return nil, fmt.Errorf("scan receipt line: %w", err)
		}
		if receiptID != lastID {
			receipts = append(receipts, matching.Receipt{WarehouseCountry: country})
			lastID = receiptID
		}
		last := &receipts[len(receipts)-1]
		last.Lines = append(last.Lines, matching.ReceiptLine{SKU: sku, Quantity: qty})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load receipts: %w", err)
	}
	return receipts, nil
}

func previouslyInvoiced(ctx context.Context, q db.Querier, poID, id string) (map[string]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT pl.sku, SUM(il.quantity)::bigint
		FROM invoice_lines il
		JOIN invoices i ON i.id = il.invoice_id
		JOIN po_lines pl ON pl.po_id = i.po_id AND pl.line_no = il.po_line_no
		WHERE i.po_id = $1
		  AND i.id <> $2
		  AND i.duplicate_of IS NULL
		  AND i.status NOT IN ('REJECTED', 'EXTRACTION_FAILED')
		GROUP BY pl.sku`, poID, id)
	if err != nil {
		return nil, fmt.Errorf("load invoiced quantities: %w", err)
	}
	defer rows.Close()
	invoiced := map[string]int64{}
	for rows.Next() {
		var sku string
		var qty int64
		if err := rows.Scan(&sku, &qty); err != nil {
			return nil, fmt.Errorf("scan invoiced quantity: %w", err)
		}
		invoiced[sku] = qty
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load invoiced quantities: %w", err)
	}
	return invoiced, nil
}

func nearestReceivingWarehouse(ctx context.Context, q db.Querier, id, poID string) (*matching.NearestWarehouse, error) {
	var w matching.NearestWarehouse
	err := q.QueryRow(ctx, `
		SELECT w.name, w.city, ST_Distance(i.ship_from_location, w.location) / 1000
		FROM invoices i
		CROSS JOIN goods_receipts gr
		JOIN warehouses w ON w.id = gr.warehouse_id
		WHERE i.id = $1 AND gr.po_id = $2 AND i.ship_from_location IS NOT NULL
		ORDER BY i.ship_from_location <-> w.location
		LIMIT 1`,
		id, poID).Scan(&w.Name, &w.City, &w.DistanceKM)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("nearest warehouse: %w", err)
	}
	return &w, nil
}

func insertLines(ctx context.Context, q db.Querier, id string, lines []extraction.Line, matches []matching.LineMatch) error {
	poLineFor := map[int]int{}
	for _, m := range matches {
		poLineFor[m.InvoiceLineNo] = m.POLineNo
	}
	for i, l := range lines {
		lineNo := i + 1
		_, err := q.Exec(ctx, `
			INSERT INTO invoice_lines (invoice_id, line_no, po_line_no, sku, description, quantity, unit_price_minor, line_total_minor)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, lineNo, nullableInt(poLineFor[lineNo]), l.SKU, l.Description, l.Quantity, l.UnitPriceMinor, l.LineTotalMinor)
		if err != nil {
			return fmt.Errorf("insert invoice line %d: %w", lineNo, err)
		}
	}
	return nil
}

func insertFindings(ctx context.Context, q db.Querier, id string, findings []matching.Finding) error {
	for _, f := range findings {
		_, err := q.Exec(ctx, `
			INSERT INTO findings (invoice_id, line_no, code, severity, expected, actual, message)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, nullableInt(f.LineNo), f.Code, string(f.Severity), f.Expected, f.Actual, f.Message)
		if err != nil {
			return fmt.Errorf("insert finding %s: %w", f.Code, err)
		}
	}
	return nil
}

func toMatchingInvoice(inv extraction.ExtractedInvoice) matching.Invoice {
	out := matching.Invoice{
		Number:          inv.InvoiceNumber,
		PONumber:        inv.PONumber,
		SupplierName:    inv.SupplierName,
		Currency:        inv.Currency,
		TaxMinor:        inv.TaxMinor,
		TotalMinor:      inv.TotalMinor,
		ShipFromCity:    inv.ShipFrom.City,
		ShipFromCountry: inv.ShipFrom.Country,
	}
	for i, l := range inv.Lines {
		out.Lines = append(out.Lines, matching.Line{
			LineNo:         i + 1,
			SKU:            l.SKU,
			Description:    l.Description,
			Quantity:       l.Quantity,
			UnitPriceMinor: l.UnitPriceMinor,
			LineTotalMinor: l.LineTotalMinor,
		})
	}
	return out
}
