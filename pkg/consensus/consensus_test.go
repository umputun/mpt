package consensus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/consensus/mocks"
	"github.com/umputun/mpt/pkg/provider"
)

func TestManager_Attempt(t *testing.T) {
	ctx := context.Background()
	manager := New(nil) // will use default logger

	t.Run("consensus reached on first attempt", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					return "YES", nil
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

		opts := Options{
			Enabled:     true,
			Attempts:    2,
			Prompt:      "What is the capital of France?",
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err, "consensus attempt should not fail")
		assert.Equal(t, 1, resp.Attempts, "should reach consensus on first attempt")
		assert.True(t, resp.Achieved, "consensus should be achieved when responses agree")
		assert.Equal(t, results, resp.FinalResults, "results should be unchanged when consensus reached")
	})

	t.Run("consensus not reached", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					return "NO", nil
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
			{Provider: "OpenAI", Text: "Go is the best"},
			{Provider: "Anthropic", Text: "Python is better"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    1,
			Prompt:      "What is the best programming language?",
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err, "consensus attempt should not fail even when no consensus reached")
		assert.Equal(t, 1, resp.Attempts, "should make 1 attempt as configured")
		assert.False(t, resp.Achieved, "consensus should not be achieved when responses disagree")
		assert.Equal(t, results, resp.FinalResults, "results should be unchanged when no further attempts")
	})

	t.Run("consensus not reached with multiple attempts and rerun", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					return "NO", nil
				}
				if strings.Contains(prompt, "Original question") {
					return "Revised response after considering", nil
				}
				return "default response", nil
			},
		}

		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Original question") {
					return "Also revised response", nil
				}
				return "default response", nil
			},
		}

		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Go is the best"},
			{Provider: "Anthropic", Text: "Python is better"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    2, // multiple attempts to trigger rerun
			Prompt:      "What is the best programming language?",
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.Attempts)
		assert.False(t, resp.Achieved)
		// results should be updated with new responses
		assert.NotEqual(t, results, resp.FinalResults)
		assert.Len(t, resp.FinalResults, 2)
		// check that providers were rerun with new prompt
		assert.Contains(t, resp.FinalResults[0].Text, "Revised response")
		assert.Contains(t, resp.FinalResults[1].Text, "revised response")
	})

	t.Run("consensus reached after rerun", func(t *testing.T) {
		attemptNum := 0
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Do the following AI responses fundamentally agree") {
					attemptNum++
					if attemptNum == 1 {
						return "NO", nil
					}
					return "YES", nil
				}
				if strings.Contains(prompt, "Original question") {
					return "Aligned response after reconsideration", nil
				}
				return "default response", nil
			},
		}

		mockAnthropic := &mocks.ProviderMock{
			NameFunc:    func() string { return "Anthropic" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				if strings.Contains(prompt, "Original question") {
					return "Also aligned response", nil
				}
				return "default response", nil
			},
		}

		providers := []provider.Provider{mockOpenAI, mockAnthropic}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Initial disagreement"},
			{Provider: "Anthropic", Text: "Different opinion"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    3,
			Prompt:      "Test question",
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.Attempts) // should succeed on second attempt
		assert.True(t, resp.Achieved)
		// results should be updated from rerun
		assert.NotEqual(t, results, resp.FinalResults)
	})

	t.Run("disabled consensus", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
		}

		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test response"},
		}

		opts := Options{
			Enabled:  false,
			Attempts: 2,
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Attempts)
		assert.False(t, resp.Achieved)
		assert.Equal(t, results, resp.FinalResults)
	})

	t.Run("single result skips consensus", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
		}

		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test response"},
		}

		opts := Options{
			Enabled:  true,
			Attempts: 2,
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.Attempts)
		assert.False(t, resp.Achieved)
		assert.Equal(t, results, resp.FinalResults)
	})

	t.Run("no enabled providers", func(t *testing.T) {
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
			{Provider: "OpenAI", Text: "Test"},
			{Provider: "Anthropic", Text: "Test"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    2,
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		_, err := manager.Attempt(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled providers")
	})

	t.Run("consensus check error", func(t *testing.T) {
		mockOpenAI := &mocks.ProviderMock{
			NameFunc:    func() string { return "OpenAI" },
			EnabledFunc: func() bool { return true },
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("API error")
			},
		}

		providers := []provider.Provider{mockOpenAI}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test"},
			{Provider: "Anthropic", Text: "Test"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    1,
			MixProvider: "openai",
		}

		req := AttemptRequest{
			Options:   opts,
			Providers: providers,
			Results:   results,
		}

		resp, err := manager.Attempt(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consensus checking failed")
		assert.Equal(t, 1, resp.Attempts)
		assert.False(t, resp.Achieved)
		assert.Equal(t, results, resp.FinalResults)
	})
}

func TestManager_findMixProvider(t *testing.T) {
	manager := New(nil)

	mockOpenAI := &mocks.ProviderMock{
		NameFunc:    func() string { return "OpenAI (gpt-4o)" },
		EnabledFunc: func() bool { return true },
	}

	mockAnthropic := &mocks.ProviderMock{
		NameFunc:    func() string { return "Anthropic" },
		EnabledFunc: func() bool { return true },
	}

	mockGoogle := &mocks.ProviderMock{
		NameFunc:    func() string { return "Google" },
		EnabledFunc: func() bool { return false },
	}

	providers := []provider.Provider{mockOpenAI, mockAnthropic, mockGoogle}

	t.Run("exact match", func(t *testing.T) {
		p := manager.findMixProvider("Anthropic", providers)
		require.NotNil(t, p)
		assert.Equal(t, "Anthropic", p.Name())
	})

	t.Run("partial match", func(t *testing.T) {
		p := manager.findMixProvider("openai", providers)
		require.NotNil(t, p)
		assert.Equal(t, "OpenAI (gpt-4o)", p.Name())
	})

	t.Run("disabled provider not returned", func(t *testing.T) {
		p := manager.findMixProvider("Google", providers)
		// should return first enabled provider as fallback
		require.NotNil(t, p)
		assert.Equal(t, "OpenAI (gpt-4o)", p.Name())
	})

	t.Run("no match returns fallback", func(t *testing.T) {
		p := manager.findMixProvider("Claude", providers)
		// should return first enabled provider as fallback
		require.NotNil(t, p)
		assert.Equal(t, "OpenAI (gpt-4o)", p.Name())
	})
}

func TestManager_buildConsensusCheckPrompt(t *testing.T) {
	manager := New(nil)

	results := []provider.Result{
		{Provider: "OpenAI", Text: "Paris is the capital"},
		{Provider: "Anthropic", Text: "The capital is Paris"},
		{Provider: "Google", Error: errors.New("failed")},
	}

	prompt := manager.buildConsensusCheckPrompt(results)

	// check that prompt contains the expected parts
	assert.Contains(t, prompt, "Do the following AI responses fundamentally agree")
	assert.Contains(t, prompt, "Response 1 from OpenAI")
	assert.Contains(t, prompt, "Paris is the capital")
	assert.Contains(t, prompt, "Response 2 from Anthropic")
	assert.Contains(t, prompt, "The capital is Paris")
	assert.NotContains(t, prompt, "Google") // error result should be skipped
	assert.Contains(t, prompt, "Answer:")
}

func TestManager_buildConsensusRerunPrompt(t *testing.T) {
	manager := New(nil)

	originalPrompt := "What is the capital of France?"
	results := []provider.Result{
		{Provider: "OpenAI", Text: "Paris is the capital"},
		{Provider: "Anthropic", Text: "The capital is Paris"},
		{Provider: "Google", Error: errors.New("failed")},
	}

	prompt := manager.buildConsensusRerunPrompt(originalPrompt, results)

	// check that prompt contains the expected parts
	assert.Contains(t, prompt, "Original question:")
	assert.Contains(t, prompt, originalPrompt)
	assert.Contains(t, prompt, "Other AI models provided these different perspectives")
	assert.Contains(t, prompt, "--- OpenAI's response ---")
	assert.Contains(t, prompt, "Paris is the capital")
	assert.Contains(t, prompt, "--- Anthropic's response ---")
	assert.Contains(t, prompt, "The capital is Paris")
	assert.NotContains(t, prompt, "Google") // error result should be skipped
	assert.Contains(t, prompt, "Please reconsider your answer")
}

func TestManager_isConsensusReached(t *testing.T) {
	manager := New(nil)
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		// explicit yes/no
		{"explicit yes", "YES", true},
		{"explicit yes lowercase", "yes", true},
		{"explicit yes with punctuation", "Yes.", true},
		{"explicit yes with more text", "Yes, they agree", true},
		{"explicit no", "NO", false},
		{"explicit no lowercase", "no", false},
		{"explicit no with punctuation", "No!", false},
		{"explicit no with more text", "No, they disagree", false},

		// agreement patterns
		{"responses agree", "The responses agree on the main points", true},
		{"they agree", "Yes, they agree", true},
		{"models agree", "The models agree", true},
		{"consensus reached", "There is consensus", true},
		{"same conclusion", "They reach the same conclusion", true},
		{"similar views", "Very similar views", true},
		{"consistent answers", "The answers are consistent", true},
		{"aligned responses", "The responses are aligned", true},

		// disagreement patterns
		{"disagree", "They disagree", false},
		{"conflict", "There is a conflict", false},
		{"different", "They have different opinions", false},
		{"not agree", "They do not agree", false},
		{"don't agree", "They don't agree", false},
		{"diverge", "The opinions diverge", false},
		{"contradict", "They contradict each other", false},
		{"inconsistent", "The answers are inconsistent", false},

		// edge cases
		{"empty", "", false},
		{"whitespace", "   ", false},
		{"ambiguous", "Maybe", false},
		{"no clear indicator", "The responses are interesting", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isConsensusReached(tt.response)
			assert.Equal(t, tt.expected, result, "Response: %q", tt.response)
		})
	}
}
