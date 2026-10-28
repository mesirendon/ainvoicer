package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ap-agent/internal/db"
)

type advanceRequest struct {
	Days int `json:"days"`
}

func (h *handlers) listPayments(c *gin.Context) {
	list, err := h.deps.Payments.ListPayments(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handlers) runDue(c *gin.Context) {
	if err := h.deps.Executor.Tick(c.Request.Context()); err != nil {
		if db.IsBusy(err) {
			handleError(c, err)
			return
		}
		slog.WarnContext(c.Request.Context(), "run-due finished with errors", "request_id", RequestIDFrom(c), "error", err)
	}
	h.listPayments(c)
}

func (h *handlers) demoClock(c *gin.Context) {
	now, err := h.deps.Demo.Now(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, now)
}

func (h *handlers) advanceDays(c *gin.Context) {
	var req advanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "INVALID_BODY", `send JSON like {"days": 1}`, nil)
		return
	}
	now, err := h.deps.Demo.Advance(c.Request.Context(), req.Days)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, now)
}

func (h *handlers) resetDemo(c *gin.Context) {
	now, err := h.deps.Demo.Reset(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, now)
}
