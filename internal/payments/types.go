package payments

import (
	"context"
	"errors"
	"time"
)

const (
	StatusScheduled  = "SCHEDULED"
	StatusProcessing = "PROCESSING"
	StatusPaid       = "PAID"
	StatusFailed     = "FAILED"
	StatusUnknown    = "UNKNOWN"
)

const (
	ProviderSucceeded = "SUCCEEDED"
	ProviderDeclined  = "DECLINED"
	ProviderNotFound  = "NOT_FOUND"
)

var ErrTransient = errors.New("transient provider error")

type Payment struct {
	ID                 string
	InvoiceID          string
	InvoiceNumber      string
	PreviousStatus     string
	DestinationAccount string
	AmountMinor        int64
	Currency           string
}

type Settlement struct {
	Status            string
	ProviderPaymentID string
	Error             string
}

type PayRequest struct {
	IdempotencyKey     string
	DestinationAccount string
	AmountMinor        int64
	Currency           string
	Description        string
}

type ProviderResult struct {
	Status            string
	ProviderPaymentID string
}

type Gateway interface {
	Pay(ctx context.Context, req PayRequest) (ProviderResult, error)
	Lookup(ctx context.Context, key string) (ProviderResult, error)
}

type Store interface {
	ClaimDuePayments(ctx context.Context, limit int) ([]Payment, error)
	ClaimStalePayments(ctx context.Context, limit int) ([]Payment, error)
	SettlePayment(ctx context.Context, id string, s Settlement) error
}

type Summary struct {
	ID                 string    `json:"id"`
	InvoiceID          string    `json:"invoiceId"`
	InvoiceNumber      string    `json:"invoiceNumber"`
	SupplierName       string    `json:"supplierName"`
	DestinationLast4   string    `json:"destinationLast4"`
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

func last4(account string) string {
	if len(account) <= 4 {
		return account
	}
	return account[len(account)-4:]
}
