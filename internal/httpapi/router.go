package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ap-agent/internal/invoices"
)

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type InvoiceService interface {
	Process(ctx context.Context, fileName string, pdf []byte) (invoices.Detail, bool, error)
	List(ctx context.Context) ([]invoices.Summary, error)
	Get(ctx context.Context, id string) (invoices.Detail, error)
	File(ctx context.Context, id string) (string, []byte, error)
}

type SampleService interface {
	ListSamples(ctx context.Context) ([]invoices.Sample, error)
	ProcessSample(ctx context.Context, name string) (invoices.Detail, bool, error)
}

type PurchaseOrderService interface {
	ListPurchaseOrders(ctx context.Context) ([]invoices.PurchaseOrder, error)
}

type Deps struct {
	Health         HealthChecker
	Invoices       InvoiceService
	Samples        SampleService
	PurchaseOrders PurchaseOrderService
	MaxUploadBytes int64
}

func NewEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(RequestID(), AccessLog(), Recover())
	router.NoRoute(func(c *gin.Context) {
		WriteError(c, http.StatusNotFound, "NOT_FOUND", "route not found", nil)
	})
	return router
}

func NewRouter(d Deps) *gin.Engine {
	router := NewEngine()
	router.MaxMultipartMemory = d.MaxUploadBytes
	router.GET("/health", Health(d.Health))

	h := &handlers{deps: d}
	api := router.Group("/api")
	api.GET("/health", Health(d.Health))
	api.POST("/invoices", h.uploadInvoice)
	api.GET("/invoices", h.listInvoices)
	api.GET("/invoices/:id", h.getInvoice)
	api.GET("/invoices/:id/file", h.invoiceFile)
	api.GET("/samples", h.listSamples)
	api.POST("/samples/:name/process", h.processSample)
	api.GET("/purchase-orders", h.listPurchaseOrders)
	return router
}

func Health(health HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := health.Ping(c.Request.Context()); err != nil {
			slog.WarnContext(c.Request.Context(), "health check failed", "request_id", RequestIDFrom(c), "error", err)
			WriteError(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "database is not reachable", nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
