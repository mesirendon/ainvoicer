package payments

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReconciler(t *testing.T) {
	tests := []struct {
		name      string
		previous  string
		lookup    ProviderResult
		lookupErr error
		want      Settlement
	}{
		{"unknown resolved as paid", StatusUnknown, ProviderResult{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"}, nil,
			Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"}},
		{"unknown not found fails", StatusUnknown, ProviderResult{Status: ProviderNotFound}, nil,
			Settlement{Status: StatusFailed, Error: "the provider has no record of the payment"}},
		{"unknown still unreachable stays unknown", StatusUnknown, ProviderResult{}, errTimeout,
			Settlement{Status: StatusUnknown, Error: "payment status unknown, provider lookup failed: " + errTimeout.Error()}},
		{"unknown declined fails", StatusUnknown, ProviderResult{Status: ProviderDeclined, ProviderPaymentID: "pm_pay-1"}, nil,
			Settlement{Status: StatusFailed, ProviderPaymentID: "pm_pay-1", Error: "declined by the destination bank"}},
		{"stale processing found paid", StatusProcessing, ProviderResult{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"}, nil,
			Settlement{Status: StatusPaid, ProviderPaymentID: "pm_pay-1"}},
		{"stale processing never reached provider is rescheduled", StatusProcessing, ProviderResult{Status: ProviderNotFound}, nil,
			Settlement{Status: StatusScheduled}},
		{"stale processing unreachable becomes unknown", StatusProcessing, ProviderResult{}, errTimeout,
			Settlement{Status: StatusUnknown, Error: "payment status unknown, provider lookup failed: " + errTimeout.Error()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, gw := NewMockStore(t), NewMockGateway(t)
			r := &Reconciler{Store: store, Gateway: gw, BatchSize: 10, CallTimeout: 3 * time.Second}
			p := due
			p.PreviousStatus = tt.previous
			store.EXPECT().ClaimStalePayments(mock.Anything, 10).Return([]Payment{p}, nil).Once()
			gw.EXPECT().Lookup(mock.Anything, "pay-1").Return(tt.lookup, tt.lookupErr).Once()
			store.EXPECT().SettlePayment(mock.Anything, "pay-1", tt.want).Return(nil).Once()

			require.NoError(t, r.Tick(context.Background()))
		})
	}
}

func TestReconciler_NothingStale(t *testing.T) {
	store, gw := NewMockStore(t), NewMockGateway(t)
	r := &Reconciler{Store: store, Gateway: gw, BatchSize: 10, CallTimeout: 3 * time.Second}
	store.EXPECT().ClaimStalePayments(mock.Anything, 10).Return(nil, nil).Once()

	require.NoError(t, r.Tick(context.Background()))
}
