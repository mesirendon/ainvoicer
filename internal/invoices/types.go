package invoices

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	StatusReceived         = "RECEIVED"
	StatusExtractionFailed = "EXTRACTION_FAILED"
	StatusApproved         = "APPROVED"
	StatusRejected         = "REJECTED"
	StatusPaymentScheduled = "PAYMENT_SCHEDULED"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("invalid state transition")
)

type ExtractionError struct {
	InvoiceID string
	Err       error
}

func (e *ExtractionError) Error() string {
	return "extraction failed for invoice " + e.InvoiceID + ": " + e.Err.Error()
}

func (e *ExtractionError) Unwrap() error {
	return e.Err
}

type NewFile struct {
	FileName string
	SHA256   string
	Content  []byte
}

type Received struct {
	ID      string
	Created bool
	Claimed bool
}

type Summary struct {
	ID            string    `json:"id"`
	FileName      string    `json:"fileName"`
	SupplierName  string    `json:"supplierName"`
	InvoiceNumber string    `json:"invoiceNumber"`
	Currency      string    `json:"currency"`
	TotalMinor    int64     `json:"totalMinor"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Detail struct {
	ID              string          `json:"id"`
	FileName        string          `json:"fileName"`
	Status          string          `json:"status"`
	SupplierID      string          `json:"supplierId"`
	SupplierName    string          `json:"supplierName"`
	InvoiceNumber   string          `json:"invoiceNumber"`
	PONumber        string          `json:"poNumber"`
	Currency        string          `json:"currency"`
	SubtotalMinor   int64           `json:"subtotalMinor"`
	TaxMinor        int64           `json:"taxMinor"`
	TotalMinor      int64           `json:"totalMinor"`
	IssueDate       string          `json:"issueDate"`
	DueDate         string          `json:"dueDate"`
	ShipFromCity    string          `json:"shipFromCity"`
	ShipFromCountry string          `json:"shipFromCountry"`
	DuplicateOf     string          `json:"duplicateOf"`
	ExtractionError string          `json:"extractionError"`
	Extraction      json.RawMessage `json:"extraction"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Lines           []Line          `json:"lines"`
	Findings        []Finding       `json:"findings"`
	PurchaseOrder   *PurchaseOrder  `json:"purchaseOrder"`
	Payment         *Payment        `json:"payment"`
	Audit           []AuditEntry    `json:"audit"`
}

type Payment struct {
	ID                 string    `json:"id"`
	Status             string    `json:"status"`
	AmountMinor        int64     `json:"amountMinor"`
	DiscountMinor      int64     `json:"discountMinor"`
	Currency           string    `json:"currency"`
	ScheduledFor       string    `json:"scheduledFor"`
	FundingCurrency    string    `json:"fundingCurrency"`
	FxRateE8           int64     `json:"fxRateE8"`
	FxSpreadBps        int       `json:"fxSpreadBps"`
	FundingAmountMinor int64     `json:"fundingAmountMinor"`
	ProcessorPaymentID string    `json:"processorPaymentId"`
	Error              string    `json:"error"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Line struct {
	LineNo         int    `json:"lineNo"`
	POLineNo       *int   `json:"poLineNo"`
	SKU            string `json:"sku"`
	Description    string `json:"description"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
	LineTotalMinor int64  `json:"lineTotalMinor"`
}

type Finding struct {
	ID       int64  `json:"id"`
	LineNo   *int   `json:"lineNo"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

type PurchaseOrder struct {
	ID           string    `json:"id"`
	Number       string    `json:"number"`
	SupplierID   string    `json:"supplierId"`
	SupplierName string    `json:"supplierName"`
	Currency     string    `json:"currency"`
	Status       string    `json:"status"`
	Lines        []POLine  `json:"lines"`
	Receipts     []Receipt `json:"receipts"`
}

type POLine struct {
	LineNo         int    `json:"lineNo"`
	SKU            string `json:"sku"`
	Description    string `json:"description"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
}

type Receipt struct {
	ID               string        `json:"id"`
	WarehouseName    string        `json:"warehouseName"`
	WarehouseCity    string        `json:"warehouseCity"`
	WarehouseCountry string        `json:"warehouseCountry"`
	ReceivedOn       string        `json:"receivedOn"`
	Lines            []ReceiptLine `json:"lines"`
}

type ReceiptLine struct {
	SKU      string `json:"sku"`
	Quantity int64  `json:"quantity"`
}

type AuditEntry struct {
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Sample struct {
	Name     string `json:"name"`
	FileName string `json:"fileName"`
}
