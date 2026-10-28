package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"ap-agent/internal/clock"
	"ap-agent/internal/payments"
)

func TestListPayments(t *testing.T) {
	a := newTestAPI(t)
	a.payments.EXPECT().ListPayments(mock.Anything).Return([]payments.Summary{{ID: "p1", Status: "SCHEDULED", FundingAmountMinor: 505000000}}, nil).Once()

	rec := a.do(t, http.MethodGet, "/api/payments", nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"fundingAmountMinor":505000000`)
}

func TestRunDue_ReturnsPaymentsEvenWhenSomeFail(t *testing.T) {
	a := newTestAPI(t)
	a.executor.EXPECT().Tick(mock.Anything).Return(errors.New("payment p2: connection reset")).Once()
	a.payments.EXPECT().ListPayments(mock.Anything).Return([]payments.Summary{{ID: "p1", Status: "PAID"}}, nil).Once()

	rec := a.do(t, http.MethodPost, "/api/payments/run-due", nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"PAID"`)
}

func TestRunDue_DatabaseBusyIsReported(t *testing.T) {
	a := newTestAPI(t)
	a.executor.EXPECT().Tick(mock.Anything).Return(fmt.Errorf("claim due payment: %w", &pgconn.PgError{Code: "55P03"})).Once()

	rec := a.do(t, http.MethodPost, "/api/payments/run-due", nil, "")

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
}

func TestAdvanceDays(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		svcErr     error
		callsSvc   bool
		wantStatus int
		wantCode   string
	}{
		{"one day", `{"days":1}`, nil, true, http.StatusOK, ""},
		{"too many days", `{"days":1000}`, clock.ErrInvalidDays, true, http.StatusUnprocessableEntity, "INVALID_DAYS"},
		{"missing days", `{}`, clock.ErrInvalidDays, true, http.StatusUnprocessableEntity, "INVALID_DAYS"},
		{"zero days", `{"days":0}`, clock.ErrInvalidDays, true, http.StatusUnprocessableEntity, "INVALID_DAYS"},
		{"not a number", `{"days":"one"}`, nil, false, http.StatusBadRequest, "INVALID_BODY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			if tt.callsSvc {
				a.demo.EXPECT().Advance(mock.Anything, mock.Anything).Return(clock.Clock{Today: "2026-10-28", OffsetDays: 1}, tt.svcErr).Once()
			}

			rec := a.do(t, http.MethodPost, "/api/demo/advance-days", bytes.NewBufferString(tt.body), "application/json")

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode == "" {
				assert.JSONEq(t, `{"today":"2026-10-28","offsetDays":1}`, rec.Body.String())
				return
			}
			code, _ := errorCode(t, rec)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}

func TestDemoClockAndReset(t *testing.T) {
	a := newTestAPI(t)
	a.demo.EXPECT().Now(mock.Anything).Return(clock.Clock{Today: "2026-10-27"}, nil).Once()
	a.demo.EXPECT().Reset(mock.Anything).Return(clock.Clock{Today: "2026-10-27"}, nil).Once()

	rec := a.do(t, http.MethodGet, "/api/demo/clock", nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = a.do(t, http.MethodPost, "/api/demo/reset", nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"today":"2026-10-27","offsetDays":0}`, rec.Body.String())
}

func TestMeta(t *testing.T) {
	a := newTestAPI(t)
	rec := a.do(t, http.MethodGet, "/api/meta", nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"fixtureMode":true,"model":"claude-sonnet-5-5","buyerName":"Andes Retail S.A.S.","buyerCountry":"CO","fundingCurrency":"COP"}`, rec.Body.String())
}
