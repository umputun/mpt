package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic implements Provider interface for Anthropic
type Anthropic struct {
	client  *anthropic.Client
	model   string
	enabled bool
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(opts Options) *Anthropic {
	if opts.APIKey == "" || !opts.Enabled {
		return &Anthropic{enabled: false}
	}

	// per the SDK, NewClient returns a Client, not *Client
	client := anthropic.NewClient(
		option.WithAPIKey(opts.APIKey),
	)

	return &Anthropic{
		client:  &client,
		model:   opts.Model,
		enabled: true,
	}
}

// Name returns the provider name
func (a *Anthropic) Name() string {
	return "Anthropic"
}

// Generate sends a prompt to Anthropic and returns the generated text
func (a *Anthropic) Generate(ctx context.Context, prompt string) (string, error) {
	if !a.enabled {
		return "", errors.New("anthropic provider is not enabled")
	}

	// create message request using the SDK
	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{
						OfRequestTextBlock: &anthropic.TextBlockParam{Text: prompt},
					},
				},
			},
		},
		Model: a.model,
	})

	if err != nil {
		return "", fmt.Errorf("anthropic api error: %w", err)
	}

	// extract text from response content
	var textParts []string
	for _, block := range message.Content {
		if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
			textParts = append(textParts, textBlock.Text)
		}
	}

	if len(textParts) == 0 {
		return "", errors.New("anthropic returned empty response")
	}

	return textParts[0], nil
}

// Enabled returns whether this provider is enabled
func (a *Anthropic) Enabled() bool {
	return a.enabled
}
