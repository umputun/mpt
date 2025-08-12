package consensus

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/provider"
)

// mockProvider implements Provider interface for testing
type mockProvider struct {
	name      string
	enabled   bool
	responses map[string]string
	err       error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Enabled() bool { return m.enabled }
func (m *mockProvider) Generate(_ context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	// check responses map for exact matches
	for key, resp := range m.responses {
		if strings.Contains(prompt, key) {
			return resp, nil
		}
	}
	// default response for unknown prompts
	return "default response", nil
}


func TestManager_Attempt(t *testing.T) {
	ctx := context.Background()
	manager := New(nil) // will use default logger

	t.Run("consensus reached on first attempt", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{
				name:    "OpenAI",
				enabled: true,
				responses: map[string]string{
					"Do the following AI responses fundamentally agree": "YES",
				},
			},
			&mockProvider{name: "Anthropic", enabled: true},
		}

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

		newResults, attempts, achieved, err := manager.Attempt(ctx, opts, providers, results)
		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
		assert.True(t, achieved)
		assert.Equal(t, results, newResults) // results unchanged when consensus reached
	})

	t.Run("consensus not reached", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{
				name:    "OpenAI",
				enabled: true,
				responses: map[string]string{
					"Do the following AI responses fundamentally agree": "NO",
				},
			},
			&mockProvider{name: "Anthropic", enabled: true},
		}

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

		newResults, attempts, achieved, err := manager.Attempt(ctx, opts, providers, results)
		require.NoError(t, err)
		assert.Equal(t, 1, attempts)
		assert.False(t, achieved)
		assert.Equal(t, results, newResults)
	})

	t.Run("disabled consensus", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{name: "OpenAI", enabled: true},
		}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test response"},
		}

		opts := Options{
			Enabled:  false,
			Attempts: 2,
		}

		newResults, attempts, achieved, err := manager.Attempt(ctx, opts, providers, results)
		require.NoError(t, err)
		assert.Equal(t, 0, attempts)
		assert.False(t, achieved)
		assert.Equal(t, results, newResults)
	})

	t.Run("single result skips consensus", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{name: "OpenAI", enabled: true},
		}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test response"},
		}

		opts := Options{
			Enabled:  true,
			Attempts: 2,
		}

		newResults, attempts, achieved, err := manager.Attempt(ctx, opts, providers, results)
		require.NoError(t, err)
		assert.Equal(t, 0, attempts)
		assert.False(t, achieved)
		assert.Equal(t, results, newResults)
	})

	t.Run("no enabled providers", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{name: "OpenAI", enabled: false},
			&mockProvider{name: "Anthropic", enabled: false},
		}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test"},
			{Provider: "Anthropic", Text: "Test"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    2,
			MixProvider: "openai",
		}

		_, _, _, err := manager.Attempt(ctx, opts, providers, results)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled providers")
	})

	t.Run("consensus check error", func(t *testing.T) {
		providers := []provider.Provider{
			&mockProvider{
				name:    "OpenAI",
				enabled: true,
				err:     errors.New("API error"),
			},
		}

		results := []provider.Result{
			{Provider: "OpenAI", Text: "Test"},
			{Provider: "Anthropic", Text: "Test"},
		}

		opts := Options{
			Enabled:     true,
			Attempts:    1,
			MixProvider: "openai",
		}

		newResults, attempts, achieved, err := manager.Attempt(ctx, opts, providers, results)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consensus checking failed")
		assert.Equal(t, 1, attempts)
		assert.False(t, achieved)
		assert.Equal(t, results, newResults)
	})
}

func TestManager_findMixProvider(t *testing.T) {
	manager := New(nil)

	providers := []provider.Provider{
		&mockProvider{name: "OpenAI (gpt-4o)", enabled: true},
		&mockProvider{name: "Anthropic", enabled: true},
		&mockProvider{name: "Google", enabled: false},
	}

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
		assert.Nil(t, p)
	})

	t.Run("no match", func(t *testing.T) {
		p := manager.findMixProvider("Claude", providers)
		assert.Nil(t, p)
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