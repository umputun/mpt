package provider

import (
	"context"
	"fmt"
)

// Provider defines the interface for LLM providers
type Provider interface {
	Name() string
	Generate(ctx context.Context, prompt string) (string, error)
	Enabled() bool
}

// Result represents a generation result from a provider
type Result struct {
	Provider string
	Text     string
	Error    error
}

// Format formats a result for output with a provider header
func (r Result) Format() string {
	if r.Error != nil {
		return fmt.Sprintf("== generated by %s ==\n%v\n", r.Provider, r.Error)
	}
	return fmt.Sprintf("== generated by %s ==\n%s\n", r.Provider, r.Text)
}

// Options defines common options for all providers
type Options struct {
	APIKey    string
	Enabled   bool
	Model     string
	MaxTokens int // maximum number of tokens to generate
}
