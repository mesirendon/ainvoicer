package emaildraft

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrNoDraft = errors.New("the model did not return a draft")

type Issue struct {
	Code     string `json:"-"`
	LineNo   int    `json:"line,omitempty"`
	Expected string `json:"expected"`
	Actual   string `json:"invoiced"`
	Message  string `json:"issue"`
}

type Input struct {
	BuyerName       string  `json:"buyer"`
	SupplierName    string  `json:"supplier"`
	SupplierCountry string  `json:"-"`
	InvoiceNumber   string  `json:"invoice_number"`
	PONumber        string  `json:"purchase_order"`
	Issues          []Issue `json:"issues"`
}

type Draft struct {
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	Language    string `json:"language"`
	GeneratedBy string `json:"generatedBy"`
}

var spanishSpeaking = map[string]bool{
	"AR": true, "BO": true, "CL": true, "CO": true, "CR": true, "DO": true, "EC": true, "ES": true,
	"GT": true, "HN": true, "MX": true, "NI": true, "PA": true, "PE": true, "PY": true, "SV": true, "UY": true, "VE": true,
}

func Language(country string) string {
	if spanishSpeaking[strings.ToUpper(country)] {
		return "es"
	}
	return "en"
}

type TemplateDrafter struct{}

func (TemplateDrafter) Draft(_ context.Context, in Input) (Draft, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Hello %s team,\n\n", orDefault(in.SupplierName, "supplier"))
	fmt.Fprintf(&b, "We received invoice %s", orDefault(in.InvoiceNumber, "(no number)"))
	if in.PONumber != "" {
		fmt.Fprintf(&b, " for purchase order %s", in.PONumber)
	}
	b.WriteString(". It is on hold because our 3-way match against the purchase order and goods receipts found the following:\n\n")
	for _, issue := range in.Issues {
		fmt.Fprintf(&b, "- %s\n", issue.Message)
	}
	b.WriteString("\nCould you please review these points and send a corrected invoice or the supporting documents?\n\n")
	fmt.Fprintf(&b, "Thank you,\nAccounts Payable\n%s\n", in.BuyerName)
	return Draft{
		Subject:     fmt.Sprintf("Invoice %s on hold: action needed", orDefault(in.InvoiceNumber, "(no number)")),
		Body:        b.String(),
		Language:    "en",
		GeneratedBy: "template",
	}, nil
}

func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
