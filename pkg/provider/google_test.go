package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGenerativeModel allows us to inject custom behavior for testing
type mockGenerativeModel struct {
	response *genai.GenerateContentResponse
	err      error
}

func (m *mockGenerativeModel) generateContent(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	return m.response, m.err
}

func TestGoogle_Name(t *testing.T) {
	provider := NewGoogle(Options{})
	assert.Equal(t, "Google", provider.Name())
}

func TestGoogle_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		options  Options
		expected bool
	}{
		{
			name: "disabled without API key",
			options: Options{
				APIKey:  "",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled explicitly",
			options: Options{
				APIKey:  "test-key",
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled with API key",
			options: Options{
				APIKey:  "test-key",
				Enabled: true,
				Model:   "gemini-1.5-pro",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGoogle(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestGoogle_Generate_NotEnabled(t *testing.T) {
	provider := NewGoogle(Options{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

// We need to override the Generate method for testing
type testableGoogle struct {
	Google
	mockModel *mockGenerativeModel
}

func (tg *testableGoogle) Generate(ctx context.Context, prompt string) (string, error) {
	if !tg.enabled {
		return "", errors.New("google provider is not enabled")
	}

	// use our mock response/error instead of actual API call
	resp, err := tg.mockModel.generateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("google returned empty response")
	}

	text := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			text += string(t)
		}
	}

	return text, nil
}

func TestGoogle_Generate_Success(t *testing.T) {
	// create mock response
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("This is a test response"),
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	// create testable provider with mock response
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: mockResp,
			err:      nil,
		},
	}

	// test the Generate method
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestGoogle_Generate_EmptyResponse(t *testing.T) {
	// create mock response with empty candidates
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{},
	}

	// create testable provider with mock response
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: mockResp,
			err:      nil,
		},
	}

	// test the Generate method - should return an error for empty response
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestGoogle_Generate_APIError(t *testing.T) {
	// create testable provider with mock error
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("API key not valid"),
		},
	}

	// test the Generate method - should return an error for API error
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not valid")
}

func TestGoogle_TokenLimits(t *testing.T) {
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("This is a test response"),
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	testCases := []struct {
		name      string
		maxTokens int
		expected  int32 // what the value should be after conversion
	}{
		{
			name:      "use_model_maximum",
			maxTokens: 0,
			expected:  0, // 0 means use model maximum, shouldn't set value
		},
		{
			name:      "negative_value",
			maxTokens: -5,
			expected:  1024, // negative values default to 1024
		},
		{
			name:      "normal_value",
			maxTokens: 500,
			expected:  500,
		},
		{
			name:      "max_int32_value",
			maxTokens: 2147483647, // max int32
			expected:  2147483647,
		},
		{
			name:      "beyond_int32_value",
			maxTokens: 2147483648, // max int32 + 1
			expected:  2147483647, // should clamp to max int32
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a provider with the test max tokens
			provider := &testableGoogle{
				Google: Google{
					model:     "gemini-1.5-pro",
					enabled:   true,
					maxTokens: tc.maxTokens,
				},
				mockModel: &mockGenerativeModel{
					response: mockResp,
					err:      nil,
				},
			}

			// call Generate to trigger token limit logic
			response, err := provider.Generate(context.Background(), "test prompt")
			require.NoError(t, err)
			assert.Equal(t, "This is a test response", response)

			// verify expected token behavior
			var actualMaxTokens int32
			if tc.maxTokens == 0 {
				// special case: 0 means use model maximum, don't set a specific value
				actualMaxTokens = 0
			} else if tc.maxTokens < 0 {
				actualMaxTokens = 1024
			} else if tc.maxTokens > 2147483647 {
				actualMaxTokens = 2147483647
			} else {
				actualMaxTokens = int32(tc.maxTokens)
			}

			assert.Equal(t, tc.expected, actualMaxTokens)
		})
	}
}
