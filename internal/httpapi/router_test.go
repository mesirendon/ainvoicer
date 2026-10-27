package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database up", "/health", nil, http.StatusOK, `{"status":"ok"}`},
		{"database up via api prefix", "/api/health", nil, http.StatusOK, `{"status":"ok"}`},
		{"database down", "/health", errors.New("connection refused"), http.StatusServiceUnavailable,
			`{"error":{"code":"DATABASE_UNAVAILABLE","message":"database is not reachable"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := NewMockHealthChecker(t)
			health.EXPECT().Ping(mock.Anything).Return(tt.pingErr).Once()

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
			NewRouter(health).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
			assert.NotEmpty(t, rec.Header().Get("X-Request-Id"))
		})
	}
}

func TestRequestID_EchoesIncomingHeader(t *testing.T) {
	health := NewMockHealthChecker(t)
	health.EXPECT().Ping(mock.Anything).Return(nil).Once()

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	NewRouter(health).ServeHTTP(rec, req)

	assert.Equal(t, "abc-123", rec.Header().Get("X-Request-Id"))
}

func TestRecover_ReturnsErrorJSON(t *testing.T) {
	router := NewEngine()
	router.GET("/boom", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body errorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL", body.Error.Code)
}

func TestNoRoute_ReturnsErrorJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil)
	NewRouter(NewMockHealthChecker(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":{"code":"NOT_FOUND","message":"route not found"}}`, rec.Body.String())
}

func TestRecover_SkipsBodyWhenAlreadyWritten(t *testing.T) {
	router := NewEngine()
	router.GET("/partial", func(c *gin.Context) {
		c.String(http.StatusOK, "partial")
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/partial", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "partial", rec.Body.String())
}
