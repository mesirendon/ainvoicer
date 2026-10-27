package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ap-agent/internal/db"
	"ap-agent/internal/httpapi"
)

type server struct {
	store     db.Querier
	slowDelay time.Duration

	mu       sync.Mutex
	inFlight map[string]time.Time
}

type createRequest struct {
	DestinationAccount string `json:"destinationAccount" binding:"required"`
	AmountMinor        int64  `json:"amountMinor"`
	Currency           string `json:"currency" binding:"required"`
	Description        string `json:"description"`
}

type paymentResponse struct {
	PaymentID        string `json:"paymentId"`
	Status           string `json:"status"`
	DestinationLast4 string `json:"destinationLast4"`
	AmountMinor      int64  `json:"amountMinor"`
	Currency         string `json:"currency"`
}

func (s *server) createPayment(c *gin.Context) {
	ctx := c.Request.Context()
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		httpapi.WriteError(c, http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required", nil)
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpapi.WriteError(c, http.StatusBadRequest, "INVALID_BODY", err.Error(), nil)
		return
	}
	if req.AmountMinor <= 0 || len(req.Currency) != 3 || len(req.DestinationAccount) < 4 {
		httpapi.WriteError(c, http.StatusUnprocessableEntity, "VALIDATION_FAILED",
			"amountMinor must be positive, currency a 3-letter code and destinationAccount at least 4 characters", nil)
		return
	}
	slog.InfoContext(ctx, "payment requested", "request_id", httpapi.RequestIDFrom(c), "key", key,
		"destination_last4", last4(req.DestinationAccount), "amount_minor", req.AmountMinor, "currency", req.Currency)

	existing, err := findPayment(ctx, s.store, key)
	if err == nil {
		s.respond(c, req, existing)
		return
	}
	if !errors.Is(err, errNotFound) {
		s.internalError(c, err)
		return
	}

	if strings.HasPrefix(req.DestinationAccount, "2222") {
		s.hangWithoutRecording(ctx, key)
		s.timedOut(c)
		return
	}

	status := "SUCCEEDED"
	if strings.HasPrefix(req.DestinationAccount, "0000") {
		status = "DECLINED"
	}
	stored, err := recordPayment(ctx, s.store, payment{
		Key:                key,
		DestinationAccount: req.DestinationAccount,
		AmountMinor:        req.AmountMinor,
		Currency:           req.Currency,
		Status:             status,
	})
	if err != nil {
		s.internalError(c, err)
		return
	}
	s.respond(c, req, stored)
}

func (s *server) respond(c *gin.Context, req createRequest, stored payment) {
	if stored.DestinationAccount != req.DestinationAccount || stored.AmountMinor != req.AmountMinor || stored.Currency != req.Currency {
		httpapi.WriteError(c, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED",
			"this Idempotency-Key was already used with a different payment", nil)
		return
	}
	if strings.HasPrefix(stored.DestinationAccount, "1111") && !sleep(c.Request.Context(), s.slowDelay) {
		s.timedOut(c)
		return
	}
	if stored.Status == "DECLINED" {
		httpapi.WriteError(c, http.StatusPaymentRequired, "PAYMENT_DECLINED", "the destination bank declined the payment", toResponse(stored))
		return
	}
	c.JSON(http.StatusOK, toResponse(stored))
}

func (s *server) getPayment(c *gin.Context) {
	ctx := c.Request.Context()
	key := c.Param("key")
	if remaining := s.inFlightFor(key); remaining > 0 && !sleep(ctx, remaining) {
		s.timedOut(c)
		return
	}
	p, err := findPayment(ctx, s.store, key)
	if errors.Is(err, errNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"paymentId": key, "status": "NOT_FOUND"})
		return
	}
	if err != nil {
		s.internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResponse(p))
}

func (s *server) hangWithoutRecording(ctx context.Context, key string) {
	now := time.Now()
	s.mu.Lock()
	for k, until := range s.inFlight {
		if now.After(until) {
			delete(s.inFlight, k)
		}
	}
	s.inFlight[key] = now.Add(s.slowDelay)
	s.mu.Unlock()
	sleep(ctx, s.slowDelay)
}

func (s *server) inFlightFor(key string) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.inFlight[key]
	if !ok {
		return 0
	}
	return time.Until(until)
}

func (s *server) timedOut(c *gin.Context) {
	httpapi.WriteError(c, http.StatusGatewayTimeout, "PROVIDER_TIMEOUT", "provider did not answer in time", nil)
}

func (s *server) internalError(c *gin.Context, err error) {
	slog.ErrorContext(c.Request.Context(), "payment mock error", "request_id", httpapi.RequestIDFrom(c), "error", err)
	httpapi.WriteError(c, http.StatusInternalServerError, "INTERNAL", "internal server error", nil)
}

func toResponse(p payment) paymentResponse {
	return paymentResponse{
		PaymentID:        "pm_" + p.Key,
		Status:           p.Status,
		DestinationLast4: last4(p.DestinationAccount),
		AmountMinor:      p.AmountMinor,
		Currency:         p.Currency,
	}
}

func last4(account string) string {
	return account[len(account)-4:]
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
