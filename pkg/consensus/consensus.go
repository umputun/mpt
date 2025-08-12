package consensus

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

//go:generate moq -out mocks/provider.go -pkg mocks -skip-ensure -fmt goimports ../provider Provider

// Manager handles consensus checking and attempts between multiple providers
type Manager struct {
	logger lgr.L
}

// Options configures consensus checking behavior
type Options struct {
	Enabled     bool
	Attempts    int
	Prompt      string
	MixProvider string
}

// AttemptRequest holds the parameters for consensus attempt
type AttemptRequest struct {
	Options   Options
	Providers []provider.Provider
	Results   []provider.Result
}

// AttemptResponse holds the result of consensus attempt
type AttemptResponse struct {
	FinalResults []provider.Result
	Attempts     int
	Achieved     bool
}

// New creates a new consensus manager
func New(logger lgr.L) *Manager {
	if logger == nil {
		logger = lgr.Default()
	}
	return &Manager{
		logger: logger,
	}
}

// Attempt tries to reach consensus among provider results
func (m *Manager) Attempt(ctx context.Context, req AttemptRequest) (*AttemptResponse, error) {
	if !req.Options.Enabled || len(req.Results) <= 1 {
		return &AttemptResponse{
			FinalResults: req.Results,
			Attempts:     0,
			Achieved:     false,
		}, nil
	}

	// find the mix provider to use for consensus checking
	mixProvider := m.findMixProvider(req.Options.MixProvider, req.Providers)
	if mixProvider == nil {
		m.logger.Logf("[WARN] no mix provider available for consensus checking, falling back to first enabled provider")
		// fall back to first enabled provider
		for _, p := range req.Providers {
			if p.Enabled() {
				mixProvider = p
				m.logger.Logf("[INFO] using %s as fallback consensus provider", p.Name())
				break
			}
		}
		if mixProvider == nil {
			return &AttemptResponse{
				FinalResults: req.Results,
				Attempts:     0,
				Achieved:     false,
			}, fmt.Errorf("no enabled providers for consensus checking")
		}
	}

	results := req.Results
	var lastError error
	for attempt := 1; attempt <= req.Options.Attempts; attempt++ {
		// check if results agree using mix model
		checkPrompt := m.buildConsensusCheckPrompt(results)
		agreement, err := mixProvider.Generate(ctx, checkPrompt)
		if err != nil {
			lastError = err
			m.logger.Logf("[WARN] consensus check failed on attempt %d: %v", attempt, err)
			continue
		}

		m.logger.Logf("[DEBUG] Consensus check response on attempt %d: %s", attempt, agreement)

		// check if consensus was reached
		if m.isConsensusReached(agreement) {
			m.logger.Logf("[INFO] consensus reached on attempt %d", attempt)
			return &AttemptResponse{
				FinalResults: results,
				Attempts:     attempt,
				Achieved:     true,
			}, nil
		}

		// if no agreement and not last attempt, re-run all providers with context
		if attempt < req.Options.Attempts {
			m.logger.Logf("[INFO] no consensus on attempt %d, retrying with context", attempt)
			rerunPrompt := m.buildConsensusRerunPrompt(req.Options.Prompt, results)
			newResults := m.rerunProviders(ctx, req.Providers, rerunPrompt)

			if len(newResults) > 0 {
				results = newResults
			} else {
				m.logger.Logf("[WARN] failed to get new results on attempt %d", attempt)
			}
		}
	}

	m.logger.Logf("[INFO] consensus not reached after %d attempts", req.Options.Attempts)
	// return the last error if all attempts failed due to errors
	if lastError != nil && req.Options.Attempts > 0 {
		return &AttemptResponse{
			FinalResults: results,
			Attempts:     req.Options.Attempts,
			Achieved:     false,
		}, fmt.Errorf("consensus checking failed: %w", lastError)
	}
	return &AttemptResponse{
		FinalResults: results,
		Attempts:     req.Options.Attempts,
		Achieved:     false,
	}, nil
}

// findMixProvider finds the provider to use for mixing/consensus
func (m *Manager) findMixProvider(mixProviderName string, providers []provider.Provider) provider.Provider {
	return provider.FindProviderByName(mixProviderName, providers)
}

// buildConsensusCheckPrompt creates a prompt to check if responses agree
func (m *Manager) buildConsensusCheckPrompt(results []provider.Result) string {
	var sb strings.Builder
	sb.WriteString("Do the following AI responses fundamentally agree on the main points? ")
	sb.WriteString("Answer with just YES if they agree, or NO if they significantly disagree.\n\n")

	for i, r := range results {
		if r.Error != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("Response %d from %s:\n", i+1, r.Provider))
		sb.WriteString(r.Text)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Answer: ")
	return sb.String()
}

// buildConsensusRerunPrompt creates a prompt for providers to reconsider with context
func (m *Manager) buildConsensusRerunPrompt(originalPrompt string, conflictingResults []provider.Result) string {
	var sb strings.Builder
	sb.WriteString("Original question:\n")
	sb.WriteString(originalPrompt)
	sb.WriteString("\n\nOther AI models provided these different perspectives:\n\n")

	for _, r := range conflictingResults {
		if r.Error != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("--- %s's response ---\n", r.Provider))
		sb.WriteString(r.Text)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Please reconsider your answer taking these different perspectives into account. ")
	sb.WriteString("Provide a thoughtful response that addresses the key points raised.")

	return sb.String()
}

// rerunProviders runs all providers again with a new prompt
func (m *Manager) rerunProviders(ctx context.Context, providers []provider.Provider, prompt string) []provider.Result {
	r := runner.New(providers...)
	_, err := r.Run(ctx, prompt)
	if err != nil {
		m.logger.Logf("[WARN] failed to rerun providers for consensus: %v", err)
		return nil
	}
	return r.GetResults()
}

// isConsensusReached checks if the response indicates consensus was reached
func (m *Manager) isConsensusReached(response string) bool {
	// normalize the response
	response = strings.TrimSpace(strings.ToLower(response))

	// remove common punctuation at the end
	response = strings.Trim(response, ".,;:!?")

	// check for explicit "yes" at the beginning
	if strings.HasPrefix(response, "yes") {
		return true
	}

	// check for explicit "no" at the beginning
	if strings.HasPrefix(response, "no") {
		return false
	}

	// check negative indicators first to avoid false positives
	negativeIndicators := []string{
		"disagree", "conflict", "different", "not", "don't", "doesn't",
		"diverge", "contradict", "oppose", "inconsistent", "vary", "differ",
	}
	for _, indicator := range negativeIndicators {
		if strings.Contains(response, indicator) {
			return false
		}
	}

	// check positive indicators
	positiveIndicators := []string{
		"agree", "consensus", "same", "similar", "consistent", "align",
		"concur", "unanimous", "accord", "harmony", "unified",
	}
	for _, indicator := range positiveIndicators {
		if strings.Contains(response, indicator) {
			return true
		}
	}

	// look for patterns like "the responses agree" or "they agree"
	agreePatterns := []string{
		"responses agree",
		"they agree",
		"models agree",
		"answers agree",
		"providers agree",
		"all agree",
	}
	for _, pattern := range agreePatterns {
		if strings.Contains(response, pattern) {
			return true
		}
	}

	// default to no consensus if uncertain
	return false
}
