package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ap-agent/internal/clock"
	"ap-agent/internal/db"
	"ap-agent/internal/extraction"
	"ap-agent/internal/invoices"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func WriteError(c *gin.Context, status int, code, message string, details any) {
	c.AbortWithStatusJSON(status, errorBody{Error: errorDetail{Code: code, Message: message, Details: details}})
}

func handleError(c *gin.Context, err error) {
	var extractionErr *invoices.ExtractionError
	switch {
	case errors.Is(err, invoices.ErrNotFound):
		WriteError(c, http.StatusNotFound, "NOT_FOUND", "not found", nil)
	case errors.Is(err, invoices.ErrConflict):
		WriteError(c, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
	case errors.Is(err, clock.ErrInvalidDays):
		WriteError(c, http.StatusUnprocessableEntity, "INVALID_DAYS", err.Error(), nil)
	case errors.Is(err, invoices.ErrReasonRequired):
		WriteError(c, http.StatusUnprocessableEntity, "REASON_REQUIRED", err.Error(), nil)
	case errors.Is(err, invoices.ErrCannotSchedule):
		WriteError(c, http.StatusUnprocessableEntity, "CANNOT_SCHEDULE", err.Error(), nil)
	case errors.As(err, &extractionErr):
		details := gin.H{"invoiceId": extractionErr.InvoiceID}
		switch {
		case errors.Is(err, extraction.ErrInvalid):
			WriteError(c, http.StatusUnprocessableEntity, "EXTRACTION_FAILED", extractionErr.Err.Error(), details)
		case errors.Is(err, context.DeadlineExceeded):
			WriteError(c, http.StatusGatewayTimeout, "EXTRACTION_TIMEOUT", "the extraction service did not answer in time", details)
		default:
			WriteError(c, http.StatusBadGateway, "EXTRACTION_UNAVAILABLE", "the extraction service failed", details)
		}
	case db.IsBusy(err):
		c.Header("Retry-After", "1")
		WriteError(c, http.StatusServiceUnavailable, "DATABASE_BUSY", "the database is busy, retry shortly", nil)
	default:
		slog.ErrorContext(c.Request.Context(), "request failed", "request_id", RequestIDFrom(c), "error", err)
		WriteError(c, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
	}
}
