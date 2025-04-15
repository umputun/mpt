package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/provider"
)

//go:generate moq -out mocks/provider.go -pkg mocks -skip-ensure -fmt goimports . Provider

// Runner executes prompts across multiple providers in parallel
type Runner struct {
	providers []Provider
	results   []provider.Result // stores the latest results
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
	r.results = make([]provider.Result, 0, len(r.providers))
	for result := range resultCh {
		r.results = append(r.results, result)
	}

	// check if all providers failed and collect all errors
	allFailed := true
	var errorMessages []string
	for _, result := range r.results {
		if result.Error == nil {
			allFailed = false
		} else {
			errorMessages = append(errorMessages, fmt.Sprintf("%s: %v", result.Provider, result.Error))
		}
	}

	// if all providers failed, return a detailed error message with all provider errors
	if allFailed {
		// with context already canceled or deadline exceeded, return a more user-friendly error
		if ctx.Err() != nil {
			switch {
			case errors.Is(ctx.Err(), context.Canceled):
				return "", fmt.Errorf("operation canceled by user")
			case errors.Is(ctx.Err(), context.DeadlineExceeded):
				return "", fmt.Errorf("operation timed out, try increasing the timeout")
			}
		}
		return "", fmt.Errorf("all providers failed: %s", strings.Join(errorMessages, "; "))
	}

	// for single provider skip the header
	if len(r.providers) == 1 && len(r.results) == 1 {
		if r.results[0].Error != nil {
			return "", fmt.Errorf("provider %s failed: %w", r.results[0].Provider, r.results[0].Error)
		}
		return r.results[0].Text, nil
	}

	// for multiple providers include headers, but skip failed ones
	resultParts := make([]string, 0, len(r.results))
	for _, result := range r.results {
		if result.Error != nil {
			// log the error but don't include it in the output
			lgr.Printf("[WARN] provider %s failed: %v", result.Provider, result.Error)
			continue
		}
		resultParts = append(resultParts, result.Format())
	}

	if len(resultParts) == 0 {
		// if all providers were filtered out due to errors, return the error from the first one
		return "", fmt.Errorf("all providers failed, see logs for details")
	}

	return strings.Join(resultParts, "\n"), nil
}

// GetResults returns the raw results from the last Run
func (r *Runner) GetResults() []provider.Result {
	return r.results
}
