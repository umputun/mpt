package provider

import (
	"context"
	"fmt"
)

// CustomOpenAI implements Provider interface for OpenAI-compatible providers
// it wraps the OpenAI provider with custom base URL and name
type CustomOpenAI struct {
	name     string  // custom provider name
	provider *OpenAI // underlying OpenAI provider
}

// CustomOptions defines options for custom OpenAI-compatible providers
type CustomOptions struct {
	Name         string       // custom provider name
	BaseURL      string       // base URL for the API
	APIKey       string       // API key for authentication
	Model        string       // model name to use
	Enabled      bool         // whether provider is enabled
	MaxTokens    int          // maximum number of tokens to generate
	Temperature  float32      // controls randomness (0-1, default: 0.7)
	EndpointType EndpointType // endpoint type (auto, responses, chat_completions)
	HTTPClient   HTTPClient   // optional HTTP client for dependency injection
}

// NewCustomOpenAI creates a new custom OpenAI-compatible provider
func NewCustomOpenAI(opts CustomOptions) *CustomOpenAI {
	if opts.BaseURL == "" || opts.Model == "" || !opts.Enabled {
		return &CustomOpenAI{provider: &OpenAI{enabled: false}}
	}

	// set default name if not specified
	name := opts.Name
	if name == "" {
		name = "CustomLLM"
	}

	// set default endpoint type if not specified
	endpointType := opts.EndpointType
	if endpointType == "" {
		endpointType = EndpointTypeChatCompletions // default to chat completions for custom providers
	}

	// create underlying OpenAI provider with custom options
	provider := NewOpenAI(Options{
		APIKey:            opts.APIKey,
		Enabled:           opts.Enabled,
		Model:             opts.Model,
		MaxTokens:         opts.MaxTokens,
		Temperature:       opts.Temperature,
		HTTPClient:        opts.HTTPClient,
		BaseURL:           opts.BaseURL,
		ForceEndpointType: endpointType,
	})

	return &CustomOpenAI{
		name:     name,
		provider: provider,
	}
}

// Name returns the custom provider name
func (c *CustomOpenAI) Name() string {
	return c.name
}

// Generate sends a prompt to the custom provider and returns the generated text
func (c *CustomOpenAI) Generate(ctx context.Context, prompt string) (string, error) {
	if !c.provider.Enabled() {
		return "", fmt.Errorf("%s provider is not enabled", c.name)
	}

	return c.provider.Generate(ctx, prompt)
}

// Enabled returns whether this provider is enabled
func (c *CustomOpenAI) Enabled() bool {
	return c.provider.Enabled()
}
