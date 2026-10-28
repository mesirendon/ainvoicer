package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type HTTPGateway struct {
	BaseURL string
	Client  *http.Client
}

type providerPayment struct {
	PaymentID string `json:"paymentId"`
	Status    string `json:"status"`
}

type providerError struct {
	Error struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details providerPayment `json:"details"`
	} `json:"error"`
}

func (g *HTTPGateway) Pay(ctx context.Context, req PayRequest) (ProviderResult, error) {
	body, err := json.Marshal(map[string]any{
		"destinationAccount": req.DestinationAccount,
		"amountMinor":        req.AmountMinor,
		"currency":           req.Currency,
		"description":        req.Description,
	})
	if err != nil {
		return ProviderResult{}, fmt.Errorf("encode payment: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/payments", bytes.NewReader(body))
	if err != nil {
		return ProviderResult{}, fmt.Errorf("build payment request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)

	status, data, err := g.do(httpReq)
	if err != nil {
		return ProviderResult{}, err
	}
	switch {
	case status == http.StatusOK:
		var p providerPayment
		if err := json.Unmarshal(data, &p); err != nil {
			return ProviderResult{}, fmt.Errorf("decode payment response: %w", err)
		}
		return ProviderResult{Status: p.Status, ProviderPaymentID: p.PaymentID}, nil
	case status == http.StatusPaymentRequired:
		var e providerError
		if err := json.Unmarshal(data, &e); err != nil {
			return ProviderResult{}, fmt.Errorf("decode decline: %w", err)
		}
		return ProviderResult{Status: ProviderDeclined, ProviderPaymentID: e.Error.Details.PaymentID}, nil
	case status >= 500:
		return ProviderResult{}, fmt.Errorf("%w: provider returned %d", ErrTransient, status)
	default:
		return ProviderResult{}, fmt.Errorf("provider rejected the payment with %d: %s", status, bytes.TrimSpace(data))
	}
}

func (g *HTTPGateway) Lookup(ctx context.Context, key string) (ProviderResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, g.BaseURL+"/payments/"+url.PathEscape(key), nil)
	if err != nil {
		return ProviderResult{}, fmt.Errorf("build lookup request: %w", err)
	}
	status, data, err := g.do(httpReq)
	if err != nil {
		return ProviderResult{}, err
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return ProviderResult{}, fmt.Errorf("%w: lookup returned %d", ErrTransient, status)
	}
	var p providerPayment
	if err := json.Unmarshal(data, &p); err != nil {
		return ProviderResult{}, fmt.Errorf("decode lookup response: %w", err)
	}
	return ProviderResult{Status: p.Status, ProviderPaymentID: p.PaymentID}, nil
}

func (g *HTTPGateway) do(req *http.Request) (int, []byte, error) {
	resp, err := g.Client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w", ErrTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: read response: %w", ErrTransient, err)
	}
	return resp.StatusCode, data, nil
}
