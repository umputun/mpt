package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/umputun/mpt/pkg/provider"
)

//go:generate moq -out mocks/provider_mock.go -pkg mocks ../provider Provider

// Runner executes prompts across multiple providers in parallel
type Runner struct {
	providers []provider.Provider
}

// New creates a new Runner with the given providers
func New(providers ...provider.Provider) *Runner {
	// filter only enabled providers
	var enabledProviders []provider.Provider
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
		go func(p provider.Provider) {
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
	resultParts := make([]string, 0, len(r.providers))
	for result := range resultCh {
		resultParts = append(resultParts, result.Format())
	}

	return strings.Join(resultParts, "\n"), nil
}
