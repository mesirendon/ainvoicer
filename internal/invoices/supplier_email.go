package invoices

import (
	"context"
	"errors"
	"fmt"

	"ap-agent/internal/emaildraft"
)

var (
	ErrNoBlockingFindings = errors.New("the invoice has no blocking findings to explain")
	ErrDraftUnavailable   = errors.New("the email draft could not be generated")
)

func (s *Service) DraftSupplierEmail(ctx context.Context, id string) (emaildraft.Draft, error) {
	d, err := s.Store.GetInvoice(ctx, id)
	if err != nil {
		return emaildraft.Draft{}, err
	}
	if d.Status != "BLOCKED" {
		return emaildraft.Draft{}, fmt.Errorf("%w: only BLOCKED invoices are on hold, this one is %s", ErrConflict, d.Status)
	}
	in := emaildraft.Input{
		BuyerName:       s.BuyerName,
		SupplierName:    d.SupplierName,
		SupplierCountry: d.SupplierCountry,
		InvoiceNumber:   d.InvoiceNumber,
		PONumber:        d.PONumber,
	}
	for _, f := range d.Findings {
		if f.Severity != "BLOCK" {
			continue
		}
		issue := emaildraft.Issue{Code: f.Code, Expected: f.Expected, Actual: f.Actual, Message: f.Message}
		if f.LineNo != nil {
			issue.LineNo = *f.LineNo
		}
		in.Issues = append(in.Issues, issue)
	}
	if len(in.Issues) == 0 {
		return emaildraft.Draft{}, ErrNoBlockingFindings
	}

	draftCtx, cancel := context.WithTimeout(ctx, s.DraftTimeout)
	defer cancel()
	draft, err := s.Drafter.Draft(draftCtx, in)
	if err != nil {
		return emaildraft.Draft{}, fmt.Errorf("%w: %w", ErrDraftUnavailable, err)
	}
	return draft, nil
}
