package extraction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ap-agent/internal/money"
)

var ErrInvalid = errors.New("extracted invoice failed validation")

type RawInvoice struct {
	SupplierName  string     `json:"supplier_name"`
	SupplierTaxID string     `json:"supplier_tax_id"`
	InvoiceNumber string     `json:"invoice_number"`
	PONumber      string     `json:"po_number"`
	IssueDate     string     `json:"issue_date"`
	DueDate       string     `json:"due_date"`
	Currency      string     `json:"currency"`
	Subtotal      string     `json:"subtotal"`
	Tax           string     `json:"tax"`
	Total         string     `json:"total"`
	PaymentTerms  string     `json:"payment_terms"`
	ShipFrom      RawAddress `json:"ship_from"`
	Lines         []RawLine  `json:"lines"`
}

type RawAddress struct {
	Line1      string `json:"line1"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type RawLine struct {
	SKU         string `json:"sku"`
	Description string `json:"description"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	LineTotal   string `json:"line_total"`
}

type ExtractedInvoice struct {
	SupplierName  string
	SupplierTaxID string
	InvoiceNumber string
	PONumber      string
	IssueDate     time.Time
	DueDate       time.Time
	Currency      string
	SubtotalMinor int64
	TaxMinor      int64
	TotalMinor    int64
	PaymentTerms  string
	ShipFrom      Address
	Lines         []Line
	Raw           json.RawMessage
}

type Address struct {
	Line1      string
	City       string
	PostalCode string
	Country    string
}

type Line struct {
	SKU            string
	Description    string
	Quantity       int64
	UnitPriceMinor int64
	LineTotalMinor int64
}

func (a Address) IsEmpty() bool {
	return a.Line1 == "" && a.City == "" && a.PostalCode == "" && a.Country == ""
}

func Parse(data []byte) (ExtractedInvoice, error) {
	var raw RawInvoice
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return ExtractedInvoice{}, fmt.Errorf("%w: decode JSON: %w", ErrInvalid, err)
	}
	inv, err := raw.Convert()
	if err != nil {
		return ExtractedInvoice{}, err
	}
	inv.Raw = json.RawMessage(data)
	return inv, nil
}

func (r RawInvoice) Convert() (ExtractedInvoice, error) {
	var errs []error
	inv := ExtractedInvoice{
		SupplierName:  strings.TrimSpace(r.SupplierName),
		SupplierTaxID: strings.TrimSpace(r.SupplierTaxID),
		InvoiceNumber: strings.TrimSpace(r.InvoiceNumber),
		PONumber:      strings.TrimSpace(r.PONumber),
		Currency:      strings.TrimSpace(r.Currency),
		PaymentTerms:  strings.TrimSpace(r.PaymentTerms),
		ShipFrom: Address{
			Line1:      strings.TrimSpace(r.ShipFrom.Line1),
			City:       strings.TrimSpace(r.ShipFrom.City),
			PostalCode: strings.TrimSpace(r.ShipFrom.PostalCode),
			Country:    strings.TrimSpace(r.ShipFrom.Country),
		},
	}

	if inv.SupplierName == "" {
		errs = append(errs, errors.New("supplier_name is required"))
	}
	if inv.InvoiceNumber == "" {
		errs = append(errs, errors.New("invoice_number is required"))
	}
	if !isUpperLetters(inv.Currency, 3) {
		errs = append(errs, fmt.Errorf("currency must be a 3-letter ISO 4217 code, got %q", inv.Currency))
	}
	if inv.ShipFrom.Country != "" && !isUpperLetters(inv.ShipFrom.Country, 2) {
		errs = append(errs, fmt.Errorf("ship_from.country must be a 2-letter ISO 3166 code, got %q", inv.ShipFrom.Country))
	}

	inv.IssueDate = parseDate("issue_date", r.IssueDate, true, &errs)
	inv.DueDate = parseDate("due_date", r.DueDate, false, &errs)
	if !inv.IssueDate.IsZero() && !inv.DueDate.IsZero() && inv.DueDate.Before(inv.IssueDate) {
		errs = append(errs, fmt.Errorf("due_date %s is before issue_date %s", r.DueDate, r.IssueDate))
	}
	inv.SubtotalMinor = parseAmount("subtotal", r.Subtotal, &errs)
	inv.TaxMinor = parseAmount("tax", r.Tax, &errs)
	inv.TotalMinor = parseAmount("total", r.Total, &errs)

	if len(r.Lines) == 0 {
		errs = append(errs, errors.New("at least one line is required"))
	}
	for i, l := range r.Lines {
		field := fmt.Sprintf("lines[%d]", i)
		line := Line{
			SKU:         strings.TrimSpace(l.SKU),
			Description: strings.TrimSpace(l.Description),
			Quantity:    l.Quantity,
		}
		if line.Description == "" {
			errs = append(errs, fmt.Errorf("%s.description is required", field))
		}
		if line.Quantity <= 0 {
			errs = append(errs, fmt.Errorf("%s.quantity must be a positive integer, got %d", field, l.Quantity))
		}
		line.UnitPriceMinor = parseAmount(field+".unit_price", l.UnitPrice, &errs)
		line.LineTotalMinor = parseAmount(field+".line_total", l.LineTotal, &errs)
		inv.Lines = append(inv.Lines, line)
	}

	if len(errs) > 0 {
		return ExtractedInvoice{}, fmt.Errorf("%w: %w", ErrInvalid, errors.Join(errs...))
	}
	return inv, nil
}

func parseDate(field, value string, required bool, errs *[]error) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			*errs = append(*errs, fmt.Errorf("%s is required", field))
		}
		return time.Time{}
	}
	t, err := time.Parse(time.DateOnly, value)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be YYYY-MM-DD, got %q", field, value))
		return time.Time{}
	}
	return t
}

func parseAmount(field, value string, errs *[]error) int64 {
	if strings.TrimSpace(value) == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", field))
		return 0
	}
	n, err := money.ParseMinor(value)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", field, err))
		return 0
	}
	return n
}

func isUpperLetters(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
