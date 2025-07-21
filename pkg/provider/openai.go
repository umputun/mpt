package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// DefaultMaxTokens defines the default value for max tokens if not specified or negative
const DefaultMaxTokens = 1024

// DefaultTemperature defines the default temperature if not specified or negative
const DefaultTemperature = 0.7

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(opts Options) *OpenAI {
	// quick validation for direct constructor usage (without CreateProvider)
	if opts.APIKey == "" || !opts.Enabled || opts.Model == "" {
		return &OpenAI{enabled: false}
	}

	client := openai.NewClient(opts.APIKey)

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens < 0 {
		maxTokens = DefaultMaxTokens
	}
	// if maxTokens is 0, we'll use the model's maximum (API will determine the limit)

	// set default temperature if not specified
	temperature := opts.Temperature
	if temperature <= 0 {
		temperature = DefaultTemperature
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
		// add more context to the error before sanitizing
		var apiErr string

		// attempt to extract more context from the OpenAI error
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "401"):
			apiErr = "openai api error (authentication failed): %w"
		case strings.Contains(errMsg, "429"):
			apiErr = "openai api error (rate limit exceeded): %w"
		case strings.Contains(errMsg, "model"):
			apiErr = "openai api error (model issue - check if model exists): %w"
		case strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline"):
			apiErr = "openai api error (request timed out): %w"
		case strings.Contains(errMsg, "context") || strings.Contains(errMsg, "length"):
			apiErr = "openai api error (context length/token limit): %w"
		default:
			apiErr = "openai api error: %w"
		}

		// sanitize any potential sensitive information in error
		return "", fmt.Errorf(apiErr, err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("openai returned no choices - check your model configuration and prompt length")
	}

	return resp.Choices[0].Message.Content, nil
}

// Enabled returns whether this provider is enabled
func (o *OpenAI) Enabled() bool {
	return o.enabled
}
