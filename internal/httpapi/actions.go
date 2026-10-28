package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type reasonRequest struct {
	Reason string `json:"reason"`
}

func (h *handlers) approve(c *gin.Context) {
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	detail, err := h.deps.Invoices.Approve(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *handlers) override(c *gin.Context) {
	id, req, ok := h.reasonFor(c)
	if !ok {
		return
	}
	detail, err := h.deps.Invoices.Override(c.Request.Context(), id, req.Reason)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *handlers) reject(c *gin.Context) {
	id, req, ok := h.reasonFor(c)
	if !ok {
		return
	}
	detail, err := h.deps.Invoices.Reject(c.Request.Context(), id, req.Reason)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *handlers) supplierEmail(c *gin.Context) {
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	draft, err := h.deps.Invoices.DraftSupplierEmail(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, draft)
}

func (h *handlers) reasonFor(c *gin.Context) (string, reasonRequest, bool) {
	id, ok := invoiceID(c)
	if !ok {
		return "", reasonRequest{}, false
	}
	var req reasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		WriteError(c, http.StatusBadRequest, "INVALID_BODY", `send JSON like {"reason": "..."}`, nil)
		return "", reasonRequest{}, false
	}
	return id, req, true
}
