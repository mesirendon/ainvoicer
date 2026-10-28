package invoices

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ap-agent/internal/emaildraft"
)

func blockedDetail() Detail {
	one := 1
	return Detail{
		ID:              invoiceID,
		Status:          "BLOCKED",
		SupplierName:    "Acme Industrial Supply Inc.",
		SupplierCountry: "US",
		InvoiceNumber:   "ACM-2026-0431",
		PONumber:        "PO-1002",
		Findings: []Finding{
			{LineNo: &one, Code: "PRICE_VARIANCE", Severity: "BLOCK", Expected: "4.00 USD", Actual: "4.32 USD", Message: "price too high"},
			{Code: "TOTAL_MISMATCH", Severity: "WARNING", Message: "totals differ"},
		},
	}
}

func TestDraftSupplierEmail_SendsOnlyBlockingFindings(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(blockedDetail(), nil).Once()
	want := emaildraft.Input{
		BuyerName:       "Andes Retail S.A.S.",
		SupplierName:    "Acme Industrial Supply Inc.",
		SupplierCountry: "US",
		InvoiceNumber:   "ACM-2026-0431",
		PONumber:        "PO-1002",
		Issues:          []emaildraft.Issue{{Code: "PRICE_VARIANCE", LineNo: 1, Expected: "4.00 USD", Actual: "4.32 USD", Message: "price too high"}},
	}
	m.drafter.EXPECT().Draft(mock.Anything, want).Return(emaildraft.Draft{Subject: "s", Body: "b", GeneratedBy: "template"}, nil).Once()

	draft, err := s.DraftSupplierEmail(context.Background(), invoiceID)

	require.NoError(t, err)
	assert.Equal(t, "s", draft.Subject)
}

func TestDraftSupplierEmail_NoBlockingFindings(t *testing.T) {
	s, m := newService(t)
	d := blockedDetail()
	d.Findings = d.Findings[1:]
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(d, nil).Once()

	_, err := s.DraftSupplierEmail(context.Background(), invoiceID)

	assert.ErrorIs(t, err, ErrNoBlockingFindings)
}

func TestDraftSupplierEmail_DrafterFailure(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(blockedDetail(), nil).Once()
	m.drafter.EXPECT().Draft(mock.Anything, mock.Anything).Return(emaildraft.Draft{}, context.DeadlineExceeded).Once()

	_, err := s.DraftSupplierEmail(context.Background(), invoiceID)

	assert.ErrorIs(t, err, ErrDraftUnavailable)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDraftSupplierEmail_OnlyForBlockedInvoices(t *testing.T) {
	s, m := newService(t)
	d := blockedDetail()
	d.Status = "PAYMENT_SCHEDULED"
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(d, nil).Once()

	_, err := s.DraftSupplierEmail(context.Background(), invoiceID)

	assert.ErrorIs(t, err, ErrConflict)
}

func TestDraftSupplierEmail_UnknownInvoice(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{}, ErrNotFound).Once()

	_, err := s.DraftSupplierEmail(context.Background(), invoiceID)

	assert.True(t, errors.Is(err, ErrNotFound))
}
