package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			name: "disabled without model",
			options: Options{
				APIKey:  "test-key",
				Enabled: true,
				Model:   "",
			},
			expected: false,
		},
		{
			name: "enabled with all required fields",
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

// Test Chat Completions API (GPT-4o, GPT-4, etc.)

func TestOpenAI_ChatCompletions_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		// send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-4o",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you?"
				},
				"finish_reason": "stop"
			}]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  server.Client().Transport,
			},
		},
		apiKey:            "test-api-key",
		model:             "gpt-4o",
		enabled:           true,
		maxTokens:         100,
		temperature:       0.7,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "Hello")
	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", result)
}

func TestOpenAI_ChatCompletions_WithMaxTokens(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		// verify max_tokens is in request
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"max_tokens":100`)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Response with token limit"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		maxTokens:         100,
		temperature:       0.7,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, "Response with token limit", result)
	assert.True(t, requestReceived)
}

func TestOpenAI_ChatCompletions_WithTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify temperature is in request
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"temperature":0.9`)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "Response with temperature"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		maxTokens:         0,
		temperature:       0.9,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, "Response with temperature", result)
}

func TestOpenAI_ChatCompletions_WithZeroTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify temperature:0 is explicitly sent for deterministic output
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"temperature":0`)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "Deterministic response"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		maxTokens:         0,
		temperature:       0, // zero for deterministic output
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, "Deterministic response", result)
}

func TestOpenAI_ChatCompletions_ReasoningModel_O1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify max_completion_tokens is used instead of max_tokens
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, `"max_completion_tokens":100`)
		assert.NotContains(t, bodyStr, `"temperature"`) // reasoning models don't support temperature

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "Reasoning model response"
				}
			}]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "o1-preview",
		enabled:           true,
		maxTokens:         100,
		temperature:       0.7,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, "Reasoning model response", result)
}

func TestOpenAI_ChatCompletions_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": []}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAI_ChatCompletions_APIError_401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Invalid API key",
				"type": "invalid_request_error"
			}
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestOpenAI_ChatCompletions_APIError_429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Rate limit exceeded"
			}
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4o",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Rate limit")
}

func TestOpenAI_ChatCompletions_APIError_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Model not found",
				"code": "model_not_found"
			}
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-4",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model issue")
	assert.Contains(t, err.Error(), "check if model exists")
}

// Test Responses API (GPT-5)

func TestOpenAI_ResponsesAPI_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/responses", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		// send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "resp_123",
			"status": "completed",
			"output": [
				{
					"type": "reasoning",
					"summary": []
				},
				{
					"type": "message",
					"content": [
						{
							"type": "output_text",
							"text": "Hello from GPT-5!"
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-api-key",
		model:             "gpt-5",
		enabled:           true,
		maxTokens:         100,
		temperature:       0.7,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "Hello")
	require.NoError(t, err)
	assert.Equal(t, "Hello from GPT-5!", result)
}

func TestOpenAI_ResponsesAPI_WithMaxOutputTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify max_output_tokens is in request (not temperature for GPT-5)
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, `"max_output_tokens":100`)
		assert.NotContains(t, bodyStr, `"temperature"`) // GPT-5 doesn't support temperature

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "completed",
			"output": [
				{
					"type": "message",
					"content": [
						{
							"type": "output_text",
							"text": "Response"
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-5",
		enabled:           true,
		maxTokens:         100,
		temperature:       0.7,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	result, err := provider.Generate(context.Background(), "test")
	require.NoError(t, err)
	assert.Equal(t, "Response", result)
}

func TestOpenAI_ResponsesAPI_StatusNotCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "in_progress",
			"output": []
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-5",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected response status")
}

func TestOpenAI_ResponsesAPI_NoOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "completed",
			"output": [
				{
					"type": "reasoning"
				}
			]
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-5",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no output_text found")
}

func TestOpenAI_ResponsesAPI_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"message": "Invalid request",
				"type": "invalid_request_error"
			}
		}`))
	}))
	defer server.Close()

	provider := &OpenAI{
		httpClient: &http.Client{
			Transport: &urlRewriteTransport{
				base:   server.URL,
				target: "https://api.openai.com",
				inner:  http.DefaultTransport,
			},
		},
		apiKey:            "test-key",
		model:             "gpt-5",
		enabled:           true,
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}

	_, err := provider.Generate(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai api error")
}

// Test Routing Logic

func TestOpenAI_RoutingLogic_GPT5_UsesResponsesAPI(t *testing.T) {
	provider := &OpenAI{
		model:             "gpt-5",
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}
	assert.True(t, provider.needsResponsesAPI())
}

func TestOpenAI_RoutingLogic_GPT5_UpperCase_UsesResponsesAPI(t *testing.T) {
	provider := &OpenAI{
		model:             "GPT-5-TURBO",
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}
	assert.True(t, provider.needsResponsesAPI())
}

func TestOpenAI_RoutingLogic_GPT4_UsesChatCompletions(t *testing.T) {
	provider := &OpenAI{
		model:             "gpt-4o",
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}
	assert.False(t, provider.needsResponsesAPI())
}

func TestOpenAI_RoutingLogic_O1_UsesChatCompletions(t *testing.T) {
	provider := &OpenAI{
		model:             "o1-preview",
		baseURL:           "https://api.openai.com",
		forceEndpointType: EndpointTypeAuto,
	}
	assert.False(t, provider.needsResponsesAPI())
}

// Test Constructor

func TestNewOpenAI_DefaultHTTPClient(t *testing.T) {
	provider := NewOpenAI(Options{
		APIKey:  "test-key",
		Model:   "gpt-4o",
		Enabled: true,
	})

	assert.NotNil(t, provider.httpClient)
	assert.True(t, provider.Enabled())
}

func TestNewOpenAI_CustomHTTPClient(t *testing.T) {
	customClient := &http.Client{}
	provider := NewOpenAI(Options{
		APIKey:     "test-key",
		Model:      "gpt-4o",
		Enabled:    true,
		HTTPClient: customClient,
	})

	assert.Equal(t, customClient, provider.httpClient)
}

func TestNewOpenAI_DefaultMaxTokens(t *testing.T) {
	provider := NewOpenAI(Options{
		APIKey:    "test-key",
		Model:     "gpt-4o",
		Enabled:   true,
		MaxTokens: -1,
	})

	assert.Equal(t, DefaultMaxTokens, provider.maxTokens)
}

func TestNewOpenAI_DefaultTemperature(t *testing.T) {
	provider := NewOpenAI(Options{
		APIKey:      "test-key",
		Model:       "gpt-4o",
		Enabled:     true,
		Temperature: -1,
	})

	assert.InDelta(t, float32(DefaultTemperature), provider.temperature, 0.001)
}

// urlRewriteTransport rewrites URLs from target to base for testing
type urlRewriteTransport struct {
	base   string
	target string
	inner  http.RoundTripper
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// rewrite URL from target to base
	if strings.HasPrefix(req.URL.String(), t.target) {
		newURL := strings.Replace(req.URL.String(), t.target, t.base, 1)
		newReq, err := http.NewRequest(req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		newReq.Header = req.Header
		req = newReq
	}

	inner := t.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(req)
}
