package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

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
				Model:   "gemini-1.5-pro",
				Enabled: true,
			},
			expected: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGoogle(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestGoogle_Generate_NotEnabled(t *testing.T) {
	provider := &Google{enabled: false}
	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

// mockGoogleServer creates a test server that simulates Google's Gemini API
func mockGoogleServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// createGoogleProviderWithMockServer creates a Google provider with a mock HTTP server
func createGoogleProviderWithMockServer(t *testing.T, server *httptest.Server, model string, maxTokens int) *Google {
	ctx := context.Background()
	client, err := genai.NewClient(ctx,
		option.WithAPIKey("test-key"),
		option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return &Google{
		client:    client,
		model:     model,
		enabled:   true,
		maxTokens: maxTokens,
	}
}

func TestGoogle_Generate_Success(t *testing.T) {
	// create a mock server that returns a successful response
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// validate request
		assert.Contains(t, r.URL.Path, "models/gemini-1.5-pro")

		// return successful response
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "This is a test response"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
					"index":        0,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "This is a test response", response)
}

func TestGoogle_Generate_EmptyResponse(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// return response with no candidates
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestGoogle_Generate_APIError(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// return API error
		w.WriteHeader(http.StatusUnauthorized)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    401,
				"message": "API key not valid. Please pass a valid API key.",
				"status":  "UNAUTHENTICATED",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
}

func TestGoogle_Generate_MultipleParts(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Part 1 "},
							{"text": "Part 2 "},
							{"text": "Part 3"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Part 1 Part 2 Part 3", response)
}

func TestGoogle_Generate_MaxTokensEdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		maxTokens         int
		expectedMaxTokens int32 // what should be sent to the API
	}{
		{
			name:              "zero max tokens uses model default",
			maxTokens:         0,
			expectedMaxTokens: 0, // 0 means don't set, use model's default
		},
		{
			name:              "positive max tokens",
			maxTokens:         1000,
			expectedMaxTokens: 1000,
		},
		{
			name:              "negative max tokens uses default",
			maxTokens:         -100,
			expectedMaxTokens: 1024,
		},
		{
			name:              "max int32 overflow",
			maxTokens:         3000000000,
			expectedMaxTokens: 2147483647,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
				// decode the request body to check maxOutputTokens
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)

				var reqBody map[string]interface{}
				err = json.Unmarshal(body, &reqBody)
				assert.NoError(t, err)

				// check if generationConfig exists in the request
				genConfig, ok := reqBody["generationConfig"].(map[string]interface{})
				assert.True(t, ok, "generationConfig not found in request")

				if tt.expectedMaxTokens > 0 {
					// when expectedMaxTokens > 0, we expect maxOutputTokens to be set
					maxOutput, ok := genConfig["maxOutputTokens"].(float64) // JSON numbers are float64
					assert.True(t, ok, "maxOutputTokens not found in generationConfig")

					assert.InEpsilon(t, float64(tt.expectedMaxTokens), maxOutput, 0.0001)
				} else {
					// when expectedMaxTokens is 0, maxOutputTokens should not be set
					_, hasMaxOutput := genConfig["maxOutputTokens"]
					assert.False(t, hasMaxOutput, "maxOutputTokens should not be set when maxTokens is 0")
				}

				// return successful response
				response := map[string]interface{}{
					"candidates": []map[string]interface{}{
						{
							"content": map[string]interface{}{
								"parts": []map[string]interface{}{
									{"text": "Response with max tokens"},
								},
								"role": "model",
							},
							"finishReason": "STOP",
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				err = json.NewEncoder(w).Encode(response)
				assert.NoError(t, err)
			})
			defer server.Close()

			provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", tt.maxTokens)

			response, err := provider.Generate(context.Background(), "test prompt")
			require.NoError(t, err)
			assert.Equal(t, "Response with max tokens", response)
		})
	}
}

func TestGoogle_Generate_DifferentFinishReasons(t *testing.T) {
	tests := []struct {
		name          string
		finishReason  string
		expected      string
		wantError     bool
		errorContains string
	}{
		{
			name:         "finish reason stop",
			finishReason: "STOP",
			expected:     "Normal completion",
			wantError:    false,
		},
		{
			name:         "finish reason max tokens",
			finishReason: "MAX_TOKENS",
			expected:     "Hit max tokens limit",
			wantError:    false,
		},
		{
			name:          "finish reason safety",
			finishReason:  "SAFETY",
			expected:      "",
			wantError:     true,
			errorContains: "blocked",
		},
		{
			name:         "finish reason other",
			finishReason: "OTHER",
			expected:     "Other reason",
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
				response := map[string]interface{}{
					"candidates": []map[string]interface{}{
						{
							"content": map[string]interface{}{
								"parts": []map[string]interface{}{
									{"text": tt.expected},
								},
								"role": "model",
							},
							"finishReason": tt.finishReason,
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(response)
				assert.NoError(t, err)
			})
			defer server.Close()

			provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

			response, err := provider.Generate(context.Background(), "test prompt")
			if tt.wantError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, response)
			}
		})
	}
}

func TestGoogle_Generate_MultipleCandidates(t *testing.T) {
	// test that we only use the first candidate
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "First candidate"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
					"index":        0,
				},
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Second candidate"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
					"index":        1,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "First candidate", response)
}

func TestGoogle_Generate_RateLimitError(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    429,
				"message": "Quota exceeded for quota metric 'generate-content-requests' and limit 'GenerateContent request limit per minute' of service 'generativelanguage.googleapis.com'",
				"status":  "RESOURCE_EXHAUSTED",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
}

func TestGoogle_Generate_ModelNotFoundError(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    404,
				"message": "models/gemini-invalid-model is not found for API version v1beta",
				"status":  "NOT_FOUND",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-invalid-model", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
}

func TestGoogle_Generate_ContextCanceled(t *testing.T) {
	// create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// this handler should not be called
		t.Fatal("Handler should not be called with canceled context")
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(ctx, "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestGoogle_Generate_EmptyPrompt(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Response to empty prompt"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "Response to empty prompt", response)
}

func TestGoogle_Generate_SpecialCharactersInResponse(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "Response with special chars: 你好世界 🌍 \n\t\r <script>alert('test')</script>", response)
}

func TestGoogle_Generate_NetworkError(t *testing.T) {
	// create a server that immediately closes the connection
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "google api error")
}

func TestGoogle_Generate_EmptyParts(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// return response with content but empty parts
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	_, err := provider.Generate(context.Background(), "test prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestGoogle_Generate_NonTextParts(t *testing.T) {
	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		// return response with non-text parts (should be ignored)
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Text part 1 "},
							{"inlineData": map[string]interface{}{"mimeType": "image/png", "data": "base64data"}},
							{"text": "Text part 2"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	// should only get text parts
	assert.Equal(t, "Text part 1 Text part 2", response)
}

func TestGoogle_Generate_LongResponse(t *testing.T) {
	// test with a very long response
	longText := ""
	for i := 0; i < 1000; i++ {
		longText += "This is line " + string(rune(i)) + " of a very long response. "
	}

	server := mockGoogleServer(t, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": longText},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		assert.NoError(t, err)
	})
	defer server.Close()

	provider := createGoogleProviderWithMockServer(t, server, "gemini-1.5-pro", 0)

	response, err := provider.Generate(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Equal(t, longText, response)
}
