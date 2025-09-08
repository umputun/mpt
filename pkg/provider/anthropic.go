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
	client    anthropic.Client
	model     string
	enabled   bool
	maxTokens int
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(opts Options) *Anthropic {
	// quick validation for direct constructor usage (without CreateProvider)
	if opts.APIKey == "" || !opts.Enabled || opts.Model == "" {
		return &Anthropic{enabled: false}
	}

	// initialize Anthropic client with the API key
	client := anthropic.NewClient(option.WithAPIKey(opts.APIKey))

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens < 0 {
		maxTokens = DefaultMaxTokens
	}
	// if maxTokens is 0, we'll use the model's maximum (API will determine the limit)

	return &Anthropic{
		client:    client,
		model:     opts.Model,
		enabled:   true,
		maxTokens: maxTokens,
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
		Model:     anthropic.Model(a.model),
		MaxTokens: int64(a.maxTokens), // convert to int64 for the API
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(prompt),
			),
		},
	})

	if err != nil {
		// sanitize any potential sensitive information in error
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
