package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"ap-agent/internal/emaildraft"
	"ap-agent/internal/invoices"
)

func TestApprove(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"approved and scheduled", nil, http.StatusOK, ""},
		{"blocked invoice cannot be approved", fmt.Errorf("%w: invoice is BLOCKED", invoices.ErrConflict), http.StatusConflict, "INVALID_STATE"},
		{"unknown invoice", invoices.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"payment cannot be scheduled", fmt.Errorf("%w: no fx rate from GBP to COP", invoices.ErrCannotSchedule), http.StatusUnprocessableEntity, "CANNOT_SCHEDULE"},
		{"lock timeout", &pgconn.PgError{Code: "55P03"}, http.StatusServiceUnavailable, "DATABASE_BUSY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			a.inv.EXPECT().Approve(mock.Anything, testID).Return(invoices.Detail{ID: testID, Status: "PAYMENT_SCHEDULED"}, tt.err).Once()

			rec := a.do(t, http.MethodPost, "/api/invoices/"+testID+"/approve", nil, "")

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode == "" {
				assert.Contains(t, rec.Body.String(), `"status":"PAYMENT_SCHEDULED"`)
				return
			}
			code, _ := errorCode(t, rec)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}

func TestReasonActions(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		body       string
		serviceErr error
		callsSvc   bool
		wantStatus int
		wantCode   string
	}{
		{"override with reason", "override", `{"reason":"supplier confirmed price"}`, nil, true, http.StatusOK, ""},
		{"reject with reason", "reject", `{"reason":"not our order"}`, nil, true, http.StatusOK, ""},
		{"override blank reason", "override", `{"reason":"  "}`, invoices.ErrReasonRequired, true, http.StatusUnprocessableEntity, "REASON_REQUIRED"},
		{"reject wrong state", "reject", `{"reason":"x"}`, fmt.Errorf("%w: invoice is PAID", invoices.ErrConflict), true, http.StatusConflict, "INVALID_STATE"},
		{"override not json", "override", `reason=x`, nil, false, http.StatusBadRequest, "INVALID_BODY"},
		{"reject empty body", "reject", ``, nil, false, http.StatusBadRequest, "INVALID_BODY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			if tt.callsSvc {
				detail := invoices.Detail{ID: testID, Status: "APPROVED"}
				if tt.action == "override" {
					a.inv.EXPECT().Override(mock.Anything, testID, mock.Anything).Return(detail, tt.serviceErr).Once()
				} else {
					a.inv.EXPECT().Reject(mock.Anything, testID, mock.Anything).Return(detail, tt.serviceErr).Once()
				}
			}

			rec := a.do(t, http.MethodPost, "/api/invoices/"+testID+"/"+tt.action, bytes.NewBufferString(tt.body), "application/json")

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode != "" {
				code, _ := errorCode(t, rec)
				assert.Equal(t, tt.wantCode, code)
			}
		})
	}
}

func TestActions_BadID(t *testing.T) {
	a := newTestAPI(t)
	for _, action := range []string{"approve", "override", "reject"} {
		rec := a.do(t, http.MethodPost, "/api/invoices/nope/"+action, bytes.NewBufferString(`{"reason":"x"}`), "application/json")
		assert.Equal(t, http.StatusNotFound, rec.Code, action)
	}
}

func TestSupplierEmail(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"draft returned", nil, http.StatusOK, ""},
		{"no blocking findings", invoices.ErrNoBlockingFindings, http.StatusUnprocessableEntity, "NO_BLOCKING_FINDINGS"},
		{"invoice not on hold", fmt.Errorf("%w: this one is PAID", invoices.ErrConflict), http.StatusConflict, "INVALID_STATE"},
		{"unknown invoice", invoices.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"claude timeout", fmt.Errorf("%w: %w", invoices.ErrDraftUnavailable, context.DeadlineExceeded), http.StatusGatewayTimeout, "DRAFT_TIMEOUT"},
		{"claude failure", fmt.Errorf("%w: 500", invoices.ErrDraftUnavailable), http.StatusBadGateway, "DRAFT_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			a.inv.EXPECT().DraftSupplierEmail(mock.Anything, testID).
				Return(emaildraft.Draft{Subject: "Invoice on hold", Body: "Hello", Language: "en", GeneratedBy: "template"}, tt.err).Once()

			rec := a.do(t, http.MethodPost, "/api/invoices/"+testID+"/supplier-email", nil, "")

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode == "" {
				assert.JSONEq(t, `{"subject":"Invoice on hold","body":"Hello","language":"en","generatedBy":"template"}`, rec.Body.String())
				return
			}
			code, _ := errorCode(t, rec)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}
