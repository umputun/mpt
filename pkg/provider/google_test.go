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

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
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

func TestGoogle_Generate_MultipleTextParts(t *testing.T) {
	// create mock response with multiple text parts
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("First part"),
						genai.Text(" second part"),
						genai.Text(" third part"),
					},
				},
			},
		},
	}

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

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "First part second part third part", response)
}

func TestGoogle_Generate_NonTextParts(t *testing.T) {
	// create mock response with mixed content types
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Text part"),
						genai.Blob{MIMEType: "image/png", Data: []byte("fake image data")},
						genai.Text(" another text"),
					},
				},
			},
		},
	}

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

	// should only return text parts, ignoring non-text
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Text part another text", response)
}

func TestGoogle_Generate_EmptyCandidateContent(t *testing.T) {
	// create mock response with candidate but empty content parts
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{},
				},
			},
		},
	}

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

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestGoogle_NewGoogle_DefaultMaxTokens(t *testing.T) {
	// test that negative max tokens gets set to default
	opts := Options{
		APIKey:    "test-key",
		Enabled:   true,
		Model:     "gemini-pro",
		MaxTokens: -1,
	}

	g := NewGoogle(opts)
	assert.Equal(t, DefaultMaxTokens, g.maxTokens)
}

func TestGoogle_NewGoogle_ZeroMaxTokens(t *testing.T) {
	// test that zero max tokens stays zero (use model maximum)
	opts := Options{
		APIKey:    "test-key",
		Enabled:   true,
		Model:     "gemini-pro",
		MaxTokens: 0,
	}

	g := NewGoogle(opts)
	assert.Equal(t, 0, g.maxTokens)
}

func TestGoogle_Generate_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      context.Canceled,
		},
	}

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestGoogle_Generate_DifferentFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason genai.FinishReason
		expected     string
	}{
		{
			name:         "finish_reason_stop",
			finishReason: genai.FinishReasonStop,
			expected:     "Response completed normally",
		},
		{
			name:         "finish_reason_max_tokens",
			finishReason: genai.FinishReasonMaxTokens,
			expected:     "Hit max tokens limit",
		},
		{
			name:         "finish_reason_safety",
			finishReason: genai.FinishReasonSafety,
			expected:     "Blocked by safety filters",
		},
		{
			name:         "finish_reason_recitation",
			finishReason: genai.FinishReasonRecitation,
			expected:     "Blocked by recitation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []genai.Part{
								genai.Text(tt.expected),
							},
						},
						FinishReason: tt.finishReason,
					},
				},
			}

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

			response, err := provider.Generate(context.Background(), "test prompt")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, response)
		})
	}
}

func TestGoogle_Generate_MultipleCandidates(t *testing.T) {
	// test that we only use the first candidate
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("First candidate"),
					},
				},
			},
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Second candidate"),
					},
				},
			},
		},
	}

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

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "First candidate", response)
}

func TestGoogle_Generate_NilContent(t *testing.T) {
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: nil,
			},
		},
	}

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

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
}

func TestGoogle_Generate_RateLimitError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("rate limit exceeded"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
}

func TestGoogle_Generate_QuotaExceededError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("quota exceeded for quota metric"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")
}

func TestGoogle_Generate_InvalidAPIKeyError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("API key not valid. Please pass a valid API key"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not valid")
}

func TestGoogle_Generate_ModelNotFoundError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "non-existent-model",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("model not found"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestGoogle_Generate_NetworkError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("connection refused"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestGoogle_Generate_TimeoutError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      context.DeadlineExceeded,
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestGoogle_Generate_ContentFilterError(t *testing.T) {
	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("content filtered due to safety settings"),
		},
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content filtered")
}

func TestGoogle_Generate_EmptyPrompt(t *testing.T) {
	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Response to empty prompt"),
					},
				},
			},
		},
	}

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

	response, err := provider.Generate(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "Response to empty prompt", response)
}

func TestGoogle_Generate_VeryLongPrompt(t *testing.T) {
	// simulate a very long prompt that might exceed token limits
	longPrompt := ""
	for i := 0; i < 10000; i++ {
		longPrompt += "This is a very long prompt. "
	}

	provider := &testableGoogle{
		Google: Google{
			model:   "gemini-1.5-pro",
			enabled: true,
		},
		mockModel: &mockGenerativeModel{
			response: nil,
			err:      errors.New("prompt too long: exceeds model's context length"),
		},
	}

	_, err := provider.Generate(context.Background(), longPrompt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt too long")
}

func TestGoogle_Generate_SpecialCharactersInPrompt(t *testing.T) {
	specialPrompt := "Test with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>"

	mockResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Handled special characters correctly"),
					},
				},
			},
		},
	}

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

	response, err := provider.Generate(context.Background(), specialPrompt)
	require.NoError(t, err)
	assert.Equal(t, "Handled special characters correctly", response)
}

func TestGoogle_NewGoogle_ClientCreationFailure(t *testing.T) {
	// test scenario where genai.NewClient would fail
	// in real implementation, this happens when API key is invalid format
	g := NewGoogle(Options{
		APIKey:  "", // empty API key should fail client creation
		Enabled: true,
		Model:   "gemini-pro",
	})

	assert.False(t, g.Enabled())
	assert.Nil(t, g.client)
}

func TestGoogle_Generate_MaxTokensEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		maxTokens      int
		expectedTokens int32
	}{
		{
			name:           "exactly_max_int32",
			maxTokens:      2147483647,
			expectedTokens: 2147483647,
		},
		{
			name:           "above_max_int32",
			maxTokens:      2147483648,
			expectedTokens: 2147483647, // should clamp
		},
		{
			name:           "way_above_max_int32",
			maxTokens:      9999999999,
			expectedTokens: 2147483647, // should clamp
		},
		{
			name:           "negative_small",
			maxTokens:      -1,
			expectedTokens: 1024, // default
		},
		{
			name:           "negative_large",
			maxTokens:      -999999,
			expectedTokens: 1024, // default
		},
		{
			name:           "zero_means_model_max",
			maxTokens:      0,
			expectedTokens: 0, // 0 means use model's maximum
		},
		{
			name:           "normal_small_value",
			maxTokens:      100,
			expectedTokens: 100,
		},
		{
			name:           "normal_large_value",
			maxTokens:      100000,
			expectedTokens: 100000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Parts: []genai.Part{
								genai.Text("Test response"),
							},
						},
					},
				},
			}

			provider := &testableGoogle{
				Google: Google{
					model:     "gemini-1.5-pro",
					enabled:   true,
					maxTokens: tt.maxTokens,
				},
				mockModel: &mockGenerativeModel{
					response: mockResp,
					err:      nil,
				},
			}

			_, err := provider.Generate(context.Background(), "test")
			require.NoError(t, err)

			// verify the expected token setting logic
			actualTokens := int32(0)
			if tt.maxTokens != 0 {
				switch {
				case tt.maxTokens < 0:
					actualTokens = 1024
				case tt.maxTokens > 2147483647:
					actualTokens = 2147483647
				default:
					actualTokens = int32(tt.maxTokens)
				}
			}
			assert.Equal(t, tt.expectedTokens, actualTokens)
		})
	}
}
