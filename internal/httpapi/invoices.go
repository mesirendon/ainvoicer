package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ap-agent/internal/invoices"
)

type handlers struct {
	deps Deps
}

func (h *handlers) uploadInvoice(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.deps.MaxUploadBytes+1<<20)
	header, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			h.fileTooLarge(c)
			return
		}
		WriteError(c, http.StatusBadRequest, "MISSING_FILE", "send the PDF as multipart field \"file\"", nil)
		return
	}
	if header.Size > h.deps.MaxUploadBytes {
		h.fileTooLarge(c)
		return
	}
	f, err := header.Open()
	if err != nil {
		WriteError(c, http.StatusBadRequest, "UNREADABLE_FILE", "the uploaded file could not be read", nil)
		return
	}
	defer func() { _ = f.Close() }()
	pdf, err := io.ReadAll(io.LimitReader(f, h.deps.MaxUploadBytes+1))
	if err != nil {
		WriteError(c, http.StatusBadRequest, "UNREADABLE_FILE", "the uploaded file could not be read", nil)
		return
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		WriteError(c, http.StatusUnprocessableEntity, "NOT_A_PDF", "the uploaded file is not a PDF", nil)
		return
	}

	detail, created, err := h.deps.Invoices.Process(c.Request.Context(), header.Filename, pdf)
	if err != nil {
		handleError(c, err)
		return
	}
	writeProcessed(c, detail, created)
}

func (h *handlers) fileTooLarge(c *gin.Context) {
	WriteError(c, http.StatusUnprocessableEntity, "FILE_TOO_LARGE",
		fmt.Sprintf("the file is larger than %d MB", h.deps.MaxUploadBytes>>20), nil)
}

func (h *handlers) listInvoices(c *gin.Context) {
	list, err := h.deps.Invoices.List(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handlers) getInvoice(c *gin.Context) {
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	detail, err := h.deps.Invoices.Get(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *handlers) invoiceFile(c *gin.Context) {
	id, ok := invoiceID(c)
	if !ok {
		return
	}
	name, file, err := h.deps.Invoices.File(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	c.Data(http.StatusOK, "application/pdf", file)
}

func (h *handlers) listSamples(c *gin.Context) {
	samples, err := h.deps.Samples.ListSamples(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, samples)
}

func (h *handlers) processSample(c *gin.Context) {
	detail, created, err := h.deps.Samples.ProcessSample(c.Request.Context(), c.Param("name"))
	if err != nil {
		handleError(c, err)
		return
	}
	writeProcessed(c, detail, created)
}

func (h *handlers) listPurchaseOrders(c *gin.Context) {
	pos, err := h.deps.PurchaseOrders.ListPurchaseOrders(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, pos)
}

func writeProcessed(c *gin.Context, detail invoices.Detail, created bool) {
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, detail)
}

func invoiceID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		WriteError(c, http.StatusNotFound, "NOT_FOUND", "invoice not found", nil)
		return "", false
	}
	return id, true
}
