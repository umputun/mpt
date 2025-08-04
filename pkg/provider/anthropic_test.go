package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropic_Name(t *testing.T) {
	provider := NewAnthropic(Options{})
	assert.Equal(t, "Anthropic", provider.Name())
}

func TestAnthropic_Enabled(t *testing.T) {
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
				Model:   "claude-test",
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewAnthropic(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestAnthropic_Generate_NotEnabled(t *testing.T) {
	provider := NewAnthropic(Options{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

func TestAnthropic_Generate_Success(t *testing.T) {
	// create a test server that returns a successful response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify the request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "messages")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("X-Api-Key"), "test-key")

		// return a successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "This is a test response"
				}
			],
			"model": "claude-3-sonnet-20240229",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 5,
				"output_tokens": 10
			}
		}`
		_, err := w.Write([]byte(response))
		_ = err
	}))
	defer server.Close()

	// create a test client that points to our test server
	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	// create a provider that uses the test client
	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	// test the Generate method
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestAnthropic_Generate_EmptyResponse(t *testing.T) {
	// create a test server that returns an empty response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [],
			"model": "claude-3-sonnet-20240229",
			"stop_reason": "end_turn",
			"usage": {
				"input_tokens": 5,
				"output_tokens": 0
			}
		}`
		_, err := w.Write([]byte(response))
		_ = err
	}))
	defer server.Close()

	// create a test client that points to our test server
	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	// create a provider that uses the test client
	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	// test the Generate method - should return an error for empty response
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestAnthropic_Generate_APIError(t *testing.T) {
	// create a test server that returns an error response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		response := `{
			"error": {
				"type": "authentication_error",
				"message": "Invalid API key"
			}
		}`
		_, err := w.Write([]byte(response))
		_ = err
	}))
	defer server.Close()

	// create a test client that points to our test server
	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	// create a provider that uses the test client
	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	// test the Generate method - should return an error for API error
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	// should contain provider name and api error
	assert.Contains(t, err.Error(), "anthropic api error", "Error should mention provider and API error")
	// should contain the actual error details (no longer redacted)
	assert.Contains(t, err.Error(), "401 Unauthorized", "Error should contain actual status")
	assert.Contains(t, err.Error(), "Invalid API key", "Error should contain actual error message")

}

func TestAnthropic_NewAnthropic_EdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		options           Options
		expectedEnabled   bool
		expectedMaxTokens int
	}{
		{
			name: "negative_max_tokens_sets_default",
			options: Options{
				APIKey:    "test-key",
				Model:     "claude-3-sonnet",
				Enabled:   true,
				MaxTokens: -100,
			},
			expectedEnabled:   true,
			expectedMaxTokens: DefaultMaxTokens,
		},
		{
			name: "zero_max_tokens_preserved",
			options: Options{
				APIKey:    "test-key",
				Model:     "claude-3-sonnet",
				Enabled:   true,
				MaxTokens: 0,
			},
			expectedEnabled:   true,
			expectedMaxTokens: 0, // 0 means use model's maximum
		},
		{
			name: "positive_max_tokens_preserved",
			options: Options{
				APIKey:    "test-key",
				Model:     "claude-3-sonnet",
				Enabled:   true,
				MaxTokens: 2048,
			},
			expectedEnabled:   true,
			expectedMaxTokens: 2048,
		},
		{
			name: "empty_api_key_disabled",
			options: Options{
				APIKey:    "",
				Model:     "claude-3-sonnet",
				Enabled:   true,
				MaxTokens: 1024,
			},
			expectedEnabled:   false,
			expectedMaxTokens: 0,
		},
		{
			name: "empty_model_disabled",
			options: Options{
				APIKey:    "test-key",
				Model:     "",
				Enabled:   true,
				MaxTokens: 1024,
			},
			expectedEnabled:   false,
			expectedMaxTokens: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewAnthropic(tt.options)
			assert.Equal(t, tt.expectedEnabled, provider.Enabled())
			if tt.expectedEnabled {
				assert.Equal(t, tt.expectedMaxTokens, provider.maxTokens)
				assert.NotNil(t, provider.client)
			}
		})
	}
}

func TestAnthropic_Generate_MultipleTextBlocks(t *testing.T) {
	// create a test server that returns multiple text blocks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "First part"
				},
				{
					"type": "text",
					"text": " second part"
				},
				{
					"type": "text",
					"text": " third part"
				}
			],
			"model": "claude-3-sonnet-20240229"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "First part second part third part", response)
}

func TestAnthropic_Generate_NonTextContent(t *testing.T) {
	// create a test server that returns mixed content types
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Text content"
				},
				{
					"type": "tool_use",
					"id": "tool_123",
					"name": "calculator",
					"input": {"expression": "2+2"}
				},
				{
					"type": "text",
					"text": " more text"
				}
			],
			"model": "claude-3-sonnet-20240229"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	// should only return text content, ignoring non-text blocks
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Text content more text", response)
}

func TestAnthropic_Generate_RateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		response := `{
			"error": {
				"type": "rate_limit_error",
				"message": "Rate limit exceeded"
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic api error")
	assert.Contains(t, err.Error(), "429")
}

func TestAnthropic_Generate_InvalidModelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		response := `{
			"error": {
				"type": "invalid_request_error",
				"message": "Invalid model: claude-4-opus does not exist"
			}
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-4-opus",
		enabled:   true,
		maxTokens: 1024,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic api error")
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "does not exist")
}

func TestAnthropic_Generate_ContextCancellation(t *testing.T) {
	// create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := NewAnthropic(Options{
		APIKey:  "test-key",
		Model:   "claude-3-sonnet",
		Enabled: true,
	})

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestAnthropic_Generate_EmptyPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Response to empty prompt"
				}
			],
			"model": "claude-3-sonnet-20240229"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	response, err := provider.Generate(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "Response to empty prompt", response)
}

func TestAnthropic_Generate_LargeMaxTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Response with large max tokens"
				}
			],
			"model": "claude-3-sonnet-20240229"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 100000, // very large value to test int64 conversion
	}

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Response with large max tokens", response)
}

func TestAnthropic_Generate_SpecialCharactersInResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := `{
			"id": "msg_123",
			"type": "message",
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>"
				}
			],
			"model": "claude-3-sonnet-20240229"
		}`
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>", response)
}

func TestAnthropic_Generate_TimeoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// simulate timeout by blocking forever
		select {}
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	// create a context with immediate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestAnthropic_Generate_NetworkError(t *testing.T) {
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

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	provider := &Anthropic{
		client:    client,
		model:     "claude-3-sonnet-20240229",
		enabled:   true,
		maxTokens: 1024,
	}

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic api error")
}
