package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"ap-agent/internal/extraction"
	"ap-agent/internal/money"
)

var currencySymbols = map[string]string{"USD": "$", "MXN": "$", "EUR": "€"}

type printedLine struct {
	sku, description, quantity, unitPrice, lineTotal string
}

type printed struct {
	sample
	issueDate, dueDate   time.Time
	lines                []printedLine
	subtotal, tax, total string
	symbol               string
}

func main() {
	today := flag.String("today", time.Now().UTC().Format(time.DateOnly), "date the samples are generated for, YYYY-MM-DD (UTC, matching the demo clock)")
	out := flag.String("out", "samples", "output directory")
	flag.Parse()

	day, err := time.Parse(time.DateOnly, *today)
	if err != nil {
		slog.Error("invalid -today", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		slog.Error("create output dir", "error", err)
		os.Exit(1)
	}
	for _, s := range samples {
		if err := generate(s, day, *out); err != nil {
			slog.Error("generate sample", "file", s.file, "error", err)
			os.Exit(1)
		}
		slog.Info("sample written", "file", s.file)
	}
}

func generate(s sample, today time.Time, dir string) error {
	p, err := compute(s, today)
	if err != nil {
		return err
	}
	if err := render(p, today, filepath.Join(dir, s.file+".pdf")); err != nil {
		return err
	}
	data, err := json.MarshalIndent(expected(p), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal expected json: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, s.file+".expected.json"), append(data, '\n'), 0o600)
}

func compute(s sample, today time.Time) (printed, error) {
	p := printed{sample: s, issueDate: s.issueDate(today), dueDate: s.dueDate(today)}
	if s.layout != "classic" {
		p.symbol = currencySymbols[s.currency]
	}
	var subtotal int64
	for _, it := range s.items {
		lineTotal, err := money.Mul(it.quantity, it.unitPriceMinor)
		if err != nil {
			return printed{}, fmt.Errorf("line %s: %w", it.sku, err)
		}
		if subtotal, err = money.Add(subtotal, lineTotal); err != nil {
			return printed{}, fmt.Errorf("subtotal: %w", err)
		}
		p.lines = append(p.lines, printedLine{
			sku:         it.sku,
			description: it.description,
			quantity:    fmt.Sprintf("%d", it.quantity),
			unitPrice:   p.symbol + money.FormatMinor(it.unitPriceMinor),
			lineTotal:   p.symbol + money.FormatMinor(lineTotal),
		})
	}
	tax, err := money.ApplyBps(subtotal, s.taxBps)
	if err != nil {
		return printed{}, fmt.Errorf("tax: %w", err)
	}
	total, err := money.Add(subtotal, tax)
	if err != nil {
		return printed{}, fmt.Errorf("total: %w", err)
	}
	p.subtotal = p.symbol + money.FormatMinor(subtotal)
	p.tax = p.symbol + money.FormatMinor(tax)
	p.total = p.symbol + money.FormatMinor(total)
	return p, nil
}

func expected(p printed) extraction.RawInvoice {
	raw := extraction.RawInvoice{
		SupplierName:  p.supplier.name,
		SupplierTaxID: p.supplier.taxID,
		InvoiceNumber: p.invoiceNumber,
		PONumber:      p.poNumber,
		IssueDate:     p.issueDate.Format(time.DateOnly),
		DueDate:       p.dueDate.Format(time.DateOnly),
		Currency:      p.currency,
		Subtotal:      p.subtotal,
		Tax:           p.tax,
		Total:         p.total,
		PaymentTerms:  p.terms,
		ShipFrom: extraction.RawAddress{
			Line1:      p.shipFrom.line1,
			City:       p.shipFrom.city,
			PostalCode: p.shipFrom.postalCode,
			Country:    p.shipFrom.country,
		},
	}
	for i, l := range p.lines {
		raw.Lines = append(raw.Lines, extraction.RawLine{
			SKU:         l.sku,
			Description: l.description,
			Quantity:    p.items[i].quantity,
			UnitPrice:   l.unitPrice,
			LineTotal:   l.lineTotal,
		})
	}
	return raw
}
