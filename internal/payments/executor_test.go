package payments

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var due = Payment{
	ID:                 "pay-1",
	InvoiceID:          "inv-1",
	InvoiceNumber:      "ACM-1",
	PreviousStatus:     StatusScheduled,
	DestinationAccount: "4242000011112222",
	AmountMinor:        558250,
	Currency:           "USD",
}

var errTimeout = fmt.Errorf("%w: context deadline exceeded", ErrTransient)

func newExecutor(t *testing.T) (*Executor, *MockStore, *MockGateway) {
	store, gw := NewMockStore(t), NewMockGateway(t)
	return &Executor{Store: store, Gateway: gw, BatchSize: 10, CallTimeout: 3 * time.Second, MaxAttempts: 3, Backoff: 0}, store, gw
}

func payRequest() PayRequest {
	return PayRequest{IdempotencyKey: "pay-1", DestinationAccount: "4242000011112222", AmountMinor: 558250, Currency: "USD", Description: "Invoice ACM-1"}
}

func TestExecutor(t *testing.T) {
	tests := []struct {
		name       string
		payResults []ProviderResult
		payErrors  []error
		lookup     *ProviderResult
		lookupErr  error
		want       Settlement
	}{
		{
			name:       "approved",
			payResults: []ProviderResult{{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"}},
			payErrors:  []error{nil},
			want:       Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"},
		},
		{
			name:       "declined is not retried",
			payResults: []ProviderResult{{Status: ProviderDeclined, ProviderPaymentID: "pm_pay-1"}},
			payErrors:  []error{nil},
			want:       Settlement{Status: StatusFailed, ProviderPaymentID: "pm_pay-1", Error: "declined by the destination bank"},
		},
		{
			name:       "errTimeout then lookup succeeded",
			payResults: []ProviderResult{{}, {}, {}},
			payErrors:  []error{errTimeout, errTimeout, errTimeout},
			lookup:     &ProviderResult{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"},
			want:       Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"},
		},
		{
			name:       "errTimeout then lookup not found",
			payResults: []ProviderResult{{}, {}, {}},
			payErrors:  []error{errTimeout, errTimeout, errTimeout},
			lookup:     &ProviderResult{Status: ProviderNotFound},
			want:       Settlement{Status: StatusFailed, Error: "the provider has no record of the payment: " + errTimeout.Error()},
		},
		{
			name:       "lookup unreachable becomes unknown",
			payResults: []ProviderResult{{}, {}, {}},
			payErrors:  []error{errTimeout, errTimeout, errTimeout},
			lookup:     &ProviderResult{},
			lookupErr:  errTimeout,
			want:       Settlement{Status: StatusUnknown, Error: "payment status unknown, provider lookup failed: " + errTimeout.Error()},
		},
		{
			name:       "transient error then success on retry",
			payResults: []ProviderResult{{}, {Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"}},
			payErrors:  []error{fmt.Errorf("%w: provider returned 503", ErrTransient), nil},
			want:       Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"},
		},
		{
			name:       "permanent error is not retried but is looked up",
			payResults: []ProviderResult{{}},
			payErrors:  []error{errors.New("provider rejected the payment with 422")},
			lookup:     &ProviderResult{Status: ProviderNotFound},
			want:       Settlement{Status: StatusFailed, Error: "the provider has no record of the payment: provider rejected the payment with 422"},
		},
		{
			name:       "ambiguous error after money moved is resolved by lookup",
			payResults: []ProviderResult{{}},
			payErrors:  []error{errors.New("decode payment response: unexpected EOF")},
			lookup:     &ProviderResult{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"},
			want:       Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"},
		},
		{
			name:       "unexpected status is looked up and stays unknown while pending",
			payResults: []ProviderResult{{Status: "PENDING"}},
			payErrors:  []error{nil},
			lookup:     &ProviderResult{Status: "PENDING"},
			want:       Settlement{Status: StatusUnknown, Error: "unexpected provider status PENDING"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, store, gw := newExecutor(t)
			expectOneDue(store)
			for i := range tt.payResults {
				gw.EXPECT().Pay(mock.Anything, payRequest()).Return(tt.payResults[i], tt.payErrors[i]).Once()
			}
			if tt.lookup != nil {
				gw.EXPECT().Lookup(mock.Anything, "pay-1").Return(*tt.lookup, tt.lookupErr).Once()
			}
			store.EXPECT().SettlePayment(mock.Anything, "pay-1", tt.want).Return(nil).Once()

			require.NoError(t, e.Tick(context.Background()))
		})
	}
}

func TestExecutor_EachAttemptHasItsOwnTimeout(t *testing.T) {
	e, store, gw := newExecutor(t)
	expectOneDue(store)
	gw.EXPECT().Pay(mock.Anything, payRequest()).
		RunAndReturn(func(ctx context.Context, _ PayRequest) (ProviderResult, error) {
			_, hasDeadline := ctx.Deadline()
			assert.True(t, hasDeadline)
			return ProviderResult{Status: ProviderSucceeded}, nil
		}).Once()
	store.EXPECT().SettlePayment(mock.Anything, "pay-1", Settlement{Status: StatusPaid}).Return(nil).Once()

	require.NoError(t, e.Tick(context.Background()))
}

func expectOneDue(store *MockStore) {
	store.EXPECT().ClaimDuePayments(mock.Anything, 1).Return([]Payment{due}, nil).Once()
	store.EXPECT().ClaimDuePayments(mock.Anything, 1).Return(nil, nil).Once()
}

func TestExecutor_ClaimsOnePaymentAtATimeUpToBatchSize(t *testing.T) {
	e, store, gw := newExecutor(t)
	e.BatchSize = 2
	store.EXPECT().ClaimDuePayments(mock.Anything, 1).Return([]Payment{due}, nil).Twice()
	gw.EXPECT().Pay(mock.Anything, payRequest()).Return(ProviderResult{Status: ProviderSucceeded}, nil).Twice()
	store.EXPECT().SettlePayment(mock.Anything, "pay-1", Settlement{Status: StatusPaid}).Return(nil).Twice()

	require.NoError(t, e.Tick(context.Background()))
}

func TestExecutor_NothingDue(t *testing.T) {
	e, store, _ := newExecutor(t)
	store.EXPECT().ClaimDuePayments(mock.Anything, 1).Return(nil, nil).Once()

	require.NoError(t, e.Tick(context.Background()))
}

func TestExecutor_ShutdownFinishesCurrentPaymentAndClaimsNoMore(t *testing.T) {
	e, store, gw := newExecutor(t)
	ctx, cancel := context.WithCancel(context.Background())
	store.EXPECT().ClaimDuePayments(mock.Anything, 1).Return([]Payment{due}, nil).Once()
	gw.EXPECT().Pay(mock.Anything, payRequest()).
		RunAndReturn(func(ctx context.Context, _ PayRequest) (ProviderResult, error) {
			cancel()
			require.NoError(t, ctx.Err())
			return ProviderResult{Status: ProviderSucceeded}, nil
		}).Once()
	store.EXPECT().SettlePayment(mock.Anything, "pay-1", Settlement{Status: StatusPaid}).Return(nil).Once()

	require.NoError(t, e.Tick(ctx))
}

func TestExecutor_SettleErrorIsReturned(t *testing.T) {
	e, store, gw := newExecutor(t)
	expectOneDue(store)
	gw.EXPECT().Pay(mock.Anything, payRequest()).Return(ProviderResult{Status: ProviderSucceeded}, nil).Once()
	store.EXPECT().SettlePayment(mock.Anything, "pay-1", mock.Anything).Return(errors.New("connection reset")).Once()

	assert.Error(t, e.Tick(context.Background()))
}
