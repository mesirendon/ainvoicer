package payments

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeProvider(t *testing.T, status int, body string) *HTTPGateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			assert.Equal(t, "pay-1", r.Header.Get("Idempotency-Key"))
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &HTTPGateway{BaseURL: srv.URL, Client: srv.Client()}
}

func TestHTTPGateway_Pay(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		want          ProviderResult
		wantTransient bool
		wantErr       bool
	}{
		{"succeeded", 200, `{"paymentId":"pm_pay-1","status":"SUCCEEDED"}`, ProviderResult{Status: ProviderSucceeded, ProviderPaymentID: "pm_pay-1"}, false, false},
		{"declined", 402, `{"error":{"code":"PAYMENT_DECLINED","message":"no","details":{"paymentId":"pm_pay-1","status":"DECLINED"}}}`, ProviderResult{Status: ProviderDeclined, ProviderPaymentID: "pm_pay-1"}, false, false},
		{"gateway timeout is transient", 504, `{}`, ProviderResult{}, true, true},
		{"server error is transient", 500, `{}`, ProviderResult{}, true, true},
		{"validation error is permanent", 422, `{"error":{}}`, ProviderResult{}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fakeProvider(t, tt.status, tt.body)
			got, err := g.Pay(context.Background(), PayRequest{IdempotencyKey: "pay-1", DestinationAccount: "4242", AmountMinor: 1, Currency: "USD"})
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantTransient, errors.Is(err, ErrTransient))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHTTPGateway_Lookup(t *testing.T) {
	g := fakeProvider(t, 404, `{"paymentId":"pay-1","status":"NOT_FOUND"}`)
	got, err := g.Lookup(context.Background(), "pay-1")
	require.NoError(t, err)
	assert.Equal(t, ProviderNotFound, got.Status)

	g = fakeProvider(t, 503, `{}`)
	_, err = g.Lookup(context.Background(), "pay-1")
	assert.ErrorIs(t, err, ErrTransient)
}

func TestHTTPGateway_NetworkErrorIsTransient(t *testing.T) {
	g := &HTTPGateway{BaseURL: "http://127.0.0.1:1", Client: &http.Client{Timeout: time.Second}}
	_, err := g.Pay(context.Background(), PayRequest{IdempotencyKey: "pay-1"})
	assert.ErrorIs(t, err, ErrTransient)
}
