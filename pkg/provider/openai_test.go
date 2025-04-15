package provider

import (
	"context"
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
	// should return a sanitized error since our error might trigger the sanitizer
	assert.Contains(t, err.Error(), "API error")
	// in lower case in the original format
	assert.Contains(t, strings.ToLower(err.Error()), "openai")
	// either contains the original error or the sanitized version
	assert.True(t,
		strings.Contains(err.Error(), "openai api error") ||
			strings.Contains(err.Error(), "redacted because it may contain sensitive information"),
		"Error should either contain original message or sanitized message")
}
