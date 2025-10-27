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

func TestManager_containsNegatedAgreement(t *testing.T) {
	manager := New(nil)
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		// should match negated agreement patterns (means disagreement)
		{"don't agree", "they don't agree", true},
		{"doesn't agree", "it doesn't agree", true},
		{"don't align", "views don't align", true},
		{"doesn't align", "this doesn't align", true},
		{"don't concur", "experts don't concur", true},
		{"doesn't concur", "opinion doesn't concur", true},
		{"not the same", "they are not the same", true},
		{"not similar", "responses are not similar", true},
		{"not consistent", "answers are not consistent", true},
		{"not aligned", "views are not aligned", true},
		{"no agreement", "there is no agreement", true},
		{"no consensus", "there is no consensus", true},

		// should NOT match regular agreement (without negation)
		{"agree", "they agree", false},
		{"aligned", "responses are aligned", false},
		{"same", "they are the same", false},

		// should NOT match unrelated text
		{"no match", "the sky is blue", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.containsNegatedAgreement(tt.response)
			assert.Equal(t, tt.expected, result, "Response: %q", tt.response)
		})
	}
}

func TestManager_containsNegatedDisagreement(t *testing.T) {
	manager := New(nil)
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		// should match negated disagreement patterns
		{"not different", "they are not different", true},
		{"not significantly different", "not significantly different", true},
		{"don't disagree", "they don't disagree", true},
		{"doesn't disagree", "it doesn't disagree", true},
		{"not in conflict", "responses are not in conflict", true},
		{"don't conflict", "answers don't conflict", true},
		{"doesn't conflict", "this doesn't conflict", true},
		{"don't contradict", "they don't contradict", true},
		{"doesn't contradict", "it doesn't contradict", true},
		{"don't diverge", "they don't diverge", true},
		{"doesn't diverge", "opinion doesn't diverge", true},
		{"don't differ", "views don't differ", true},
		{"doesn't differ", "answer doesn't differ", true},
		{"no disagreement", "there is no disagreement", true},
		{"no conflict", "no conflict exists", true},
		{"no contradiction", "there's no contradiction", true},
		{"no significant difference", "no significant difference", true},
		{"not contradictory", "not contradictory", true},
		{"not opposing", "not opposing", true},
		{"not inconsistent", "not inconsistent", true},

		// should NOT match regular disagreement (without negation)
		{"disagree", "they disagree", false},
		{"different", "they are different", false},
		{"conflict", "there is conflict", false},
		{"contradict", "they contradict", false},

		// should NOT match unrelated text
		{"no match", "the sky is blue", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.containsNegatedDisagreement(tt.response)
			assert.Equal(t, tt.expected, result, "Response: %q", tt.response)
		})
	}
}

func TestManager_containsNegativeIndicator(t *testing.T) {
	manager := New(nil)
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		// should match negative indicators with word boundaries
		{"disagree", "they disagree", true},
		{"conflict", "there is a conflict", true},
		{"different", "they are different", true},
		{"diverge", "opinions diverge", true},
		{"contradict", "they contradict", true},
		{"oppose", "they oppose", true},
		{"inconsistent", "results are inconsistent", true},
		{"vary", "answers vary", true},
		{"differ", "views differ", true},

		// should NOT match substrings (word boundary test)
		{"indifferent", "they are indifferent", false},
		{"preferred", "the preferred option", false},

		// should NOT match unrelated text
		{"no match", "the sky is blue", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.containsNegativeIndicator(tt.response)
			assert.Equal(t, tt.expected, result, "Response: %q", tt.response)
		})
	}
}

func TestManager_containsPositiveIndicator(t *testing.T) {
	manager := New(nil)
	tests := []struct {
		name     string
		response string
		expected bool
	}{
		// should match positive indicators with word boundaries
		{"agree", "they agree", true},
		{"consensus", "there is consensus", true},
		{"same", "they have the same view", true},
		{"similar", "very similar", true},
		{"consistent", "answers are consistent", true},
		{"align", "views align", true},
		{"concur", "experts concur", true},
		{"unanimous", "decision is unanimous", true},
		{"accord", "in accord", true},
		{"harmony", "in harmony", true},
		{"unified", "unified position", true},

		// specific agreement patterns
		{"responses agree", "the responses agree", true},
		{"they agree", "they agree on this", true},
		{"models agree", "all models agree", true},
		{"answers agree", "the answers agree", true},
		{"providers agree", "providers agree", true},
		{"all agree", "all agree", true},

		// should NOT match substrings
		{"disagree contains agree", "they disagree", false},

		// should NOT match unrelated text
		{"no match", "the sky is blue", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.containsPositiveIndicator(tt.response)
			assert.Equal(t, tt.expected, result, "Response: %q", tt.response)
		})
	}
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
		{"don't agree", "They don't agree", false},
		{"diverge", "The opinions diverge", false},
		{"contradict", "They contradict each other", false},
		{"inconsistent", "The answers are inconsistent", false},

		// negation patterns (double negative = positive/consensus)
		{"not different", "They are not different", true},
		{"not significantly different", "They are not significantly different", true},
		{"don't disagree", "They don't disagree", true},
		{"doesn't disagree", "The second response doesn't disagree with the first", true},
		{"not in conflict", "The responses are not in conflict", true},
		{"don't conflict", "The answers don't conflict", true},
		{"doesn't conflict", "This doesn't conflict with that", true},
		{"don't contradict", "They don't contradict each other", true},
		{"doesn't contradict", "Response A doesn't contradict response B", true},
		{"don't diverge", "The opinions don't diverge", true},
		{"doesn't diverge", "The conclusion doesn't diverge", true},
		{"don't differ", "They don't differ on this point", true},
		{"doesn't differ", "The answer doesn't differ", true},
		{"no disagreement", "There is no disagreement", true},
		{"no conflict", "There is no conflict between them", true},
		{"no contradiction", "There is no contradiction", true},
		{"no significant difference", "There is no significant difference", true},
		{"not contradictory", "The responses are not contradictory", true},
		{"not opposing", "They are not opposing views", true},
		{"not inconsistent", "The answers are not inconsistent", true},

		// word boundary tests (should not match substrings)
		{"indifferent should not match", "They are indifferent to the question", false},
		{"different should match", "They are different", false},
		{"agreement should match", "There is agreement", true},
		{"disagreement should match", "There is disagreement", false},
		{"disagreeable should match as negative", "They are disagreeable", false},
		{"dissimilar should match as negative", "Views are dissimilar", false},

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
