package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	// should return a sanitized error since our error might trigger the sanitizer
	assert.Contains(t, err.Error(), "API error")
	// in lower case in the original format
	assert.Contains(t, strings.ToLower(err.Error()), "anthropic")
	// either contains the original error or the sanitized version
	assert.True(t,
		strings.Contains(err.Error(), "anthropic api error") ||
			strings.Contains(err.Error(), "redacted because it may contain sensitive information"),
		"Error should either contain original message or sanitized message")

}
