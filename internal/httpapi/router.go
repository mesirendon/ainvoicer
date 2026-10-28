package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ap-agent/internal/clock"
	"ap-agent/internal/invoices"
	"ap-agent/internal/payments"
)

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type InvoiceService interface {
	Process(ctx context.Context, fileName string, pdf []byte) (invoices.Detail, bool, error)
	List(ctx context.Context) ([]invoices.Summary, error)
	Get(ctx context.Context, id string) (invoices.Detail, error)
	File(ctx context.Context, id string) (string, []byte, error)
	Approve(ctx context.Context, id string) (invoices.Detail, error)
	Override(ctx context.Context, id, reason string) (invoices.Detail, error)
	Reject(ctx context.Context, id, reason string) (invoices.Detail, error)
}

type SampleService interface {
	ListSamples(ctx context.Context) ([]invoices.Sample, error)
	ProcessSample(ctx context.Context, name string) (invoices.Detail, bool, error)
}

type PurchaseOrderService interface {
	ListPurchaseOrders(ctx context.Context) ([]invoices.PurchaseOrder, error)
}

type PaymentLister interface {
	ListPayments(ctx context.Context) ([]payments.Summary, error)
}

type DueRunner interface {
	Tick(ctx context.Context) error
}

type DemoService interface {
	Now(ctx context.Context) (clock.Clock, error)
	Advance(ctx context.Context, days int) (clock.Clock, error)
	Reset(ctx context.Context) (clock.Clock, error)
}

type Meta struct {
	FixtureMode     bool   `json:"fixtureMode"`
	Model           string `json:"model"`
	BuyerName       string `json:"buyerName"`
	BuyerCountry    string `json:"buyerCountry"`
	FundingCurrency string `json:"fundingCurrency"`
}

type Deps struct {
	Health         HealthChecker
	Invoices       InvoiceService
	Samples        SampleService
	PurchaseOrders PurchaseOrderService
	Payments       PaymentLister
	Executor       DueRunner
	Demo           DemoService
	Meta           Meta
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
	api.POST("/invoices/:id/approve", h.approve)
	api.POST("/invoices/:id/override", h.override)
	api.POST("/invoices/:id/reject", h.reject)
	api.GET("/samples", h.listSamples)
	api.POST("/samples/:name/process", h.processSample)
	api.GET("/purchase-orders", h.listPurchaseOrders)
	api.GET("/payments", h.listPayments)
	api.POST("/payments/run-due", h.runDue)
	api.GET("/demo/clock", h.demoClock)
	api.POST("/demo/advance-days", h.advanceDays)
	api.POST("/demo/reset", h.resetDemo)
	api.GET("/meta", func(c *gin.Context) { c.JSON(http.StatusOK, d.Meta) })
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
