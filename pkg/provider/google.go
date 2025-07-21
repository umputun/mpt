package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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
	client, err := genai.NewClient(ctx, option.WithAPIKey(opts.APIKey))
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

	model := g.client.GenerativeModel(g.model)

	// only set max output tokens if not zero (0 means use model's maximum)
	if g.maxTokens != 0 {
		// set max output tokens with safe conversion to int32
		switch {
		case g.maxTokens < 0:
			model.SetMaxOutputTokens(1024) // default value
		case g.maxTokens > 2147483647: // max int32 value
			model.SetMaxOutputTokens(2147483647)
		default:
			model.SetMaxOutputTokens(int32(g.maxTokens))
		}
	}
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		// sanitize any potential sensitive information in error
		return "", fmt.Errorf("google api error: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("google returned empty response")
	}

	text := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			text += string(t)
		}
	}

	return text, nil
}

// Enabled returns whether this provider is enabled
func (g *Google) Enabled() bool {
	return g.enabled
}
