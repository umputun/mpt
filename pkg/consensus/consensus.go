package consensus

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

//go:generate moq -out mocks/provider.go -pkg mocks -skip-ensure -fmt goimports ../provider Provider

// Manager handles consensus checking and attempts between multiple providers
type Manager struct {
	logger                      lgr.L
	negatedAgreementPatterns    []*regexp.Regexp
	negatedDisagreementPatterns []*regexp.Regexp
	negativeIndicators          []*regexp.Regexp
	positiveIndicators          []*regexp.Regexp
	agreePatterns               []*regexp.Regexp
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

// New creates a new consensus manager with pre-compiled regex patterns
func New(logger lgr.L) *Manager {
	if logger == nil {
		logger = lgr.Default()
	}
	return &Manager{
		logger:                      logger,
		negatedAgreementPatterns:    compileNegatedAgreementPatterns(),
		negatedDisagreementPatterns: compileNegatedDisagreementPatterns(),
		negativeIndicators:          compileNegativeIndicators(),
		positiveIndicators:          compilePositiveIndicators(),
		agreePatterns:               compileAgreePatterns(),
	}
}

// compileNegatedAgreementPatterns compiles patterns for negated positive indicators
func compileNegatedAgreementPatterns() []*regexp.Regexp {
	patterns := []string{
		`\bdon't\s+agree`, `\bdoesn't\s+agree`,
		`\bdon't\s+align`, `\bdoesn't\s+align`,
		`\bdon't\s+concur`, `\bdoesn't\s+concur`,
		`\bnot\s+the\s+same\b`, `\bnot\s+similar\b`,
		`\bnot\s+consistent`, `\bnot\s+aligned`,
		`\bno\s+agreement\b`, `\bno\s+consensus\b`,
	}
	return mustCompileAll(patterns)
}

// compileNegatedDisagreementPatterns compiles patterns for negated negative indicators
func compileNegatedDisagreementPatterns() []*regexp.Regexp {
	patterns := []string{
		`\bnot\s+(significantly\s+)?different\b`,
		`\bnot\s+in\s+conflict\b`, `\bnot\s+contradictory\b`,
		`\bnot\s+opposing\b`, `\bnot\s+inconsistent\b`,
		`\bdon't\s+disagree\b`, `\bdoesn't\s+disagree\b`,
		`\bdon't\s+conflict\b`, `\bdoesn't\s+conflict\b`,
		`\bdon't\s+contradict\b`, `\bdoesn't\s+contradict\b`,
		`\bdon't\s+diverge\b`, `\bdoesn't\s+diverge\b`,
		`\bdon't\s+differ\b`, `\bdoesn't\s+differ\b`,
		`\bno\s+disagreement\b`, `\bno\s+conflict\b`,
		`\bno\s+contradiction\b`, `\bno\s+significant\s+difference\b`,
	}
	return mustCompileAll(patterns)
}

// compileNegativeIndicators compiles patterns for disagreement indicators
func compileNegativeIndicators() []*regexp.Regexp {
	patterns := []string{
		`\bdisagree\b`, `\bconflict\b`, `\bdifferent\b`, `\bdiverge\b`,
		`\bcontradict\b`, `\boppose\b`, `\binconsistent\b`, `\bvary\b`, `\bdiffer\b`,
		`\bdisagreeable\b`, `\bdissimilar\b`,
	}
	return mustCompileAll(patterns)
}

// compilePositiveIndicators compiles patterns for agreement indicators
func compilePositiveIndicators() []*regexp.Regexp {
	patterns := []string{
		`\bagree`, `\bconsensus\b`, `\bsame\b`, `\bsimilar`, `\bconsistent`,
		`\balign`, `\bconcur`, `\bunanimous\b`, `\baccord\b`, `\bharmony\b`, `\bunified\b`,
	}
	return mustCompileAll(patterns)
}

// compileAgreePatterns compiles specific agreement phrase patterns
func compileAgreePatterns() []*regexp.Regexp {
	patterns := []string{
		`\bresponses\s+agree`, `\bthey\s+agree`, `\bmodels\s+agree`,
		`\banswers\s+agree`, `\bproviders\s+agree`, `\ball\s+agree`,
	}
	return mustCompileAll(patterns)
}

// mustCompileAll compiles a slice of regex patterns, panicking on any error
func mustCompileAll(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		compiled[i] = regexp.MustCompile(pattern)
	}
	return compiled
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
	sb.WriteString("IMPORTANT: You must answer with ONLY the word YES or NO. ")
	sb.WriteString("Answer YES if they agree on the core message. Answer NO if they significantly disagree.\n\n")

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
	normalized := m.normalizeResponse(response)

	// check for explicit yes/no first
	if result, found := m.checkExplicitAnswer(normalized); found {
		return result
	}

	// check for negated positive indicators (e.g., "don't agree", "not same") - these indicate disagreement
	// must be checked BEFORE checking for positive indicators
	if m.containsNegatedAgreement(normalized) {
		return false
	}

	// check for negation patterns that indicate consensus (e.g., "not different", "don't disagree")
	// these must be checked BEFORE standalone negative indicators
	if m.containsNegatedDisagreement(normalized) {
		return true
	}

	// check for negative indicators (disagreement)
	if m.containsNegativeIndicator(normalized) {
		return false
	}

	// check for positive indicators (agreement)
	if m.containsPositiveIndicator(normalized) {
		return true
	}

	// default to no consensus if uncertain
	return false
}

// normalizeResponse normalizes the response for analysis
func (m *Manager) normalizeResponse(response string) string {
	normalized := strings.TrimSpace(strings.ToLower(response))
	// remove common punctuation at the end
	return strings.Trim(normalized, ".,;:!?")
}

// checkExplicitAnswer checks for explicit yes/no at the beginning
func (m *Manager) checkExplicitAnswer(response string) (result, found bool) {
	if strings.HasPrefix(response, "yes") {
		return true, true
	}
	if strings.HasPrefix(response, "no") {
		return false, true
	}
	return false, false
}

// containsNegatedAgreement checks for negated positive indicators that indicate disagreement
// e.g., "don't agree", "not the same", "doesn't align"
// IMPORTANT: must be called BEFORE containsPositiveIndicator to avoid false positives
func (m *Manager) containsNegatedAgreement(response string) bool {
	for _, pattern := range m.negatedAgreementPatterns {
		if pattern.MatchString(response) {
			return true
		}
	}
	return false
}

// containsNegatedDisagreement checks for negated negative indicators that actually mean consensus
// e.g., "not different", "don't disagree", "not in conflict"
// IMPORTANT: must be called BEFORE containsNegativeIndicator to avoid false negatives
func (m *Manager) containsNegatedDisagreement(response string) bool {
	for _, pattern := range m.negatedDisagreementPatterns {
		if pattern.MatchString(response) {
			return true
		}
	}
	return false
}

// containsNegativeIndicator checks if response contains negative consensus indicators using word boundaries
func (m *Manager) containsNegativeIndicator(response string) bool {
	for _, pattern := range m.negativeIndicators {
		if pattern.MatchString(response) {
			return true
		}
	}
	return false
}

// containsPositiveIndicator checks if response contains positive consensus indicators using word boundaries
// note: patterns use start boundary only to match word variations (e.g., "agree", "agrees", "agreement")
func (m *Manager) containsPositiveIndicator(response string) bool {
	for _, pattern := range m.positiveIndicators {
		if pattern.MatchString(response) {
			return true
		}
	}

	// check specific agreement patterns
	for _, pattern := range m.agreePatterns {
		if pattern.MatchString(response) {
			return true
		}
	}

	return false
}
