package emaildraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const toolName = "draft_email"

const instructions = `You write short, polite emails from a buyer's accounts-payable team to a supplier.
The supplier's invoice is on hold. Draft an email that explains each issue below in plain words and asks the supplier for a corrected invoice or the missing documents.

Rules:
- Use only the facts in the issues and details below. Do not invent amounts, dates, policies or payment promises.
- Do not mention internal codes, tolerances in basis points, or that software checked the invoice.
- Keep the body under 180 words, with one short bullet per issue.
- Sign as "Accounts Payable" followed by the buyer name.
- Write in %s.
- Record the result by calling the draft_email tool.`

type ClaudeDrafter struct {
	client anthropic.Client
	model  string
}

func NewClaudeDrafter(apiKey, model string) *ClaudeDrafter {
	return &ClaudeDrafter{client: anthropic.NewClient(option.WithAPIKey(apiKey)), model: model}
}

type toolInput struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (d *ClaudeDrafter) Draft(ctx context.Context, in Input) (Draft, error) {
	lang := Language(in.SupplierCountry)
	languageName := "English"
	if lang == "es" {
		languageName = "Spanish"
	}
	facts, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return Draft{}, fmt.Errorf("encode draft input: %w", err)
	}

	prompt := fmt.Sprintf(instructions, languageName) + "\n\n<details>\n" + string(facts) + "\n</details>"
	draft, err := d.attempt(ctx, prompt, lang)
	if errors.Is(err, ErrNoDraft) {
		draft, err = d.attempt(ctx, prompt, lang)
	}
	return draft, err
}

func (d *ClaudeDrafter) attempt(ctx context.Context, prompt, lang string) (Draft, error) {
	resp, err := d.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     d.model,
		MaxTokens: 4000,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		Tools:     []anthropic.ToolUnionParam{{OfTool: draftTool()}},
		ToolChoice: anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{
			DisableParallelToolUse: anthropic.Bool(true),
		}},
	})
	if err != nil {
		return Draft{}, fmt.Errorf("call Claude: %w", err)
	}
	for _, block := range resp.Content {
		tool, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || tool.Name != toolName {
			continue
		}
		var out toolInput
		if err := json.Unmarshal([]byte(tool.JSON.Input.Raw()), &out); err != nil {
			return Draft{}, fmt.Errorf("%w: %w", ErrNoDraft, err)
		}
		if strings.TrimSpace(out.Subject) == "" || strings.TrimSpace(out.Body) == "" {
			return Draft{}, fmt.Errorf("%w: empty subject or body", ErrNoDraft)
		}
		return Draft{Subject: out.Subject, Body: out.Body, Language: lang, GeneratedBy: "claude"}, nil
	}
	return Draft{}, fmt.Errorf("%w: stop reason %s", ErrNoDraft, resp.StopReason)
}

func draftTool() *anthropic.ToolParam {
	return &anthropic.ToolParam{
		Name:        toolName,
		Description: anthropic.String("Record the drafted email to the supplier."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"subject": map[string]any{"type": "string", "description": "Email subject line."},
				"body":    map[string]any{"type": "string", "description": "Email body in plain text."},
			},
			Required:    []string{"subject", "body"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
}
