package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic implements Provider interface for Anthropic
type Anthropic struct {
	client  anthropic.Client
	model   string
	enabled bool
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(opts Options) *Anthropic {
	if opts.APIKey == "" || !opts.Enabled {
		return &Anthropic{enabled: false}
	}

	// initialize Anthropic client with the API key
	client := anthropic.NewClient(option.WithAPIKey(opts.APIKey))

	return &Anthropic{
		client:  client,
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

	// create a message request using the SDK
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(prompt),
			),
		},
	})

	if err != nil {
		return "", fmt.Errorf("anthropic api error: %w", err)
	}

	// extract text from response
	var textParts []string
	for _, content := range resp.Content {
		if content.Type == "text" {
			textParts = append(textParts, content.Text)
		}
	}

	if len(textParts) == 0 {
		return "", errors.New("anthropic returned empty response")
	}

	return strings.Join(textParts, ""), nil
}

// Enabled returns whether this provider is enabled
func (a *Anthropic) Enabled() bool {
	return a.enabled
}
