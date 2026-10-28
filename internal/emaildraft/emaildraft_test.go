package emaildraft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleInput(country string) Input {
	return Input{
		BuyerName:       "Andes Retail S.A.S.",
		SupplierName:    "Acme Industrial Supply Inc.",
		SupplierCountry: country,
		InvoiceNumber:   "ACM-2026-0431",
		PONumber:        "PO-1002",
		Issues: []Issue{{
			Code:    "PRICE_VARIANCE",
			LineNo:  1,
			Message: "Unit price 4.32 USD for ACM-400 is 8.00% above the PO price 4.00 USD, beyond the 2.00% tolerance.",
		}},
	}
}

func TestLanguage(t *testing.T) {
	assert.Equal(t, "es", Language("MX"))
	assert.Equal(t, "es", Language("co"))
	assert.Equal(t, "en", Language("US"))
	assert.Equal(t, "en", Language("DE"))
	assert.Equal(t, "en", Language(""))
}

func TestTemplateDrafter(t *testing.T) {
	d, err := TemplateDrafter{}.Draft(context.Background(), sampleInput("US"))
	require.NoError(t, err)

	assert.Equal(t, "Invoice ACM-2026-0431 on hold: action needed", d.Subject)
	assert.Contains(t, d.Body, "Hello Acme Industrial Supply Inc. team,")
	assert.Contains(t, d.Body, "invoice ACM-2026-0431 for purchase order PO-1002")
	assert.Contains(t, d.Body, "- Unit price 4.32 USD for ACM-400 is 8.00% above")
	assert.Contains(t, d.Body, "Accounts Payable\nAndes Retail S.A.S.")
	assert.Equal(t, "template", d.GeneratedBy)

	again, err := TemplateDrafter{}.Draft(context.Background(), sampleInput("US"))
	require.NoError(t, err)
	assert.Equal(t, d, again)
}

func fakeClaude(t *testing.T, responses ...string) (*ClaudeDrafter, *map[string]any, *int) {
	t.Helper()
	var request map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responses[calls])
		calls++
	}))
	t.Cleanup(srv.Close)
	client := anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	return &ClaudeDrafter{client: client, model: "test-model"}, &request, &calls
}

func message(content string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[` + content +
		`],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
}

func TestClaudeDrafter_ReturnsDraftInSupplierLanguage(t *testing.T) {
	d, request, _ := fakeClaude(t, message(`{"type":"tool_use","id":"toolu_1","name":"draft_email","input":{"subject":"Factura en espera","body":"Hola equipo"}}`))

	draft, err := d.Draft(context.Background(), sampleInput("MX"))

	require.NoError(t, err)
	assert.Equal(t, Draft{Subject: "Factura en espera", Body: "Hola equipo", Language: "es", GeneratedBy: "claude"}, draft)
	text := (*request)["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	assert.Contains(t, text, "Write in Spanish.")
	assert.Contains(t, text, "ACM-2026-0431")
	assert.Contains(t, text, "8.00% above the PO price")
	assert.NotContains(t, text, "PRICE_VARIANCE")
	tool := (*request)["tools"].([]any)[0].(map[string]any)
	assert.Equal(t, true, tool["strict"])
}

func TestClaudeDrafter_NoToolCallIsRetriedOnce(t *testing.T) {
	text := message(`{"type":"text","text":"Here is an email."}`)
	d, _, calls := fakeClaude(t, text, message(`{"type":"tool_use","id":"toolu_1","name":"draft_email","input":{"subject":"On hold","body":"Hello"}}`))
	draft, err := d.Draft(context.Background(), sampleInput("US"))
	require.NoError(t, err)
	assert.Equal(t, "On hold", draft.Subject)
	assert.Equal(t, 2, *calls)

	d, _, calls = fakeClaude(t, text, text)
	_, err = d.Draft(context.Background(), sampleInput("US"))
	assert.ErrorIs(t, err, ErrNoDraft)
	assert.Equal(t, 2, *calls)
}

func TestClaudeDrafter_EmptyBody(t *testing.T) {
	empty := message(`{"type":"tool_use","id":"toolu_1","name":"draft_email","input":{"subject":"Hi","body":" "}}`)
	d, _, _ := fakeClaude(t, empty, empty)
	_, err := d.Draft(context.Background(), sampleInput("US"))
	assert.ErrorIs(t, err, ErrNoDraft)
}
