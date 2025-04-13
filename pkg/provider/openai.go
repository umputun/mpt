package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// OpenAI implements Provider interface for OpenAI
type OpenAI struct {
	client      *openai.Client
	model       string
	enabled     bool
	maxTokens   int
	temperature float32
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(opts Options) *OpenAI {
	if opts.APIKey == "" || !opts.Enabled {
		return &OpenAI{enabled: false}
	}

	client := openai.NewClient(opts.APIKey)

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens < 0 {
		maxTokens = 1024 // default value
	}
	// if maxTokens is 0, we'll use the model's maximum (API will determine the limit)

	// set default temperature if not specified
	temperature := opts.Temperature
	if temperature <= 0 {
		temperature = 0.7 // default OpenAI temperature
	}

	return &OpenAI{
		client:      client,
		model:       opts.Model,
		enabled:     true,
		maxTokens:   maxTokens,
		temperature: temperature,
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
			MaxTokens:   o.maxTokens,
			Temperature: o.temperature,
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
