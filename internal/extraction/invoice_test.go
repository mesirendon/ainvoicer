package extraction

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validRaw() RawInvoice {
	return RawInvoice{
		SupplierName:  "Acme Industrial Supply Inc.",
		InvoiceNumber: "ACM-1",
		PONumber:      "PO-1001",
		IssueDate:     "2026-10-15",
		DueDate:       "2026-11-14",
		Currency:      "USD",
		Subtotal:      "1,250.00",
		Tax:           "0.00",
		Total:         "$1,250.00",
		ShipFrom:      RawAddress{City: "Houston", PostalCode: "77002", Country: "US"},
		Lines: []RawLine{
			{SKU: "ACM-100", Description: "Bearing", Quantity: 100, UnitPrice: "12.50", LineTotal: "1,250.00"},
		},
	}
}

func TestConvert_Valid(t *testing.T) {
	inv, err := validRaw().Convert()
	require.NoError(t, err)
	assert.Equal(t, int64(125000), inv.TotalMinor)
	assert.Equal(t, int64(1250), inv.Lines[0].UnitPriceMinor)
	assert.Equal(t, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), inv.IssueDate)
	assert.Equal(t, "US", inv.ShipFrom.Country)
}

func TestConvert_OptionalFieldsMayBeEmpty(t *testing.T) {
	raw := validRaw()
	raw.DueDate = ""
	raw.PONumber = ""
	raw.ShipFrom = RawAddress{}
	inv, err := raw.Convert()
	require.NoError(t, err)
	assert.True(t, inv.DueDate.IsZero())
	assert.True(t, inv.ShipFrom.IsEmpty())
}

func TestConvert_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RawInvoice)
		wantMsg string
	}{
		{"missing supplier", func(r *RawInvoice) { r.SupplierName = " " }, "supplier_name is required"},
		{"missing invoice number", func(r *RawInvoice) { r.InvoiceNumber = "" }, "invoice_number is required"},
		{"lowercase currency", func(r *RawInvoice) { r.Currency = "usd" }, "currency must be a 3-letter"},
		{"currency symbol", func(r *RawInvoice) { r.Currency = "$" }, "currency must be a 3-letter"},
		{"bad issue date", func(r *RawInvoice) { r.IssueDate = "15/10/2026" }, "issue_date must be YYYY-MM-DD"},
		{"missing issue date", func(r *RawInvoice) { r.IssueDate = "" }, "issue_date is required"},
		{"bad due date", func(r *RawInvoice) { r.DueDate = "next month" }, "due_date must be YYYY-MM-DD"},
		{"three decimals", func(r *RawInvoice) { r.Total = "1,250.000" }, "total"},
		{"missing tax", func(r *RawInvoice) { r.Tax = "" }, "tax is required"},
		{"no lines", func(r *RawInvoice) { r.Lines = nil }, "at least one line"},
		{"zero quantity", func(r *RawInvoice) { r.Lines[0].Quantity = 0 }, "lines[0].quantity must be a positive integer"},
		{"missing line total", func(r *RawInvoice) { r.Lines[0].LineTotal = "" }, "lines[0].line_total is required"},
		{"missing description", func(r *RawInvoice) { r.Lines[0].Description = "" }, "lines[0].description is required"},
		{"country name instead of code", func(r *RawInvoice) { r.ShipFrom.Country = "USA" }, "ship_from.country"},
		{"due before issue", func(r *RawInvoice) { r.DueDate = "2026-10-01" }, "due_date 2026-10-01 is before issue_date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := validRaw()
			tt.mutate(&raw)
			_, err := raw.Convert()
			require.ErrorIs(t, err, ErrInvalid)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestParse_RejectsFractionalQuantity(t *testing.T) {
	_, err := Parse([]byte(`{"supplier_name":"x","lines":[{"quantity":2.5}]}`))
	require.ErrorIs(t, err, ErrInvalid)
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`{"supplier_name":"x","surprise":1}`))
	require.ErrorIs(t, err, ErrInvalid)
}

func TestFixtureExtractor_AllSamplesConvert(t *testing.T) {
	dir := filepath.Join("..", "..", "samples")
	pdfs, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	require.NoError(t, err)
	require.NotEmpty(t, pdfs)

	f := NewFixtureExtractor(dir)
	for _, pdf := range pdfs {
		t.Run(filepath.Base(pdf), func(t *testing.T) {
			inv, err := f.Extract(context.Background(), filepath.Base(pdf), nil)
			require.NoError(t, err)
			assert.NotEmpty(t, inv.Raw)
			var sum int64
			for _, l := range inv.Lines {
				assert.Equal(t, l.Quantity*l.UnitPriceMinor, l.LineTotalMinor)
				sum += l.LineTotalMinor
			}
			assert.Equal(t, inv.SubtotalMinor, sum)
			assert.Equal(t, inv.TotalMinor, inv.SubtotalMinor+inv.TaxMinor)
		})
	}
}

func TestFixtureExtractor_UnknownFile(t *testing.T) {
	f := NewFixtureExtractor(t.TempDir())
	_, err := f.Extract(context.Background(), "../../etc/passwd.pdf", nil)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "passwd.pdf")
}
