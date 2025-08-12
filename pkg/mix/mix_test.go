package mix

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/mix/mocks"
	"github.com/umputun/mpt/pkg/provider"
)

func TestManager_Process(t *testing.T) {
	ctx := context.Background()
	manager := New(nil) // will use default logger

	t.Run("successful mixing without consensus", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "merge results from all providers") {
					return "Here is the merged result", nil
				}
				return "default response", nil
			},
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Result from OpenAI"},
			{Provider: "Anthropic", Text: "Result from Anthropic"},
		}

		req := Request{
			Prompt:            "Test prompt",
			MixPrompt:         "merge results from all providers",
			MixProvider:       "openai",
			ConsensusEnabled:  false,
			ConsensusAttempts: 0,
			Providers:         providers,
			Results:           results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err, "mix processing should succeed with valid providers")
		assert.NotEmpty(t, resp.TextWithHeader, "mixed results should have header")
		assert.Contains(t, resp.TextWithHeader, "== mixed results by OpenAI ==", "should show mix provider in header")
		assert.Contains(t, resp.TextWithHeader, "Here is the merged result", "should contain mixed content")
		assert.Equal(t, "Here is the merged result", resp.RawText, "raw text should match generated result")
		assert.Equal(t, "OpenAI", resp.MixProvider, "should use OpenAI as mix provider")
		assert.False(t, resp.ConsensusAchieved, "consensus not enabled so should be false")
		assert.Equal(t, 0, resp.ConsensusAttempts, "consensus not enabled so attempts should be 0")
	})

	t.Run("consensus error but continues with mixing", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					// simulate error during consensus check
					return "", errors.New("consensus check failed")
				}
				if strings.Contains(prompt, "merge results from all providers") {
					return "Mixed results despite consensus error", nil
				}
				return "default response", nil
			},
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Different opinion 1"},
			{Provider: "Anthropic", Text: "Different opinion 2"},
		}

		req := Request{
			Prompt:            "What is the best language?",
			MixPrompt:         "merge results from all providers",
			MixProvider:       "openai",
			ConsensusEnabled:  true,
			ConsensusAttempts: 1,
			Providers:         providers,
			Results:           results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.TextWithHeader)
		assert.Contains(t, resp.TextWithHeader, "== mixed results by OpenAI ==")
		assert.Contains(t, resp.TextWithHeader, "Mixed results despite consensus error")
		assert.Equal(t, "OpenAI", resp.MixProvider)
		assert.False(t, resp.ConsensusAchieved) // consensus failed due to error
		assert.Equal(t, 1, resp.ConsensusAttempts)
	})

	t.Run("successful mixing with consensus", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					return "YES", nil
				}
				if strings.Contains(prompt, "merge results from all providers") {
					return "Merged consensus results", nil
				}
				return "default response", nil
			},
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Paris is the capital"},
			{Provider: "Anthropic", Text: "The capital is Paris"},
		}

		req := Request{
			Prompt:            "What is the capital of France?",
			MixPrompt:         "merge results from all providers",
			MixProvider:       "openai",
			ConsensusEnabled:  true,
			ConsensusAttempts: 2,
			Providers:         providers,
			Results:           results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.TextWithHeader)
		assert.Contains(t, resp.TextWithHeader, "== mixed results by OpenAI ==")
		assert.Equal(t, "OpenAI", resp.MixProvider)
		assert.True(t, resp.ConsensusAchieved)
		assert.Equal(t, 1, resp.ConsensusAttempts)
	})

	t.Run("insufficient results for mixing", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Single result"},
		}

		req := Request{
			Prompt:      "Test prompt",
			MixPrompt:   "merge results",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err)
		assert.Empty(t, resp.TextWithHeader)
		assert.Empty(t, resp.RawText)
		assert.Empty(t, resp.MixProvider)
	})

	t.Run("all results have errors", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Error: errors.New("API error")},
			{Provider: "Anthropic", Error: errors.New("Network error")},
		}

		req := Request{
			Prompt:      "Test prompt",
			MixPrompt:   "merge results",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err)
		assert.Empty(t, resp.TextWithHeader)
		assert.Empty(t, resp.RawText)
		assert.Empty(t, resp.MixProvider)
	})

	t.Run("mix provider not found", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return false },
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return false },
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Result 1"},
			{Provider: "Anthropic", Text: "Result 2"},
		}

		req := Request{
			Prompt:      "Test prompt",
			MixPrompt:   "merge results",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		_, err := manager.Process(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled provider found for mixing results")
	})

	t.Run("fallback to different provider", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return false },
		}
		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "merge results") {
					return "Fallback mixed result", nil
				}
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Result 1"},
			{Provider: "Anthropic", Text: "Result 2"},
		}

		req := Request{
			Prompt:      "Test prompt",
			MixPrompt:   "merge results",
			MixProvider: "openai", // this provider is disabled
			Providers:   providers,
			Results:     results,
		}

		resp, err := manager.Process(ctx, req)
		require.NoError(t, err)
		assert.Contains(t, resp.TextWithHeader, "== mixed results by Anthropic ==")
		assert.Equal(t, "Anthropic", resp.MixProvider)
		assert.Equal(t, "Fallback mixed result", resp.RawText)
	})

	t.Run("mix provider fails", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("API failure")
			},
		}
		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Result 1"},
			{Provider: "Anthropic", Text: "Result 2"},
		}

		req := Request{
			Prompt:      "Test prompt",
			MixPrompt:   "merge results",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		_, err := manager.Process(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate mixed result")
		assert.Contains(t, err.Error(), "API failure")
	})
}

func TestManager_mixResults(t *testing.T) {
	ctx := context.Background()
	manager := New(nil)

	t.Run("successful mix", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "custom mix prompt") {
					return "Mixed output", nil
				}
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "First result"},
			{Provider: "Anthropic", Text: "Second result"},
		}

		req := mixRequest{
			MixPrompt:   "custom mix prompt",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		textWithHeader, rawText, mixProvider, err := manager.mixResults(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "== mixed results by OpenAI ==\nMixed output", textWithHeader)
		assert.Equal(t, "Mixed output", rawText)
		assert.Equal(t, "OpenAI", mixProvider)
	})

	t.Run("build prompt with multiple results", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Result 1 from OpenAI") {
					return "should match", nil
				}
				if strings.Contains(prompt, "Result 2 from Anthropic") {
					return "should match", nil
				}
				if strings.Contains(prompt, "Result 3 from Google") {
					return "should not match", nil // has error
				}
				return "default response", nil
			},
		}
		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "First result"},
			{Provider: "Anthropic", Text: "Second result"},
			{Provider: "Google", Text: "Third result", Error: errors.New("failed")},
		}

		req := mixRequest{
			MixPrompt:   "merge all",
			MixProvider: "openai",
			Providers:   providers,
			Results:     results,
		}

		textWithHeader, _, _, err := manager.mixResults(ctx, req)
		require.NoError(t, err)
		// verify that error result is skipped
		assert.NotContains(t, textWithHeader, "Google")
	})
}
