package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ap-agent/internal/db"
	"ap-agent/internal/invoices"
)

func (r *Repo) GetInvoice(ctx context.Context, id string) (invoices.Detail, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return invoices.Detail{}, fmt.Errorf("begin read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return loadInvoiceDetail(ctx, tx, id)
}

func loadInvoiceDetail(ctx context.Context, q db.Querier, id string) (invoices.Detail, error) {
	var d invoices.Detail
	var supplierID, poID, duplicateOf *string
	var issueDate, dueDate *time.Time
	err := q.QueryRow(ctx, `
		SELECT i.id, i.file_name, i.status,
		       i.supplier_id, COALESCE(s.name, i.extraction->>'supplier_name', ''),
		       i.invoice_number, i.po_id, COALESCE(po.po_number, i.extraction->>'po_number', ''),
		       i.currency, i.subtotal_minor, i.tax_minor, i.total_minor,
		       i.issue_date, i.due_date, i.ship_from_city, i.ship_from_country,
		       i.duplicate_of, i.extraction_error, i.extraction, i.created_at, i.updated_at
		FROM invoices i
		LEFT JOIN suppliers s ON s.id = i.supplier_id
		LEFT JOIN purchase_orders po ON po.id = i.po_id
		WHERE i.id = $1`, id).Scan(
		&d.ID, &d.FileName, &d.Status,
		&supplierID, &d.SupplierName,
		&d.InvoiceNumber, &poID, &d.PONumber,
		&d.Currency, &d.SubtotalMinor, &d.TaxMinor, &d.TotalMinor,
		&issueDate, &dueDate, &d.ShipFromCity, &d.ShipFromCountry,
		&duplicateOf, &d.ExtractionError, &d.Extraction, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.Detail{}, invoices.ErrNotFound
	}
	if err != nil {
		return invoices.Detail{}, fmt.Errorf("load invoice: %w", err)
	}
	d.SupplierID, d.DuplicateOf = text(supplierID), text(duplicateOf)
	d.IssueDate, d.DueDate = formatDate(issueDate), formatDate(dueDate)

	if d.Lines, err = loadInvoiceLines(ctx, q, id); err != nil {
		return invoices.Detail{}, err
	}
	if d.Findings, err = loadFindings(ctx, q, id); err != nil {
		return invoices.Detail{}, err
	}
	if poID != nil {
		po, err := loadPurchaseOrder(ctx, q, *poID)
		if err != nil {
			return invoices.Detail{}, err
		}
		d.PurchaseOrder = &po
	}
	if d.Audit, err = loadAudit(ctx, q, id); err != nil {
		return invoices.Detail{}, err
	}
	return d, nil
}

func (r *Repo) ListInvoices(ctx context.Context) ([]invoices.Summary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.id, i.file_name, COALESCE(s.name, i.extraction->>'supplier_name', ''),
		       i.invoice_number, i.currency, i.total_minor, i.status, i.created_at
		FROM invoices i
		LEFT JOIN suppliers s ON s.id = i.supplier_id
		ORDER BY i.created_at DESC
		LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()
	list := []invoices.Summary{}
	for rows.Next() {
		var s invoices.Summary
		if err := rows.Scan(&s.ID, &s.FileName, &s.SupplierName, &s.InvoiceNumber, &s.Currency, &s.TotalMinor, &s.Status, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invoice: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	return list, nil
}

func (r *Repo) InvoiceFile(ctx context.Context, id string) (string, []byte, error) {
	var name string
	var file []byte
	err := r.pool.QueryRow(ctx, `SELECT file_name, file FROM invoices WHERE id = $1`, id).Scan(&name, &file)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, invoices.ErrNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("load invoice file: %w", err)
	}
	return name, file, nil
}

func loadInvoiceLines(ctx context.Context, q db.Querier, id string) ([]invoices.Line, error) {
	rows, err := q.Query(ctx, `
		SELECT line_no, po_line_no, sku, description, quantity, unit_price_minor, line_total_minor
		FROM invoice_lines WHERE invoice_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return nil, fmt.Errorf("load invoice lines: %w", err)
	}
	defer rows.Close()
	lines := []invoices.Line{}
	for rows.Next() {
		var l invoices.Line
		if err := rows.Scan(&l.LineNo, &l.POLineNo, &l.SKU, &l.Description, &l.Quantity, &l.UnitPriceMinor, &l.LineTotalMinor); err != nil {
			return nil, fmt.Errorf("scan invoice line: %w", err)
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load invoice lines: %w", err)
	}
	return lines, nil
}

func loadFindings(ctx context.Context, q db.Querier, id string) ([]invoices.Finding, error) {
	rows, err := q.Query(ctx, `
		SELECT id, line_no, code, severity, expected, actual, message
		FROM findings WHERE invoice_id = $1
		ORDER BY CASE severity WHEN 'BLOCK' THEN 0 ELSE 1 END, id`, id)
	if err != nil {
		return nil, fmt.Errorf("load findings: %w", err)
	}
	defer rows.Close()
	findings := []invoices.Finding{}
	for rows.Next() {
		var f invoices.Finding
		if err := rows.Scan(&f.ID, &f.LineNo, &f.Code, &f.Severity, &f.Expected, &f.Actual, &f.Message); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load findings: %w", err)
	}
	return findings, nil
}

func loadAudit(ctx context.Context, q db.Querier, id string) ([]invoices.AuditEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT actor, action, detail, created_at
		FROM audit_log WHERE invoice_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return nil, fmt.Errorf("load audit log: %w", err)
	}
	defer rows.Close()
	entries := []invoices.AuditEntry{}
	for rows.Next() {
		var e invoices.AuditEntry
		if err := rows.Scan(&e.Actor, &e.Action, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load audit log: %w", err)
	}
	return entries, nil
}
