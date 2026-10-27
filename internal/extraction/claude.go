package extraction

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const toolName = "record_invoice"

const prompt = `Extract the data from this supplier invoice and record it by calling the record_invoice tool exactly once.

Rules:
- Copy every monetary amount exactly as printed on the invoice, as text, including thousands separators and decimals. Never convert, round or compute an amount.
- Write dates as YYYY-MM-DD.
- Currency is the ISO 4217 code (USD, EUR, MXN, ...). If only a symbol is printed, use the code the invoice implies.
- Country codes are ISO 3166-1 alpha-2 (US, MX, DE, ...).
- Copy quantities exactly as printed, including any decimals.
- Use an empty string for any field that is not printed on the invoice.
- The invoice may be written in English or Spanish.`

type ClaudeExtractor struct {
	client anthropic.Client
	model  string
}

func NewClaudeExtractor(apiKey, model string) *ClaudeExtractor {
	return &ClaudeExtractor{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (e *ClaudeExtractor) Extract(ctx context.Context, _ string, pdf []byte) (ExtractedInvoice, error) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(pdf)}),
			anthropic.ContentBlockParamUnion{OfText: &anthropic.TextBlockParam{
				Text:         prompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}},
		),
	}

	var attemptErrs []error
	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := e.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     e.model,
			MaxTokens: 16000,
			Messages:  messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: invoiceTool()}},
			ToolChoice: anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{
				DisableParallelToolUse: anthropic.Bool(true),
			}},
		})
		if err != nil {
			return ExtractedInvoice{}, fmt.Errorf("call Claude: %w", err)
		}
		switch resp.StopReason {
		case anthropic.StopReasonRefusal:
			return ExtractedInvoice{}, fmt.Errorf("%w: the model declined to process this document", ErrInvalid)
		case anthropic.StopReasonMaxTokens:
			return ExtractedInvoice{}, fmt.Errorf("%w: the response hit the token limit before the invoice was recorded", ErrInvalid)
		}

		toolUseID, input := findToolCall(resp)
		if toolUseID == "" {
			attemptErrs = append(attemptErrs, fmt.Errorf("attempt %d: %w: the model did not call %s", attempt, ErrInvalid, toolName))
			if len(resp.Content) > 0 {
				messages = append(messages, resp.ToParam())
			}
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(
				"You did not call the record_invoice tool. Call it now with the invoice data.")))
			continue
		}

		messages = append(messages, resp.ToParam())
		inv, err := Parse([]byte(input))
		if err == nil {
			return inv, nil
		}
		attemptErrs = append(attemptErrs, fmt.Errorf("attempt %d: %w", attempt, err))
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewToolResultBlock(toolUseID,
			"The extracted data failed validation:\n"+err.Error()+"\nCall record_invoice again with corrected values copied from the invoice.", true)))
	}
	return ExtractedInvoice{}, errors.Join(attemptErrs...)
}

func findToolCall(resp *anthropic.Message) (string, string) {
	for _, block := range resp.Content {
		if tool, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tool.Name == toolName {
			return tool.ID, tool.JSON.Input.Raw()
		}
	}
	return "", ""
}

func invoiceTool() *anthropic.ToolParam {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	address := object(map[string]any{
		"line1":       text("Street address the goods ship from."),
		"city":        text("City the goods ship from."),
		"postal_code": text("Postal or ZIP code the goods ship from."),
		"country":     text("ISO 3166-1 alpha-2 country code the goods ship from."),
	}, []string{"line1", "city", "postal_code", "country"})
	line := object(map[string]any{
		"sku":         text("Supplier SKU or item code, empty if not printed."),
		"description": text("Item description as printed."),
		"quantity":    map[string]any{"type": "number", "description": "Quantity billed, exactly as printed."},
		"unit_price":  text("Unit price exactly as printed."),
		"line_total":  text("Line total exactly as printed."),
	}, []string{"sku", "description", "quantity", "unit_price", "line_total"})

	return &anthropic.ToolParam{
		Name:        toolName,
		Description: anthropic.String("Record the data extracted from one supplier invoice."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"supplier_name":   text("Legal name of the supplier issuing the invoice."),
				"supplier_tax_id": text("Supplier tax id (EIN, RFC, VAT number), empty if not printed."),
				"invoice_number":  text("Invoice number as printed."),
				"po_number":       text("Buyer purchase order number referenced on the invoice, empty if not printed."),
				"issue_date":      text("Invoice issue date, YYYY-MM-DD."),
				"due_date":        text("Payment due date, YYYY-MM-DD, empty if not printed."),
				"currency":        text("ISO 4217 currency code of the invoice."),
				"subtotal":        text("Subtotal before tax, exactly as printed."),
				"tax":             text("Total tax, exactly as printed. Use 0.00 if the invoice shows zero tax."),
				"total":           text("Invoice total, exactly as printed."),
				"payment_terms":   text("Payment terms text as printed, for example Net 30 or 2/10 net 30."),
				"ship_from":       address,
				"lines":           map[string]any{"type": "array", "items": line},
			},
			Required: []string{
				"supplier_name", "supplier_tax_id", "invoice_number", "po_number", "issue_date", "due_date",
				"currency", "subtotal", "tax", "total", "payment_terms", "ship_from", "lines",
			},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
}
