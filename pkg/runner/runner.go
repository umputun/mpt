package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/umputun/mpt/pkg/provider"
)

//go:generate moq -out mocks/provider.go -pkg mocks -skip-ensure -fmt goimports . Provider

// Runner executes prompts across multiple providers in parallel
type Runner struct {
	providers []Provider
}

// Provider defines the interface for LLM providers
type Provider = provider.Provider

// New creates a new Runner with the given providers
func New(providers ...Provider) *Runner {
	// filter only enabled providers
	var enabledProviders []Provider
	for _, p := range providers {
		if p.Enabled() {
			enabledProviders = append(enabledProviders, p)
		}
	}

	return &Runner{
		providers: enabledProviders,
	}
}

// Run sends a prompt to all enabled providers and returns combined results
func (r *Runner) Run(ctx context.Context, prompt string) (string, error) {
	if len(r.providers) == 0 {
		return "", fmt.Errorf("no enabled providers")
	}

	var wg sync.WaitGroup
	resultCh := make(chan provider.Result, len(r.providers))

	for _, p := range r.providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()

			text, err := p.Generate(ctx, prompt)
			resultCh <- provider.Result{
				Provider: p.Name(),
				Text:     text,
				Error:    err,
			}
		}(p)
	}

	// wait for all goroutines to finish and close the channel
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// collect results
	results := make([]provider.Result, 0, len(r.providers))
	for result := range resultCh {
		results = append(results, result)
	}

	// for single provider skip the header
	if len(r.providers) == 1 && len(results) == 1 {
		if results[0].Error != nil {
			return fmt.Sprintf("%v", results[0].Error), nil
		}
		return results[0].Text, nil
	}

	// for multiple providers include headers
	resultParts := make([]string, 0, len(results))
	for _, result := range results {
		resultParts = append(resultParts, result.Format())
	}

	return strings.Join(resultParts, "\n"), nil
}
