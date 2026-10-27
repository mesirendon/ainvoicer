package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ap-agent/internal/extraction"
	"ap-agent/internal/invoices"
)

const testID = "11111111-2222-4333-8444-555555555555"

type testAPI struct {
	router  http.Handler
	inv     *MockInvoiceService
	samples *MockSampleService
	pos     *MockPurchaseOrderService
}

func newTestAPI(t *testing.T) testAPI {
	a := testAPI{inv: NewMockInvoiceService(t), samples: NewMockSampleService(t), pos: NewMockPurchaseOrderService(t)}
	a.router = NewRouter(Deps{
		Health:         NewMockHealthChecker(t),
		Invoices:       a.inv,
		Samples:        a.samples,
		PurchaseOrders: a.pos,
		MaxUploadBytes: 1 << 20,
	})
	return a
}

func (a testAPI) do(t *testing.T, method, path string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = &bytes.Buffer{}
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

func multipartFile(t *testing.T, field, name string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, name)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (string, map[string]any) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Error.Message)
	return body.Error.Code, body.Error.Details
}

func TestUploadInvoice_Statuses(t *testing.T) {
	pdf := []byte("%PDF-1.4 test")
	tests := []struct {
		name       string
		created    bool
		err        error
		wantStatus int
		wantCode   string
	}{
		{"new invoice", true, nil, http.StatusCreated, ""},
		{"same file again", false, nil, http.StatusOK, ""},
		{"extraction invalid", false, &invoices.ExtractionError{InvoiceID: testID, Err: fmt.Errorf("%w: bad currency", extraction.ErrInvalid)}, http.StatusUnprocessableEntity, "EXTRACTION_FAILED"},
		{"extraction timeout", false, &invoices.ExtractionError{InvoiceID: testID, Err: context.DeadlineExceeded}, http.StatusGatewayTimeout, "EXTRACTION_TIMEOUT"},
		{"extraction upstream", false, &invoices.ExtractionError{InvoiceID: testID, Err: errors.New("500 from Claude")}, http.StatusBadGateway, "EXTRACTION_UNAVAILABLE"},
		{"database lock timeout", false, &pgconn.PgError{Code: "55P03"}, http.StatusServiceUnavailable, "DATABASE_BUSY"},
		{"database statement timeout", false, fmt.Errorf("apply verdict: %w", &pgconn.PgError{Code: "57014"}), http.StatusServiceUnavailable, "DATABASE_BUSY"},
		{"state changed concurrently", false, fmt.Errorf("apply verdict: %w", invoices.ErrConflict), http.StatusConflict, "INVALID_STATE"},
		{"unexpected error", false, errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			a.inv.EXPECT().Process(mock.Anything, "inv.pdf", pdf).Return(invoices.Detail{ID: testID, Status: "MATCHED"}, tt.created, tt.err).Once()
			body, ct := multipartFile(t, "file", "inv.pdf", pdf)

			rec := a.do(t, http.MethodPost, "/api/invoices", body, ct)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantCode == "" {
				var d invoices.Detail
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &d))
				assert.Equal(t, testID, d.ID)
				return
			}
			code, details := errorCode(t, rec)
			assert.Equal(t, tt.wantCode, code)
			if errors.As(tt.err, new(*invoices.ExtractionError)) {
				assert.Equal(t, testID, details["invoiceId"])
			}
			if tt.wantStatus == http.StatusServiceUnavailable {
				assert.Equal(t, "1", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestUploadInvoice_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		content    []byte
		wantStatus int
		wantCode   string
	}{
		{"missing file field", "other", []byte("%PDF-1.4"), http.StatusBadRequest, "MISSING_FILE"},
		{"not a pdf", "file", []byte("hello"), http.StatusUnprocessableEntity, "NOT_A_PDF"},
		{"too large", "file", append([]byte("%PDF-"), make([]byte, 1<<20)...), http.StatusUnprocessableEntity, "FILE_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t)
			body, ct := multipartFile(t, tt.field, "x.pdf", tt.content)

			rec := a.do(t, http.MethodPost, "/api/invoices", body, ct)

			assert.Equal(t, tt.wantStatus, rec.Code)
			code, _ := errorCode(t, rec)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}

func TestGetInvoice(t *testing.T) {
	a := newTestAPI(t)
	a.inv.EXPECT().Get(mock.Anything, testID).Return(invoices.Detail{ID: testID, Status: "BLOCKED"}, nil).Once()

	rec := a.do(t, http.MethodGet, "/api/invoices/"+testID, nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"BLOCKED"`)
}

func TestGetInvoice_NotFound(t *testing.T) {
	a := newTestAPI(t)
	a.inv.EXPECT().Get(mock.Anything, testID).Return(invoices.Detail{}, invoices.ErrNotFound).Once()

	rec := a.do(t, http.MethodGet, "/api/invoices/"+testID, nil, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	code, _ := errorCode(t, rec)
	assert.Equal(t, "NOT_FOUND", code)

	rec = a.do(t, http.MethodGet, "/api/invoices/not-a-uuid", nil, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListInvoices(t *testing.T) {
	a := newTestAPI(t)
	a.inv.EXPECT().List(mock.Anything).Return([]invoices.Summary{{ID: testID, TotalMinor: 558250}}, nil).Once()

	rec := a.do(t, http.MethodGet, "/api/invoices", nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"totalMinor":558250`)
}

func TestInvoiceFile(t *testing.T) {
	a := newTestAPI(t)
	a.inv.EXPECT().File(mock.Anything, testID).Return("inv.pdf", []byte("%PDF-1.4"), nil).Once()

	rec := a.do(t, http.MethodGet, "/api/invoices/"+testID+"/file", nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Equal(t, "inline; filename=inv.pdf", rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "%PDF-1.4", rec.Body.String())
}

func TestInvoiceFile_NonASCIIName(t *testing.T) {
	a := newTestAPI(t)
	a.inv.EXPECT().File(mock.Anything, testID).Return("factura_bogotá.pdf", []byte("%PDF-1.4"), nil).Once()

	rec := a.do(t, http.MethodGet, "/api/invoices/"+testID+"/file", nil, "")

	assert.Equal(t, "inline; filename*=utf-8''factura_bogot%C3%A1.pdf", rec.Header().Get("Content-Disposition"))
}

func TestSamples(t *testing.T) {
	a := newTestAPI(t)
	a.samples.EXPECT().ListSamples(mock.Anything).Return([]invoices.Sample{{Name: "01-clean-usd", FileName: "01-clean-usd.pdf"}}, nil).Once()
	a.samples.EXPECT().ProcessSample(mock.Anything, "01-clean-usd").Return(invoices.Detail{ID: testID}, true, nil).Once()
	a.samples.EXPECT().ProcessSample(mock.Anything, "nope").Return(invoices.Detail{}, false, invoices.ErrNotFound).Once()

	rec := a.do(t, http.MethodGet, "/api/samples", nil, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[{"name":"01-clean-usd","fileName":"01-clean-usd.pdf"}]`, rec.Body.String())

	rec = a.do(t, http.MethodPost, "/api/samples/01-clean-usd/process", nil, "")
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = a.do(t, http.MethodPost, "/api/samples/nope/process", nil, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListPurchaseOrders(t *testing.T) {
	a := newTestAPI(t)
	a.pos.EXPECT().ListPurchaseOrders(mock.Anything).Return([]invoices.PurchaseOrder{{Number: "PO-1001"}}, nil).Once()

	rec := a.do(t, http.MethodGet, "/api/purchase-orders", nil, "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"number":"PO-1001"`)
}
