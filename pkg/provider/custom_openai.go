package provider

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// CustomOpenAI implements Provider interface for OpenAI-compatible providers
type CustomOpenAI struct {
	name      string     // custom provider name
	client    *openai.Client
	model     string
	enabled   bool
	maxTokens int
}

// CustomOptions defines options for custom OpenAI-compatible providers
type CustomOptions struct {
	Name      string // custom provider name
	BaseURL   string // base URL for the API
	APIKey    string
	Model     string
	Enabled   bool
	MaxTokens int
}

// NewCustomOpenAI creates a new custom OpenAI-compatible provider
func NewCustomOpenAI(opts CustomOptions) *CustomOpenAI {
	if opts.BaseURL == "" || opts.Model == "" || !opts.Enabled {
		return &CustomOpenAI{enabled: false}
	}

	// create custom configuration
	config := openai.DefaultConfig(opts.APIKey)
	config.BaseURL = opts.BaseURL

	// create client with custom configuration
	client := openai.NewClientWithConfig(config)

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024 // default value
	}

	// set default name if not specified
	name := opts.Name
	if name == "" {
		name = "CustomLLM"
	}

	return &CustomOpenAI{
		name:      name,
		client:    client,
		model:     opts.Model,
		enabled:   true,
		maxTokens: maxTokens,
	}
}

// Name returns the custom provider name
func (c *CustomOpenAI) Name() string {
	return c.name
}

// Generate sends a prompt to the custom provider and returns the generated text
func (c *CustomOpenAI) Generate(ctx context.Context, prompt string) (string, error) {
	if !c.enabled {
		return "", fmt.Errorf("%s provider is not enabled", c.name)
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: c.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			MaxTokens: c.maxTokens,
		},
	)

	if err != nil {
		return "", fmt.Errorf("%s api error: %w", c.name, err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s returned no choices", c.name)
	}

	return resp.Choices[0].Message.Content, nil
}

// Enabled returns whether this provider is enabled
func (c *CustomOpenAI) Enabled() bool {
	return c.enabled
}