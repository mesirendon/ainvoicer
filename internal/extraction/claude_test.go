package extraction

import (
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

func toolUseResponse(input RawInvoice) string {
	in, _ := json.Marshal(input)
	return `{"id":"msg_1","type":"message","role":"assistant","model":"test-model",
		"content":[{"type":"tool_use","id":"toolu_1","name":"record_invoice","input":` + string(in) + `}],
		"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
}

func fakeClaude(t *testing.T, responses []string) (*ClaudeExtractor, *[]map[string]any) {
	t.Helper()
	var requests []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))
		requests = append(requests, req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responses[len(requests)-1])
	}))
	t.Cleanup(srv.Close)
	client := anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	return &ClaudeExtractor{client: client, model: "test-model"}, &requests
}

func TestClaudeExtract_FirstAttemptValid(t *testing.T) {
	e, requests := fakeClaude(t, []string{toolUseResponse(validRaw())})

	inv, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.NoError(t, err)
	assert.Equal(t, "ACM-1", inv.InvoiceNumber)
	require.Len(t, *requests, 1)
	req := (*requests)[0]
	assert.Equal(t, "test-model", req["model"])
	assert.Equal(t, map[string]any{"type": "auto", "disable_parallel_tool_use": true}, req["tool_choice"])
	tool := req["tools"].([]any)[0].(map[string]any)
	assert.Equal(t, true, tool["strict"])
	assert.Equal(t, false, tool["input_schema"].(map[string]any)["additionalProperties"])
	doc := req["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "document", doc["type"])
	assert.Equal(t, "JVBERi0xLjQ=", doc["source"].(map[string]any)["data"])
	instructions := req["messages"].([]any)[0].(map[string]any)["content"].([]any)[1].(map[string]any)
	assert.Equal(t, map[string]any{"type": "ephemeral"}, instructions["cache_control"])
}

func TestClaudeExtract_RetriesWithValidationError(t *testing.T) {
	bad := validRaw()
	bad.Total = "1.250,00"
	e, requests := fakeClaude(t, []string{toolUseResponse(bad), toolUseResponse(validRaw())})

	inv, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.NoError(t, err)
	assert.Equal(t, int64(125000), inv.TotalMinor)
	require.Len(t, *requests, 2)
	messages := (*requests)[1]["messages"].([]any)
	require.Len(t, messages, 3)
	result := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_result", result["type"])
	assert.Equal(t, "toolu_1", result["tool_use_id"])
	assert.Equal(t, true, result["is_error"])
}

func TestClaudeExtract_FailsAfterRetry(t *testing.T) {
	bad := validRaw()
	bad.Currency = "dollars"
	e, requests := fakeClaude(t, []string{toolUseResponse(bad), toolUseResponse(bad)})

	_, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "attempt 1:")
	assert.Contains(t, err.Error(), "attempt 2:")
	assert.Contains(t, err.Error(), "currency")
	assert.Len(t, *requests, 2)
}

func TestClaudeExtract_KeepsFirstErrorWhenSecondHasNoToolCall(t *testing.T) {
	bad := validRaw()
	bad.Currency = "dollars"
	text := `{"id":"msg_2","type":"message","role":"assistant","model":"test-model",
		"content":[{"type":"text","text":"Sorry."}],
		"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	e, _ := fakeClaude(t, []string{toolUseResponse(bad), text})

	_, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "currency")
	assert.Contains(t, err.Error(), "did not call record_invoice")
}

func TestClaudeExtract_TruncatedResponseIsNotRetried(t *testing.T) {
	truncated := `{"id":"msg_1","type":"message","role":"assistant","model":"test-model",
		"content":[{"type":"tool_use","id":"toolu_1","name":"record_invoice","input":{}}],
		"stop_reason":"max_tokens","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	e, requests := fakeClaude(t, []string{truncated})

	_, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "token limit")
	assert.Len(t, *requests, 1)
}

func TestClaudeExtract_NoToolCall(t *testing.T) {
	text := `{"id":"msg_1","type":"message","role":"assistant","model":"test-model",
		"content":[{"type":"text","text":"Here is the invoice."}],
		"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	e, requests := fakeClaude(t, []string{text, toolUseResponse(validRaw())})

	_, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.NoError(t, err)
	require.Len(t, *requests, 2)
}

func TestClaudeExtract_UpstreamErrorIsNotValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	e := &ClaudeExtractor{
		client: anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0)),
		model:  "test-model",
	}

	_, err := e.Extract(t.Context(), "invoice.pdf", []byte("%PDF-1.4"))

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalid)
}
