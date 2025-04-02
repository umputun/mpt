package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// OpenAI implements Provider interface for OpenAI
type OpenAI struct {
	client  *openai.Client
	model   string
	enabled bool
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(opts Options) *OpenAI {
	if opts.APIKey == "" || !opts.Enabled {
		return &OpenAI{enabled: false}
	}

	client := openai.NewClient(opts.APIKey)

	return &OpenAI{
		client:  client,
		model:   opts.Model,
		enabled: true,
	}
}

// Name returns the provider name
func (o *OpenAI) Name() string {
	return "OpenAI"
}

// Generate sends a prompt to OpenAI and returns the generated text
func (o *OpenAI) Generate(ctx context.Context, prompt string) (string, error) {
	if !o.enabled {
		return "", errors.New("openai provider is not enabled")
	}

	resp, err := o.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: o.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)

	if err != nil {
		return "", fmt.Errorf("openai api error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}

// Enabled returns whether this provider is enabled
func (o *OpenAI) Enabled() bool {
	return o.enabled
}