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
	client  *genai.Client
	model   string
	enabled bool
}

// NewGoogle creates a new Google provider
func NewGoogle(opts Options) *Google {
	if opts.APIKey == "" || !opts.Enabled {
		return &Google{enabled: false}
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(opts.APIKey))
	if err != nil {
		return &Google{enabled: false}
	}

	return &Google{
		client:  client,
		model:   opts.Model,
		enabled: true,
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
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
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