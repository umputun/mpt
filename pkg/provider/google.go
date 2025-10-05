package provider

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

// Google implements Provider interface for Google's Gemini models
type Google struct {
	client    *genai.Client
	model     string
	enabled   bool
	maxTokens int
}

// NewGoogle creates a new Google provider
func NewGoogle(opts Options) *Google {
	// quick validation for direct constructor usage (without CreateProvider)
	if opts.APIKey == "" || !opts.Enabled || opts.Model == "" {
		return &Google{enabled: false}
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  opts.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return &Google{enabled: false}
	}

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens < 0 {
		maxTokens = DefaultMaxTokens
	}
	// if maxTokens is 0, we'll use the model's maximum (API will determine the limit)

	return &Google{
		client:    client,
		model:     opts.Model,
		enabled:   true,
		maxTokens: maxTokens,
	}
}

// Name returns the provider name
func (g *Google) Name() string {
	return "Google"
}

// Generate sends a prompt to Google and returns the generated text
func (g *Google) Generate(ctx context.Context, prompt string) (string, error) {
	if !g.enabled {
		return "", errors.New("google provider is not enabled")
	}

	// prepare content for request
	content := &genai.Content{
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}

	// prepare generation config
	var config *genai.GenerateContentConfig
	if g.maxTokens > 0 {
		// only set max output tokens if not zero (0 means use model's maximum)
		maxTokens := int32(g.maxTokens)
		if g.maxTokens > 2147483647 { // max int32 value
			maxTokens = 2147483647
		}
		config = &genai.GenerateContentConfig{
			MaxOutputTokens: maxTokens,
		}
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, []*genai.Content{content}, config)
	if err != nil {
		// sanitize any potential sensitive information in error
		return "", fmt.Errorf("google api error: %w", err)
	}

	// extract text from response
	text := resp.Text()
	if text == "" {
		return "", errors.New("google returned empty response")
	}

	return text, nil
}

// Enabled returns whether this provider is enabled
func (g *Google) Enabled() bool {
	return g.enabled
}
