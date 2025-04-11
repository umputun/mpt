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
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(jsonResponse))
		_ = err
	}))

	// create a custom client configuration
	config := openai.DefaultConfig("test-key")
	config.BaseURL = server.URL
	client := openai.NewClientWithConfig(config)

	return client, server
}

func TestOpenAI_Generate_Success(t *testing.T) {
	// create a mock response
	jsonResponse := `{
		"id": "test-id",
		"object": "chat.completion",
		"created": 123,
		"model": "gpt-4",
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
	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &OpenAI{
		client:     client,
		model:      "gpt-4",
		enabled:    true,
		temperature: 0.7,
	}

	// test the Generate method
	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestOpenAI_Generate_EmptyChoices(t *testing.T) {
	// create a mock response with empty choices
	jsonResponse := `{
		"id": "test-id",
		"object": "chat.completion",
		"created": 123,
		"model": "gpt-4",
		"choices": []
	}`

	// create a mock client
	client, server := mockOpenAIClient(t, jsonResponse)
	defer server.Close()

	// create a provider with the mock client
	provider := &OpenAI{
		client:     client,
		model:      "gpt-4",
		enabled:    true,
		temperature: 0.7,
	}

	// test the Generate method
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAI_Generate_MalformedJSON(t *testing.T) {
	// create a mock server directly to simulate malformed JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{malformed json`))
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
	assert.Contains(t, err.Error(), "openai api error")
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
	assert.Contains(t, err.Error(), "openai api error")
}
