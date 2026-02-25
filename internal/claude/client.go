package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/vertex"
)

type Client struct {
	client anthropic.Client
	model  string
}

func NewClient(ctx context.Context, gcpRegion, gcpProject, model string) (*Client, error) {
	client := anthropic.NewClient(
		vertex.WithGoogleAuth(ctx, gcpRegion, gcpProject),
	)
	return &Client{
		client: client,
		model:  model,
	}, nil
}

func (c *Client) Evaluate(ctx context.Context, threadText string) (*EvaluationResult, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    anthropic.Model(c.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(BuildUserPrompt(threadText))),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("claude API call failed: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	var text string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text = tb.Text
			break
		}
	}

	if text == "" {
		return nil, fmt.Errorf("no text content in Claude response")
	}

	// Strip markdown code fences if present (e.g. ```json ... ```)
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		if i := strings.Index(trimmed, "\n"); i != -1 {
			trimmed = trimmed[i+1:]
		}
		if strings.HasSuffix(trimmed, "```") {
			trimmed = strings.TrimSuffix(trimmed, "```")
		}
		trimmed = strings.TrimSpace(trimmed)
	}

	var result EvaluationResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response as JSON: %w\nraw response: %s", err, text)
	}

	return &result, nil
}
