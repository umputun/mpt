package mix

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/consensus"
	"github.com/umputun/mpt/pkg/provider"
)

//go:generate moq -out mocks/provider.go -pkg mocks -skip-ensure -fmt goimports ../provider Provider

// Manager handles mixing results from multiple providers
type Manager struct {
	logger lgr.L
}

// New creates a new mix manager
func New(logger lgr.L) *Manager {
	if logger == nil {
		logger = lgr.Default()
	}
	return &Manager{
		logger: logger,
	}
}

// Request holds the parameters for processing mix mode
type Request struct {
	Prompt            string
	MixPrompt         string
	MixProvider       string
	ConsensusEnabled  bool
	ConsensusAttempts int
	Providers         []provider.Provider
	Results           []provider.Result
}

// Response holds the result of mixing provider responses including consensus information
type Response struct {
	TextWithHeader    string
	RawText           string
	MixProvider       string
	ConsensusAchieved bool
	ConsensusAttempts int
	ConsensusError    error // error from consensus checking, if any
}

// Process handles mixing results from multiple providers with optional consensus
func (m *Manager) Process(ctx context.Context, req Request) (*Response, error) {
	// validate input
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if len(req.Results) == 0 {
		return nil, fmt.Errorf("no results provided to mix")
	}
	if len(req.Providers) == 0 {
		return nil, fmt.Errorf("no providers available for mixing")
	}
	if req.MixPrompt == "" {
		return nil, fmt.Errorf("mix prompt cannot be empty")
	}
	
	// filter successful results
	var successfulResults []provider.Result
	for _, res := range req.Results {
		if res.Error == nil {
			successfulResults = append(successfulResults, res)
		}
	}

	// need at least 2 successful results to mix
	if len(successfulResults) < 2 {
		return &Response{}, nil
	}

	result := &Response{}

	// if consensus enabled, check and attempt consensus
	if req.ConsensusEnabled && len(successfulResults) > 1 {
		cm := consensus.New(m.logger)
		consensusOpts := consensus.Options{
			Enabled:     true,
			Attempts:    req.ConsensusAttempts,
			Prompt:      req.Prompt,
			MixProvider: req.MixProvider,
		}

		consensusReq := consensus.AttemptRequest{
			Options:   consensusOpts,
			Providers: req.Providers,
			Results:   successfulResults,
		}

		consensusResp, consensusErr := cm.Attempt(ctx, consensusReq)
		if consensusErr != nil {
			// log the error but continue with mixing
			m.logger.Logf("[ERROR] consensus checking encountered errors: %v", consensusErr)
			result.ConsensusError = consensusErr
		}
		if consensusResp != nil {
			successfulResults = consensusResp.FinalResults
			result.ConsensusAttempts = consensusResp.Attempts
			result.ConsensusAchieved = consensusResp.Achieved
		}
		// log consensus attempts for transparency
		m.logger.Logf("[INFO] consensus attempts made: %d, achieved: %v", result.ConsensusAttempts, result.ConsensusAchieved)
	}

	// mix the results
	mixReq := mixRequest{
		MixPrompt:   req.MixPrompt,
		MixProvider: req.MixProvider,
		Providers:   req.Providers,
		Results:     successfulResults,
	}

	textWithHeader, rawText, mixProvider, err := m.mixResults(ctx, mixReq)
	if err != nil {
		return nil, err
	}

	result.TextWithHeader = textWithHeader
	result.RawText = rawText
	result.MixProvider = mixProvider

	return result, nil
}

// mixRequest holds parameters for mixing results (internal use)
type mixRequest struct {
	MixPrompt   string
	MixProvider string
	Providers   []provider.Provider
	Results     []provider.Result
}

// mixResults takes multiple provider results and uses a selected provider to mix them
func (m *Manager) mixResults(ctx context.Context, req mixRequest) (textWithHeader, rawText, mixProvider string, err error) {
	// find the mix provider using shared utility
	mixProv := provider.FindProviderByName(req.MixProvider, req.Providers)

	if mixProv == nil {
		return "", "", "", fmt.Errorf("no enabled provider found for mixing results")
	}

	// log if we're using a fallback provider
	if !strings.Contains(strings.ToLower(mixProv.Name()), strings.ToLower(req.MixProvider)) {
		m.logger.Logf("[INFO] specified mix provider '%s' not enabled, falling back to '%s'",
			req.MixProvider, mixProv.Name())
	}

	// build a prompt with all results
	var mixPromptBuilder strings.Builder
	mixPromptBuilder.WriteString(req.MixPrompt)
	mixPromptBuilder.WriteString("\n\n")

	for i, result := range req.Results {
		if result.Error != nil {
			continue
		}
		mixPromptBuilder.WriteString(fmt.Sprintf("=== Result %d from %s ===\n", i+1, result.Provider))
		mixPromptBuilder.WriteString(result.Text)
		mixPromptBuilder.WriteString("\n\n")
	}

	// generate the mixed result
	mixedResult, err := mixProv.Generate(ctx, mixPromptBuilder.String())
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate mixed result using %s: %w", mixProv.Name(), err)
	}

	// return both formatted (with header) and raw versions
	textWithHeader = fmt.Sprintf("== mixed results by %s ==\n%s", mixProv.Name(), mixedResult)
	rawText = mixedResult
	mixProvider = mixProv.Name()
	return textWithHeader, rawText, mixProvider, nil
}
