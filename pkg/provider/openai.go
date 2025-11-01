package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPClient is an interface for making HTTP requests, allows for dependency injection and testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// OpenAI implements Provider interface for OpenAI
type OpenAI struct {
	httpClient        HTTPClient
	apiKey            string
	model             string
	enabled           bool
	maxTokens         int
	temperature       float32
	baseURL           string       // base URL for API (defaults to https://api.openai.com)
	forceEndpointType EndpointType // manual endpoint selection (auto, responses, chat_completions)
}

// responsesRequest represents request to OpenAI responses API
type responsesRequest struct {
	Model           string  `json:"model"`
	Input           string  `json:"input"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	Temperature     float32 `json:"temperature,omitempty"`
}

// responsesResponse represents response from OpenAI responses API
type responsesResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content,omitempty"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// chatCompletionRequest represents request to OpenAI chat completions API
type chatCompletionRequest struct {
	Model               string                  `json:"model"`
	Messages            []chatCompletionMessage `json:"messages"`
	MaxTokens           int                     `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                     `json:"max_completion_tokens,omitempty"`
	Temperature         *float32                `json:"temperature,omitempty"` // pointer to distinguish between unset and zero
}

// chatCompletionMessage represents a message in chat completions request
type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse represents response from OpenAI chat completions API
type chatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// DefaultMaxTokens defines the default value for max tokens if not specified or negative
const DefaultMaxTokens = 1024

// DefaultTemperature defines the default temperature if not specified or negative
const DefaultTemperature = 0.7

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(opts Options) *OpenAI {
	// quick validation for direct constructor usage (without CreateProvider)
	// note: APIKey can be empty for custom providers that don't require authentication
	if !opts.Enabled || opts.Model == "" {
		return &OpenAI{enabled: false}
	}

	// use provided HTTP client or default to standard http.Client
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	// set default max tokens if not specified
	maxTokens := opts.MaxTokens
	if maxTokens < 0 {
		maxTokens = DefaultMaxTokens
	}
	// if maxTokens is 0, we'll use the model's maximum (API will determine the limit)

	// set default temperature if not specified (negative means unset)
	temperature := opts.Temperature
	if temperature < 0 {
		temperature = DefaultTemperature
	}

	// set default base URL if not specified
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	// set default endpoint type if not specified
	forceEndpointType := opts.ForceEndpointType
	if forceEndpointType == "" {
		forceEndpointType = EndpointTypeAuto
	}

	return &OpenAI{
		httpClient:        httpClient,
		apiKey:            opts.APIKey,
		model:             opts.Model,
		enabled:           true,
		maxTokens:         maxTokens,
		temperature:       temperature,
		baseURL:           baseURL,
		forceEndpointType: forceEndpointType,
	}
}

// Name returns the provider name
func (o *OpenAI) Name() string {
	return "OpenAI"
}

// needsResponsesAPI checks if the model requires v1/responses endpoint instead of v1/chat/completions
func (o *OpenAI) needsResponsesAPI() bool {
	// check if endpoint type is manually forced
	switch o.forceEndpointType {
	case EndpointTypeResponses:
		return true
	case EndpointTypeChatCompletions:
		return false
	}

	// auto-detect based on model name
	modelLower := strings.ToLower(o.model)
	// gpt-5 models only work with responses API
	return strings.Contains(modelLower, "gpt-5")
}

// doRequest handles the common HTTP request logic for OpenAI API calls
func (o *OpenAI) doRequest(ctx context.Context, url string, reqBody interface{}) ([]byte, error) {
	// marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// set headers
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	// send request
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}
	defer resp.Body.Close()

	// read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// check HTTP status code for non-JSON responses (e.g., proxy errors, cloudflare errors)
	// note: OpenAI API returns JSON with error details even for non-2xx status codes,
	// so we only return an HTTP error if the response doesn't look like JSON
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// check if response looks like JSON (starts with { or [)
		trimmedBody := strings.TrimSpace(string(body))
		if !strings.HasPrefix(trimmedBody, "{") && !strings.HasPrefix(trimmedBody, "[") {
			// non-JSON error response (HTML, plain text, etc.)
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, trimmedBody)
		}
		// otherwise, return JSON body and let parse functions handle the error
	}

	return body, nil
}

// buildResponsesRequest creates a request body for the responses API
func (o *OpenAI) buildResponsesRequest(prompt string) responsesRequest {
	reqBody := responsesRequest{
		Model: o.model,
		Input: prompt,
	}

	// set max_output_tokens if specified (0 means use model maximum)
	if o.maxTokens > 0 {
		reqBody.MaxOutputTokens = o.maxTokens
	}

	// note: GPT-5 doesn't support temperature parameter, so we don't set it
	return reqBody
}

// parseResponsesResponse parses and validates the responses API response
func (o *OpenAI) parseResponsesResponse(body []byte) (string, error) {
	var result responsesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// check for error in response
	if result.Error != nil {
		return "", fmt.Errorf("openai api error: %s", result.Error.Message)
	}

	// check status
	if result.Status != "completed" {
		return "", fmt.Errorf("unexpected response status: %s", result.Status)
	}

	// extract text from output array
	for _, output := range result.Output {
		if output.Type == "message" {
			for _, content := range output.Content {
				if content.Type == "output_text" && content.Text != "" {
					return content.Text, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no output_text found in response")
}

// generateWithResponsesAPI calls the OpenAI v1/responses endpoint
func (o *OpenAI) generateWithResponsesAPI(ctx context.Context, prompt string) (string, error) {
	reqBody := o.buildResponsesRequest(prompt)
	url := o.baseURL + "/v1/responses"
	body, err := o.doRequest(ctx, url, reqBody)
	if err != nil {
		return "", err
	}

	return o.parseResponsesResponse(body)
}

// isReasoningModel checks if the model is a reasoning model (o1, o3, o4)
func (o *OpenAI) isReasoningModel() bool {
	modelLower := strings.ToLower(o.model)
	return strings.HasPrefix(modelLower, "o1") ||
		strings.HasPrefix(modelLower, "o3") ||
		strings.HasPrefix(modelLower, "o4")
}

// buildChatCompletionRequest creates a request body for the chat completions API
func (o *OpenAI) buildChatCompletionRequest(prompt string) chatCompletionRequest {
	reqBody := chatCompletionRequest{
		Model: o.model,
		Messages: []chatCompletionMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	// reasoning models use MaxCompletionTokens and don't support temperature
	if o.isReasoningModel() {
		if o.maxTokens > 0 {
			reqBody.MaxCompletionTokens = o.maxTokens
		}
	} else {
		// standard models use max_tokens and support temperature
		if o.maxTokens > 0 {
			reqBody.MaxTokens = o.maxTokens
		}
		if o.temperature >= 0 {
			temp := o.temperature
			reqBody.Temperature = &temp
		}
	}

	return reqBody
}

// parseChatCompletionResponse parses and validates the chat completion API response
func (o *OpenAI) parseChatCompletionResponse(body []byte) (string, error) {
	var result chatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// check for error in response
	if result.Error != nil {
		return "", o.formatChatCompletionError(result.Error)
	}

	// check if there are choices in response
	if len(result.Choices) == 0 {
		return "", errors.New("openai returned no choices - check your model configuration and prompt length")
	}

	return result.Choices[0].Message.Content, nil
}

// formatChatCompletionError formats error messages from chat completion API with additional context
func (o *OpenAI) formatChatCompletionError(apiError *struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}) error {
	errMsg := apiError.Message
	switch {
	case strings.Contains(errMsg, "401") || apiError.Type == "invalid_request_error":
		return fmt.Errorf("openai api error (authentication failed): %s", errMsg)
	case strings.Contains(errMsg, "429"):
		return fmt.Errorf("openai api error (rate limit exceeded): %s", errMsg)
	case strings.Contains(errMsg, "model") || apiError.Code == "model_not_found":
		return fmt.Errorf("openai api error (model issue - check if model exists): %s", errMsg)
	case strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline"):
		return fmt.Errorf("openai api error (request timed out): %s", errMsg)
	case strings.Contains(errMsg, "context") || strings.Contains(errMsg, "length"):
		return fmt.Errorf("openai api error (context length/token limit): %s", errMsg)
	default:
		return fmt.Errorf("openai api error: %s", errMsg)
	}
}

// generateWithChatCompletions calls the OpenAI v1/chat/completions endpoint
func (o *OpenAI) generateWithChatCompletions(ctx context.Context, prompt string) (string, error) {
	reqBody := o.buildChatCompletionRequest(prompt)
	url := o.baseURL + "/v1/chat/completions"
	body, err := o.doRequest(ctx, url, reqBody)
	if err != nil {
		return "", err
	}

	return o.parseChatCompletionResponse(body)
}

// Generate sends a prompt to OpenAI and returns the generated text
func (o *OpenAI) Generate(ctx context.Context, prompt string) (string, error) {
	if !o.enabled {
		return "", errors.New("openai provider is not enabled")
	}

	// use responses API for GPT-5 models
	if o.needsResponsesAPI() {
		return o.generateWithResponsesAPI(ctx, prompt)
	}

	// use chat completions API for all other models
	return o.generateWithChatCompletions(ctx, prompt)
}

// Enabled returns whether this provider is enabled
func (o *OpenAI) Enabled() bool {
	return o.enabled
}
