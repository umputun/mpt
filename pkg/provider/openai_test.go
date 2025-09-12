package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAI_Name(t *testing.T) {
	provider := NewOpenAI(Options{})
	assert.Equal(t, "OpenAI", provider.Name())
}

func TestOpenAI_Enabled(t *testing.T) {
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
				Model:   "gpt-test",
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOpenAI(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestOpenAI_Generate_NotEnabled(t *testing.T) {
	provider := NewOpenAI(Options{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

// mockOpenAIClient creates a custom OpenAI client that uses a test server
func mockOpenAIClient(t *testing.T, jsonResponse string) (*openai.Client, *httptest.Server) {
	// create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(jsonResponse))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	}))

	// create a custom client configuration
	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	return client, server
}

func TestOpenAI_Generate_Success(t *testing.T) {
	// create a response message in the format expected by the API
	jsonResponse := `
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "This is a test response."
      },
      "finish_reason": "stop",
      "index": 0
    }
  ]
}
`

	// create a client with the mock server
	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	// test the Generate method
	resp, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response.", resp)
}

func TestOpenAI_Generate_EmptyResponse(t *testing.T) {
	// create a response with no choices
	jsonResponse := `{"choices": []}`

	// create a client with the mock server
	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	// test the Generate method
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	// should return a sanitized error since our error might trigger the sanitizer
	assert.Contains(t, err.Error(), "openai returned no choices")
}

func TestOpenAI_Generate_APIError(t *testing.T) {
	// create a mock server directly to simulate API error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{
			"error": {
				"message": "Invalid API key",
				"type": "invalid_request_error",
				"param": null,
				"code": "invalid_api_key"
			}
		}`))
		_ = err
	}))
	defer server.Close()

	// create a custom client configuration
	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	// create a provider with the mock client
	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	// test the Generate method
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	// check for key elements in the error
	errorMsg := strings.ToLower(err.Error())

	// API error should be mentioned
	assert.Contains(t, errorMsg, "api error", "Error should mention API error")

	// at least one of: provider name or guidance should be present
	assert.True(t,
		strings.Contains(errorMsg, "openai") ||
			strings.Contains(errorMsg, "check") ||
			strings.Contains(errorMsg, "redacted"),
		"Error should contain at least one helpful element: provider name, guidance, or redaction notice")
}

func TestOpenAI_Generate_401AuthError(t *testing.T) {
	// create a mock server to simulate 401 authentication error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Incorrect API key provided",
				"type": "invalid_request_error",
				"param": null,
				"code": "invalid_api_key"
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Contains(t, err.Error(), "openai api error")
}

func TestOpenAI_Generate_429RateLimitError(t *testing.T) {
	// create a mock server to simulate 429 rate limit error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Rate limit reached for requests",
				"type": "rate_limit_error",
				"param": null,
				"code": "rate_limit_exceeded"
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Contains(t, err.Error(), "openai api error")
}

func TestOpenAI_Generate_ModelNotFoundError(t *testing.T) {
	// create a mock server to simulate model not found error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "The model 'gpt-5' does not exist",
				"type": "invalid_request_error",
				"param": "model",
				"code": "model_not_found"
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-5",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model issue")
	assert.Contains(t, err.Error(), "check if model exists")
}

func TestOpenAI_Generate_TimeoutError(t *testing.T) {
	// create a mock server that simulates timeout by not responding
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// simulate timeout by blocking forever
		select {}
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	// create a context with immediate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	// check that it's categorized as timeout
	assert.Contains(t, err.Error(), "request timed out")
}

func TestOpenAI_Generate_ContextLengthError(t *testing.T) {
	// create a mock server to simulate context length exceeded error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "This model's maximum context length is 4097 tokens. However, your messages resulted in 5000 tokens.",
				"type": "invalid_request_error",
				"param": "messages",
				"code": "context_length_exceeded"
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-3.5-turbo",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "very long prompt")
	require.Error(t, err)
	// the error detection looks for "model" first, so it categorizes as model issue
	assert.Contains(t, err.Error(), "model issue")
}

func TestOpenAI_Generate_GenericError(t *testing.T) {
	// create a mock server to simulate generic server error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Internal server error",
				"type": "server_error",
				"param": null,
				"code": null
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai api error")
	// should not contain specific categorization
	assert.NotContains(t, err.Error(), "authentication")
	assert.NotContains(t, err.Error(), "rate limit")
	assert.NotContains(t, err.Error(), "model issue")
}

func TestOpenAI_Generate_ContextCancellation(t *testing.T) {
	// create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := NewOpenAI(Options{
		APIKey:  "test-key",
		Model:   "gpt-4",
		Enabled: true,
	})

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestOpenAI_Generate_MaxTokensEdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		maxTokens         int
		expectedMaxTokens int // what should be sent to the API
	}{
		{
			name:              "zero max tokens passes zero",
			maxTokens:         0,
			expectedMaxTokens: 0, // openAI passes 0 directly, unlike Google
		},
		{
			name:              "positive max tokens",
			maxTokens:         500,
			expectedMaxTokens: 500,
		},
		{
			name:              "negative max tokens passes negative",
			maxTokens:         -100,
			expectedMaxTokens: -100, // openAI passes negative values directly
		},
		{
			name:              "very large max tokens",
			maxTokens:         1000000,
			expectedMaxTokens: 1000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create a mock server that validates the request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// decode and verify the request body
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)

				var reqBody map[string]interface{}
				err = json.Unmarshal(body, &reqBody)
				assert.NoError(t, err)

				// check max_tokens in the request
				if tt.expectedMaxTokens == 0 {
					// when maxTokens is 0, OpenAI SDK might omit it from the request
					maxTokens, hasMaxTokens := reqBody["max_tokens"]
					if hasMaxTokens {
						// if it's present, it should be 0
						assert.InEpsilon(t, float64(0), maxTokens.(float64), 0.0001)
					}
					// either way is acceptable for zero
				} else {
					// for non-zero values, max_tokens must be present
					maxTokens, ok := reqBody["max_tokens"].(float64)
					assert.True(t, ok, "max_tokens not found in request")
					assert.InEpsilon(t, float64(tt.expectedMaxTokens), maxTokens, 0.0001)
				}

				// return successful response
				response := `{
					"choices": [{
						"message": {
							"role": "assistant",
							"content": "Response with max tokens"
						},
						"finish_reason": "stop",
						"index": 0
					}]
				}`

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write([]byte(response))
				assert.NoError(t, err)
			}))
			defer server.Close()

			// create client with custom server
			config := openai.DefaultConfig("test-key")
			config.BaseURL = server.URL
			client := openai.NewClientWithConfig(config)

			provider := &OpenAI{
				client:    client,
				model:     "gpt-4",
				enabled:   true,
				maxTokens: tt.maxTokens,
			}

			resp, err := provider.Generate(context.Background(), "test prompt")
			require.NoError(t, err)
			assert.Equal(t, "Response with max tokens", resp)
		})
	}
}

func TestOpenAI_NewOpenAI_EdgeCases(t *testing.T) {
	tests := []struct {
		name                string
		options             Options
		expectedEnabled     bool
		expectedMaxTokens   int
		expectedTemperature float32
	}{
		{
			name: "negative_temperature_sets_default",
			options: Options{
				APIKey:      "test-key",
				Model:       "gpt-4",
				Enabled:     true,
				Temperature: -0.5,
				MaxTokens:   100,
			},
			expectedEnabled:     true,
			expectedMaxTokens:   100,
			expectedTemperature: DefaultTemperature,
		},
		{
			name: "zero_temperature_preserved",
			options: Options{
				APIKey:      "test-key",
				Model:       "gpt-4",
				Enabled:     true,
				Temperature: 0,
				MaxTokens:   100,
			},
			expectedEnabled:     true,
			expectedMaxTokens:   100,
			expectedTemperature: 0, // zero temperature is preserved for deterministic output
		},
		{
			name: "positive_temperature_preserved",
			options: Options{
				APIKey:      "test-key",
				Model:       "gpt-4",
				Enabled:     true,
				Temperature: 0.9,
				MaxTokens:   100,
			},
			expectedEnabled:     true,
			expectedMaxTokens:   100,
			expectedTemperature: 0.9,
		},
		{
			name: "negative_max_tokens_sets_default",
			options: Options{
				APIKey:      "test-key",
				Model:       "gpt-4",
				Enabled:     true,
				Temperature: 0.7,
				MaxTokens:   -100,
			},
			expectedEnabled:     true,
			expectedMaxTokens:   DefaultMaxTokens,
			expectedTemperature: 0.7,
		},
		{
			name: "zero_max_tokens_preserved",
			options: Options{
				APIKey:      "test-key",
				Model:       "gpt-4",
				Enabled:     true,
				Temperature: 0.7,
				MaxTokens:   0,
			},
			expectedEnabled:     true,
			expectedMaxTokens:   0, // 0 means use model's maximum
			expectedTemperature: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOpenAI(tt.options)
			assert.Equal(t, tt.expectedEnabled, provider.Enabled())
			assert.Equal(t, tt.expectedMaxTokens, provider.maxTokens)
			// use exact comparison for 0, epsilon for non-zero values
			if tt.expectedTemperature == 0 {
				assert.Equal(t, tt.expectedTemperature, provider.temperature) //nolint:testifylint // need exact comparison for 0
			} else {
				assert.InEpsilon(t, tt.expectedTemperature, provider.temperature, 0.0001)
			}
		})
	}
}

func TestOpenAI_Generate_EmptyPrompt(t *testing.T) {
	jsonResponse := `
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Response to empty prompt"
      },
      "finish_reason": "stop",
      "index": 0
    }
  ]
}
`

	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	resp, err := provider.Generate(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "Response to empty prompt", resp)
}

func TestOpenAI_Generate_MultipleChoices(t *testing.T) {
	// test that we only use the first choice
	jsonResponse := `
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "First choice"
      },
      "finish_reason": "stop",
      "index": 0
    },
    {
      "message": {
        "role": "assistant",
        "content": "Second choice"
      },
      "finish_reason": "stop",
      "index": 1
    }
  ]
}
`

	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	resp, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "First choice", resp)
}

func TestOpenAI_Generate_DifferentFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expected     string
	}{
		{
			name:         "finish_reason_stop",
			finishReason: "stop",
			expected:     "Normal completion",
		},
		{
			name:         "finish_reason_length",
			finishReason: "length",
			expected:     "Hit max tokens",
		},
		{
			name:         "finish_reason_content_filter",
			finishReason: "content_filter",
			expected:     "Content filtered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonResponse := fmt.Sprintf(`
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "%s"
      },
      "finish_reason": "%s",
      "index": 0
    }
  ]
}
`, tt.expected, tt.finishReason)

			client, server := mockOpenAIClient(t, jsonResponse)
			defer server.Close()

			provider := &OpenAI{
				client:  client,
				model:   "gpt-4",
				enabled: true,
			}

			resp, err := provider.Generate(context.Background(), "test prompt")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, resp)
		})
	}
}

func TestOpenAI_Generate_SpecialCharactersInResponse(t *testing.T) {
	jsonResponse := `
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>"
      },
      "finish_reason": "stop",
      "index": 0
    }
  ]
}
`

	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	resp, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>", resp)
}

func TestOpenAI_Generate_NetworkError(t *testing.T) {
	// create a server that immediately closes the connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// close the connection without sending response
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server doesn't support hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-4",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai api error")
}

func TestOpenAI_Generate_PureContextLengthError(t *testing.T) {
	// create a mock server to simulate context length error without "model" in message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Maximum context length exceeded. Your messages resulted in 5000 tokens.",
				"type": "invalid_request_error",
				"param": "messages",
				"code": "context_length_exceeded"
			}
		}`))
	}))
	defer server.Close()

	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	provider := &OpenAI{
		client:  client,
		model:   "gpt-3.5-turbo",
		enabled: true,
	}

	_, err := provider.Generate(context.Background(), "very long prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context length/token limit")
}
