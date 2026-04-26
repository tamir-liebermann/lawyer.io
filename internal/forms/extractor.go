package forms

import (
	"context"
	"fmt"

	"github.com/lawyer-io/lawyer/internal/anthropic"
)

// ToolClient is the minimal LLM surface needed for structured form extraction.
type ToolClient interface {
	ExtractWithTool(ctx context.Context, system string, msgs []anthropic.Message, tool anthropic.Tool) (*anthropic.ToolUseResult, error)
}

// BuildFormTool creates an Anthropic tool definition from a Form's field list.
func BuildFormTool(form Form) anthropic.Tool {
	props := make(map[string]anthropic.ToolProperty, len(form.Fields))
	var required []string
	for _, f := range form.Fields {
		props[f.Key] = anthropic.ToolProperty{
			Type:        "string",
			Description: f.LabelHE,
		}
		if f.Required {
			required = append(required, f.Key)
		}
	}
	return anthropic.Tool{
		Name:        "fill_form_" + form.ID,
		Description: "מלא את השדות עבור " + form.NameHE + " על פי המידע בשיחה",
		InputSchema: anthropic.ToolInputSchema{
			Type:       "object",
			Properties: props,
			Required:   required,
		},
	}
}

// Extract calls Claude with the conversation text and returns a map of
// field_key → value. Only fields explicitly mentioned in the conversation are
// included; unmentioned fields are absent (not empty strings).
func Extract(ctx context.Context, client ToolClient, form Form, conversationText string) (map[string]string, error) {
	tool := BuildFormTool(form)
	msgs := []anthropic.Message{{Role: "user", Content: conversationText}}
	system := "אתה מסייע לחילוץ נתוני טופס משפטי. " +
		"נתח את השיחה הבאה וחלץ את כל השדות הרלוונטיים לטופס " + form.NameHE + ". " +
		"כלול רק שדות שמוזכרים בשיחה. שדות שלא מוזכרים — אל תכלול."
	result, err := client.ExtractWithTool(ctx, system, msgs, tool)
	if err != nil {
		return nil, fmt.Errorf("forms: extract %s: %w", form.ID, err)
	}
	if result == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(result.Input))
	for k, v := range result.Input {
		if s, ok := v.(string); ok && s != "" {
			out[k] = s
		}
	}
	return out, nil
}
