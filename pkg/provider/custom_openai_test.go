package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomOpenAI_Name(t *testing.T) {
	provider := NewCustomOpenAI(CustomOptions{
		Name:    "TestProvider",
		BaseURL: "http://example.com",
		Model:   "test-model",
		Enabled: true,
	})
	assert.Equal(t, "TestProvider", provider.Name())
}

func TestCustomOpenAI_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		options  CustomOptions
		expected bool
	}{
		{
			name: "disabled without base URL",
			options: CustomOptions{
				BaseURL: "",
				Model:   "test-model",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled without model",
			options: CustomOptions{
				BaseURL: "http://example.com",
				Model:   "",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled explicitly",
			options: CustomOptions{
				BaseURL: "http://example.com",
				Model:   "test-model",
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled with base URL and model",
			options: CustomOptions{
				BaseURL: "http://example.com",
				Model:   "test-model",
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewCustomOpenAI(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestCustomOpenAI_Generate_NotEnabled(t *testing.T) {
	provider := NewCustomOpenAI(CustomOptions{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

// mockOpenAIClient creates a custom OpenAI client that uses a test server
func mockCustomOpenAIServer(t *testing.T, jsonResponse string) (*openai.Client, *httptest.Server) {
	// create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(jsonResponse))
		assert.NoError(t, err, "failed to write test server response")
	}))

	// create a custom client configuration
	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	return client, server
}

func TestCustomOpenAI_Generate_Success(t *testing.T) {
	// create a mock response
	jsonResponse := `{
		"id": "test-id",
		"object": "chat.completion",
		"created": 123,
		"model": "local-model",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "This is a test response"
				},
				"finish_reason": "stop"
			}
		]
	}`

	// create a mock client
	client, server := mockCustomOpenAIServer(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &CustomOpenAI{
		name:        "LocalLLM",
		client:      client,
		model:       "local-model",
		enabled:     true,
		maxTokens:   1024,
		temperature: 0.7,
	}

	// test the Generate method
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestCustomOpenAI_Generate_EmptyChoices(t *testing.T) {
	// create a mock response with empty choices
	jsonResponse := `{
		"id": "test-id",
		"object": "chat.completion",
		"created": 123,
		"model": "local-model",
		"choices": []
	}`

	// create a mock client
	client, server := mockCustomOpenAIServer(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &CustomOpenAI{
		name:        "LocalLLM",
		client:      client,
		model:       "local-model",
		enabled:     true,
		maxTokens:   1024,
		temperature: 0.7,
	}

	// test the Generate method
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned no choices")
}

func TestCustomOpenAI_Generate_APIError(t *testing.T) {
	// create a mock server directly to simulate API error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{
			"error": {
				"message": "Invalid API key",
				"type": "invalid_request_error",
				"code": "invalid_api_key"
			}
		}`))
		assert.NoError(t, err, "failed to write test server response")
	}))
	defer server.Close()

	// create a custom client configuration
	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	// create a provider with the mock client
	provider := &CustomOpenAI{
		name:      "LocalLLM",
		client:    client,
		model:     "local-model",
		enabled:   true,
		maxTokens: 1024,
	}

	// test the Generate method
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	// should contain provider name and api error
	assert.Contains(t, err.Error(), "LocalLLM api error", "Error should mention provider and API error")
	// should contain the actual error details (no longer redacted)
	assert.Contains(t, err.Error(), "401", "Error should contain actual status code")
	assert.Contains(t, err.Error(), "Invalid API key", "Error should contain actual error message")

}
