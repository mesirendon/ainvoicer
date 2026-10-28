package invoices

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ap-agent/internal/fx"
	"ap-agent/internal/payments"
)

func TestApprove_SchedulesThroughStore(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().Approve(mock.Anything, invoiceID, "reviewer", s.Rules).Return(nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: StatusPaymentScheduled}, nil).Once()

	d, err := s.Approve(context.Background(), invoiceID)

	require.NoError(t, err)
	assert.Equal(t, StatusPaymentScheduled, d.Status)
}

func TestApprove_InvalidStateIsReturned(t *testing.T) {
	s, m := newService(t)
	conflict := fmt.Errorf("%w: invoice is BLOCKED", ErrConflict)
	m.store.EXPECT().Approve(mock.Anything, invoiceID, "reviewer", s.Rules).Return(conflict).Once()

	_, err := s.Approve(context.Background(), invoiceID)

	assert.ErrorIs(t, err, ErrConflict)
}

func TestOverride_RequiresReason(t *testing.T) {
	s, _ := newService(t)
	_, err := s.Override(context.Background(), invoiceID, "   ")
	assert.ErrorIs(t, err, ErrReasonRequired)
}

func TestOverride_PassesTrimmedReason(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().Override(mock.Anything, invoiceID, "reviewer", "supplier confirmed new price", s.Rules).Return(nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID}, nil).Once()

	_, err := s.Override(context.Background(), invoiceID, "  supplier confirmed new price ")

	require.NoError(t, err)
}

func TestReason_CountsCharactersNotBytes(t *testing.T) {
	_, err := cleanReason(strings.Repeat("á", 300))
	require.NoError(t, err)
	_, err = cleanReason(strings.Repeat("á", 501))
	assert.ErrorIs(t, err, ErrReasonRequired)
}

func TestReject(t *testing.T) {
	s, m := newService(t)
	m.store.EXPECT().Reject(mock.Anything, invoiceID, "reviewer", "duplicate").Return(nil).Once()
	m.store.EXPECT().GetInvoice(mock.Anything, invoiceID).Return(Detail{ID: invoiceID, Status: StatusRejected}, nil).Once()

	d, err := s.Reject(context.Background(), invoiceID, "duplicate")
	require.NoError(t, err)
	assert.Equal(t, StatusRejected, d.Status)

	_, err = s.Reject(context.Background(), invoiceID, "")
	assert.ErrorIs(t, err, ErrReasonRequired)
}

func TestPlanPayment(t *testing.T) {
	rules := Rules{FundingCurrency: "COP"}
	usdCop := &fx.Rate{Base: "USD", Quote: "COP", RateE8: 400000000000, SpreadBps: 100}
	mxnCop := &fx.Rate{Base: "MXN", Quote: "COP", RateE8: 22000000000, SpreadBps: 150}
	tests := []struct {
		name         string
		in           PaymentInput
		wantPayOn    string
		wantAmount   int64
		wantDiscount int64
		wantFunding  int64
		wantErr      error
	}{
		{"early discount converted after the discount", PaymentInput{
			InvoiceAmountMinor: 19836000, Currency: "MXN", IssueDate: day("2026-10-24"), DueDate: day("2026-11-23"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30, EarlyDiscountBps: 200, EarlyDiscountDays: 10}, Rate: mxnCop,
		}, "2026-11-03", 19439280, 396720, 4340791224, nil},
		{"usd on due date", PaymentInput{
			InvoiceAmountMinor: 558250, Currency: "USD", IssueDate: day("2026-10-15"), DueDate: day("2026-11-14"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30}, Rate: usdCop,
		}, "2026-11-14", 558250, 0, 2255330000, nil},
		{"funding currency needs no rate", PaymentInput{
			InvoiceAmountMinor: 100000, Currency: "COP", IssueDate: day("2026-10-15"), DueDate: day("2026-11-14"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30},
		}, "2026-11-14", 100000, 0, 100000, nil},
		{"missing rate", PaymentInput{
			InvoiceAmountMinor: 100000, Currency: "GBP", IssueDate: day("2026-10-15"), DueDate: day("2026-11-14"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30},
		}, "", 0, 0, 0, ErrCannotSchedule},
		{"full discount leaves nothing to pay", PaymentInput{
			InvoiceAmountMinor: 100000, Currency: "COP", IssueDate: day("2026-10-24"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30, EarlyDiscountBps: 10000, EarlyDiscountDays: 10},
		}, "", 0, 0, 0, ErrCannotSchedule},
		{"funding rounds to zero", PaymentInput{
			InvoiceAmountMinor: 1, Currency: "USD", IssueDate: day("2026-10-15"), DueDate: day("2026-11-14"),
			Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30}, Rate: &fx.Rate{Base: "USD", Quote: "COP", RateE8: 1000},
		}, "", 0, 0, 0, ErrCannotSchedule},
		{"missing issue date", PaymentInput{
			InvoiceAmountMinor: 100000, Currency: "COP", Today: day("2026-10-27"), Terms: payments.Terms{NetDays: 30},
		}, "", 0, 0, 0, ErrCannotSchedule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rules.PlanPayment(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, day(tt.wantPayOn), got.Schedule.PayOn)
			assert.Equal(t, tt.wantAmount, got.Schedule.AmountMinor)
			assert.Equal(t, tt.wantDiscount, got.Schedule.DiscountMinor)
			assert.Equal(t, tt.wantFunding, got.Conversion.FundingAmountMinor)
			assert.Equal(t, "COP", got.Conversion.FundingCurrency)
			assert.Equal(t, tt.in.Currency, got.Currency)
		})
	}
}
