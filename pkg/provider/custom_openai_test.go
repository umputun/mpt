package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomOpenAI_Name(t *testing.T) {
	provider := NewCustomOpenAI(CustomOptions{
		Name:    "TestProvider",
		BaseURL: "http://example.com",
		APIKey:  "test-key",
		Model:   "test-model",
		Enabled: true,
	})
	assert.Equal(t, "TestProvider", provider.Name())
}

func TestCustomOpenAI_DefaultName(t *testing.T) {
	provider := NewCustomOpenAI(CustomOptions{
		BaseURL: "http://example.com",
		APIKey:  "test-key",
		Model:   "test-model",
		Enabled: true,
	})
	assert.Equal(t, "CustomLLM", provider.Name())
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
				APIKey:  "test-key",
				Model:   "test-model",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled without model",
			options: CustomOptions{
				BaseURL: "http://example.com",
				APIKey:  "test-key",
				Model:   "",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled explicitly",
			options: CustomOptions{
				BaseURL: "http://example.com",
				APIKey:  "test-key",
				Model:   "test-model",
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled with base URL and model",
			options: CustomOptions{
				BaseURL: "http://example.com",
				APIKey:  "test-key",
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

func TestCustomOpenAI_Generate_ChatCompletions_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test-id",
			"object": "chat.completion",
			"created": 123,
			"model": "local-model",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "This is a custom provider response"
					},
					"finish_reason": "stop"
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewCustomOpenAI(CustomOptions{
		Name:    "LocalLLM",
		BaseURL: server.URL,
		APIKey:  "test-api-key",
		Model:   "local-model",
		Enabled: true,
	})

	result, err := provider.Generate(context.Background(), "Hello")
	require.NoError(t, err)
	assert.Equal(t, "This is a custom provider response", result)
}

func TestCustomOpenAI_Generate_Responses_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/responses", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test-id",
			"status": "completed",
			"output": [
				{
					"type": "message",
					"content": [
						{
							"type": "output_text",
							"text": "This is a responses API response"
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	provider := NewCustomOpenAI(CustomOptions{
		Name:         "CustomGPT5",
		BaseURL:      server.URL,
		APIKey:       "test-api-key",
		Model:        "custom-gpt-5",
		Enabled:      true,
		EndpointType: EndpointTypeResponses,
	})

	result, err := provider.Generate(context.Background(), "Hello")
	require.NoError(t, err)
	assert.Equal(t, "This is a responses API response", result)
}

func TestCustomOpenAI_Generate_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test-id",
			"object": "chat.completion",
			"created": 123,
			"model": "local-model",
			"choices": []
		}`))
	}))
	defer server.Close()

	provider := NewCustomOpenAI(CustomOptions{
		Name:    "LocalLLM",
		BaseURL: server.URL,
		APIKey:  "test-api-key",
		Model:   "local-model",
		Enabled: true,
	})

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestCustomOpenAI_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Invalid API key",
				"type": "invalid_request_error",
				"code": "invalid_api_key"
			}
		}`))
	}))
	defer server.Close()

	provider := NewCustomOpenAI(CustomOptions{
		Name:    "LocalLLM",
		BaseURL: server.URL,
		APIKey:  "test-api-key",
		Model:   "local-model",
		Enabled: true,
	})

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestCustomOpenAI_DefaultEndpointType(t *testing.T) {
	// default should use chat completions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify it's using chat completions endpoint
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "test"}, "finish_reason": "stop"}]
		}`))
	}))
	defer server.Close()

	provider := NewCustomOpenAI(CustomOptions{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "custom-model",
		Enabled: true,
		// endpointType not specified, should default to chat_completions
	})

	_, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
}

func TestCustomOpenAI_ForceEndpointType(t *testing.T) {
	tests := []struct {
		name         string
		endpointType EndpointType
		expectedPath string
		response     string
	}{
		{
			name:         "force chat completions",
			endpointType: EndpointTypeChatCompletions,
			expectedPath: "/v1/chat/completions",
			response: `{
				"choices": [{"message": {"content": "test"}, "finish_reason": "stop"}]
			}`,
		},
		{
			name:         "force responses",
			endpointType: EndpointTypeResponses,
			expectedPath: "/v1/responses",
			response: `{
				"status": "completed",
				"output": [{"type": "message", "content": [{"type": "output_text", "text": "test"}]}]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.expectedPath, r.URL.Path)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			provider := NewCustomOpenAI(CustomOptions{
				BaseURL:      server.URL,
				APIKey:       "test-key",
				Model:        "custom-model",
				Enabled:      true,
				EndpointType: tt.endpointType,
			})

			_, err := provider.Generate(context.Background(), "test")
			require.NoError(t, err)
		})
	}
}

func TestCustomOpenAI_HTTPClientInjection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "test"}, "finish_reason": "stop"}]
		}`))
	}))
	defer server.Close()

	// create custom HTTP client with URL rewriting transport
	customClient := &http.Client{
		Transport: &urlRewriteTransport{
			base:   server.URL,
			target: "http://example.com",
			inner:  http.DefaultTransport,
		},
	}

	provider := NewCustomOpenAI(CustomOptions{
		Name:       "TestProvider",
		BaseURL:    "http://example.com",
		APIKey:     "test-key",
		Model:      "custom-model",
		Enabled:    true,
		HTTPClient: customClient,
	})

	_, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
}
