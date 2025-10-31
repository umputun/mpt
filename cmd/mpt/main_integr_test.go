package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/mcp"
	"github.com/umputun/mpt/pkg/runner"
	"github.com/umputun/mpt/pkg/runner/mocks"
)

// TestIntegrationCustomProvider is an integration test for the custom provider integration in main.go
func TestIntegrationCustomProvider(t *testing.T) {
	// create a stub server that simulates the custom provider API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
				"choices": [
					{
						"message": {"role": "assistant", "content": "This is a custom provider response!"},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// set up options for the custom provider in main.go
	opts := &options{
		Prompt: "test prompt",
		Custom: customOpenAIProvider{
			Enabled:   true,
			URL:       ts.URL,
			Model:     "test-model",
			APIKey:    "test-key",
			MaxTokens: 16384,
		},
		Timeout: 5 * time.Second,
	}

	// redirect stdout to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err, "failed to create pipe")
	os.Stdout = wOut

	// call run() from main.go with the custom provider options
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err, "run() returned an error")

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify that the output contains the expected custom provider response
	require.Contains(t, output, "This is a custom provider response!", "Output should contain custom provider response")
}

// TestIntegrationMCPServerModeIntegration is an integration test for MCP server mode.
// It simulates running the application in MCP server mode by providing a stubbed custom provider and a simulated MCP call via STDIN.
func TestIntegrationMCPServerModeIntegration(t *testing.T) {
	// track server request data for verification
	var requestBody []byte
	var requestHeaders http.Header
	requestReceived := false
	testPrompt := "Integration test prompt"

	// create a stub HTTP server that simulates the custom provider API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// capture request data for verification
		requestReceived = true
		requestHeaders = r.Header.Clone()
		requestBody, _ = io.ReadAll(r.Body)
		defer r.Body.Close()

		if r.URL.Path == "/v1/chat/completions" {
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
				"choices": [
					{
						"message": {"role": "assistant", "content": "This is a custom provider response!"},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// prepare options for MCP server mode with custom provider enabled
	opts := &options{
		MCP: mcpOpts{
			Server:     true,
			ServerName: "Test MCP Server",
		},
		Custom: customOpenAIProvider{
			Enabled:   true,
			URL:       ts.URL,
			Model:     "test-model",
			APIKey:    "test-key",
			MaxTokens: 16384,
		},
		Timeout: 5 * time.Second,
	}

	// we'll simulate MCP protocol input via STDIN. Prepare a JSON message that represents a tool call request.
	mcpRequest := fmt.Sprintf(`{"jsonrpc": "2.0", "id": "test-id", "method": "tools/call", "params": {"name": "mpt_generate", "arguments": {"prompt": %q}}}`, testPrompt) + "\n"

	// redirect STDIN: Create a pipe and write the simulated MCP request, then close the writer to signal EOF
	oldStdin := os.Stdin
	rIn, wIn, err := os.Pipe()
	require.NoError(t, err, "failed to create stdin pipe")
	os.Stdin = rIn

	// write the simulated MCP request to wIn in a separate goroutine
	go func() {
		wIn.WriteString(mcpRequest)
		wIn.Close()
	}()

	// redirect STDOUT to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err, "failed to create stdout pipe")
	os.Stdout = wOut

	// call run() from main.go. This should run the MCP server mode which will read the MCP request from STDIN and process it.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err, "run() returned an error")

	// restore STDOUT and STDIN
	wOut.Close()
	os.Stdout = oldStdout
	os.Stdin = oldStdin

	// read the captured output from rOut
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify the HTTP request was made to the mock server
	require.True(t, requestReceived, "The custom provider HTTP endpoint should have been called")

	// verify the request headers
	require.NotEmpty(t, requestHeaders.Get("Content-Type"), "Content-Type header should be set")
	require.Equal(t, "application/json", requestHeaders.Get("Content-Type"), "Content-Type should be application/json")
	require.Equal(t, "Bearer test-key", requestHeaders.Get("Authorization"), "API key should be in Authorization header")

	// verify request body contains the prompt from the MCP request
	require.NotEmpty(t, requestBody, "Request body should not be empty")
	require.Contains(t, string(requestBody), testPrompt, "Request to provider should contain the prompt from MCP request")

	// verify MCP response format
	require.Contains(t, output, "jsonrpc", "Response should follow JSON-RPC format")
	require.Contains(t, output, "\"result\"", "Response should contain a result field")
	require.Contains(t, output, "\"id\":\"test-id\"", "Response should echo back the request ID")

	// verify the content
	require.Contains(t, output, "This is a custom provider response!", "Output should contain the custom provider response")
}

// TestCustomProviderIntegration is an integration test that verifies the custom provider works end-to-end
func TestCustomProviderIntegration(t *testing.T) {
	// create a stub server that simulates an OpenAI-compatible API
	requestReceived := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s\n", r.Method, r.URL.Path)
		requestReceived = true

		// only respond to the chat completions endpoint
		if r.URL.Path == "/v1/chat/completions" {
			// return an OpenAI-compatible response
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				},
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "This is a response from the custom provider!"
						},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// set up command line arguments with our test server URL
	os.Args = []string{
		"test",
		"--prompt", "test prompt",
		"--custom.enabled",
		"--custom.url", ts.URL,
		"--custom.model", "test-model",
		"--custom.api-key", "test-key",
	}

	// override the timeout for testing purposes
	opts := &options{
		Timeout: 5 * time.Second,
	}

	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	_, err := p.Parse()
	require.NoError(t, err)

	setupLog(opts.Debug, collectSecrets(opts)...)

	// verify options parsed correctly
	require.True(t, opts.Custom.Enabled)
	require.Equal(t, ts.URL, opts.Custom.URL)
	require.Equal(t, "test-model", opts.Custom.Model)

	// since we'll be making an actual HTTP request to our test server,
	// we need to redirect stdout to capture the output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = run(ctx, opts)

	// restore stdout
	w.Close()
	os.Stdout = oldStdout

	// read the output
	var output strings.Builder
	io.Copy(&output, r)

	// check results
	require.NoError(t, err)
	require.True(t, requestReceived, "The test server should have received a request")
	require.Contains(t, output.String(), "This is a response from the custom provider!",
		"Output should contain the custom provider response")
}

// TestCustomProviderWithFileAndVerbose tests the custom provider with file input and verbose output
func TestCustomProviderWithFileAndVerbose(t *testing.T) {
	// create a test file
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test.txt")
	testContent := "This is test content for the file parameter"
	err := os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// create a stub server that simulates an OpenAI-compatible API
	requestReceived := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s\n", r.Method, r.URL.Path)
		requestReceived = true

		// check if the request body contains our test file content
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// only respond to the chat completions endpoint and verify the content includes our file
		if r.URL.Path == "/v1/chat/completions" && strings.Contains(bodyStr, testContent) {
			// return an OpenAI-compatible response
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				},
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "File and verbose mode test successful"
						},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// set up command line arguments with our test server URL
	// include the file parameter and verbose flag
	os.Args = []string{
		"test",
		"--prompt", "test prompt with file",
		"-f", testFilePath, // add file parameter
		"-v", // add verbose flag
		"--custom.enabled",
		"--custom.url", ts.URL,
		"--custom.model", "test-model",
	}

	// override the timeout for testing purposes
	opts := &options{
		Timeout: 5 * time.Second,
	}

	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	_, err = p.Parse()
	require.NoError(t, err)

	// verify options parsed correctly
	require.True(t, opts.Custom.Enabled)
	require.Equal(t, ts.URL, opts.Custom.URL)
	require.Equal(t, "test-model", opts.Custom.Model)
	require.True(t, opts.Verbose, "Verbose flag should be enabled")
	require.Contains(t, opts.Files, testFilePath, "Files should contain our test file path")

	// since we'll be making an actual HTTP request to our test server,
	// we need to redirect stdout to capture the output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = run(ctx, opts)

	// restore stdout
	w.Close()
	os.Stdout = oldStdout

	// read the output
	var output strings.Builder
	io.Copy(&output, r)
	outputStr := output.String()

	// check results
	require.NoError(t, err)
	require.True(t, requestReceived, "The test server should have received a request")

	// check for the verbose output
	require.Contains(t, outputStr, "=== Prompt sent to models ===",
		"Output should contain the verbose header")
	require.Contains(t, outputStr, testContent,
		"Output should contain the test file content in the verbose output")

	// check for the model response
	require.Contains(t, outputStr, "File and verbose mode test successful",
		"Output should contain the custom provider response")
}

// TestRunMCPServer tests the MCP server mode functionality
func TestRunMCPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MCP server test in short mode")
	}

	// create a mock runner for testing instead of using real providers
	mockRunner := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Mock response for MCP test: " + prompt, nil
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	// test the MCP server implementation directly
	srv := &options{
		MCP: mcpOpts{
			Server:     true,
			ServerName: "Test MCP Server",
		},
	}

	// verify the MCP server options are set correctly
	require.True(t, srv.MCP.Server, "MCP server flag should be enabled")
	require.Equal(t, "Test MCP Server", srv.MCP.ServerName)

	// test that the code recognizes MCP server mode is enabled
	isMCPServerMode := srv.MCP.Server
	require.True(t, isMCPServerMode, "MCP server mode should be enabled")

	// test that the runner is initialized correctly when in MCP mode
	r := runner.New(mockRunner)
	require.NotNil(t, r, "Runner should be initialized")

	// since we can't access the internal providers field directly in runner.Runner,
	// we'll verify that the runner was created successfully
	require.NotNil(t, r, "Runner should be initialized with the provider")

	// run a prompt through the runner to verify it works
	result, err := r.Run(context.Background(), "test prompt")
	require.NoError(t, err, "Runner should not return an error")
	require.Contains(t, result, "Mock response for MCP test: test prompt",
		"Runner should return the expected response")

	// verify that the mock provider was called
	require.Len(t, mockRunner.GenerateCalls(), 1,
		"Mock provider's Generate method should have been called once")
	require.Equal(t, "test prompt", mockRunner.GenerateCalls()[0].Prompt,
		"Mock provider should have been called with the expected prompt")

	// we can't easily test the stdio-based server, so we're testing just the implementation parts
	// that are accessible without actually running the server
}

// TestMCPServerImplementation tests the specific MCP server implementation components
func TestMCPServerImplementation(t *testing.T) {
	// create a mock provider
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Test response", nil
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	// create a runner with our mock provider
	mockRunner := runner.New(mockProvider)
	require.NotNil(t, mockRunner, "Runner should be created successfully")

	// compile-time check that Runner satisfies the interface needed by pkg/mcp
	var _ mcp.Runner = mockRunner

	// verify we can pass options to the MCP server constructor
	mcpOpts := mcp.ServerOptions{
		Name:    "Test MCP Server",
		Version: "test-version",
	}

	// we can't create the MCP server without the mcp-go dependency in the test
	// but we can verify the settings in the options structure
	require.Equal(t, "Test MCP Server", mcpOpts.Name, "MCP server name should be set correctly")
	require.Equal(t, "test-version", mcpOpts.Version, "MCP server version should be set correctly")
}

// TestIntegrationMCPServerModeErrorScenarios tests error scenarios in MCP server mode
func TestIntegrationMCPServerModeErrorScenarios(t *testing.T) {
	t.Run("invalid MCP request", func(t *testing.T) {
		// prepare options for MCP server mode with a mocked provider
		opts := &options{
			MCP: mcpOpts{
				Server:     true,
				ServerName: "Test MCP Server",
			},
			// need at least one provider enabled
			Custom: customOpenAIProvider{
				Enabled:   true,
				URL:       "http://localhost:12345", // intentionally unreachable
				Model:     "test-model",
				APIKey:    "test-key",
				MaxTokens: 16384,
			},
			Timeout: 2 * time.Second, // shorter timeout for errors
		}

		// simulate an invalid MCP request (missing required fields)
		invalidRequest := `{"jsonrpc": "2.0", "id": "test-id", "method": "tools/call", "params": {"name": "mpt_generate", "arguments": {}}}`

		// redirect STDIN
		oldStdin := os.Stdin
		rIn, wIn, err := os.Pipe()
		require.NoError(t, err, "failed to create stdin pipe")
		os.Stdin = rIn

		// write the invalid request
		go func() {
			wIn.WriteString(invalidRequest + "\n")
			wIn.Close()
		}()

		// redirect STDOUT
		oldStdout := os.Stdout
		rOut, wOut, err := os.Pipe()
		require.NoError(t, err, "failed to create stdout pipe")
		os.Stdout = wOut

		// call run(), which should handle the invalid request
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = run(ctx, opts)
		// should not return an error even for bad requests, as the MCP server handles them internally
		require.NoError(t, err, "run() should not return an error for invalid MCP requests")

		// restore STDOUT and STDIN
		wOut.Close()
		os.Stdout = oldStdout
		os.Stdin = oldStdin

		// read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, rOut)
		output := buf.String()

		// verify error response format follows JSON-RPC
		require.Contains(t, output, "jsonrpc", "Error response should follow JSON-RPC format")
		require.Contains(t, output, "\"error\"", "Response should contain an error field")
		require.Contains(t, output, "\"id\":\"test-id\"", "Response should echo back the request ID")
	})

	t.Run("unknown tool name", func(t *testing.T) {
		// prepare options for MCP server mode
		opts := &options{
			MCP: mcpOpts{
				Server:     true,
				ServerName: "Test MCP Server",
			},
			// need at least one provider enabled
			Custom: customOpenAIProvider{
				Enabled:   true,
				URL:       "http://localhost:12345", // intentionally unreachable
				Model:     "test-model",
				APIKey:    "test-key",
				MaxTokens: 16384,
			},
			Timeout: 2 * time.Second,
		}

		// simulate a request with an unknown tool name
		unknownToolRequest := `{"jsonrpc": "2.0", "id": "test-id", "method": "tools/call", "params": {"name": "unknown_tool", "arguments": {"prompt": "test"}}}`

		// redirect STDIN
		oldStdin := os.Stdin
		rIn, wIn, err := os.Pipe()
		require.NoError(t, err, "failed to create stdin pipe")
		os.Stdin = rIn

		// write the request with unknown tool
		go func() {
			wIn.WriteString(unknownToolRequest + "\n")
			wIn.Close()
		}()

		// redirect STDOUT
		oldStdout := os.Stdout
		rOut, wOut, err := os.Pipe()
		require.NoError(t, err, "failed to create stdout pipe")
		os.Stdout = wOut

		// call run()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = run(ctx, opts)
		// should not return an error even for bad requests
		require.NoError(t, err, "run() should not return an error for unknown tool requests")

		// restore STDOUT and STDIN
		wOut.Close()
		os.Stdout = oldStdout
		os.Stdin = oldStdin

		// read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, rOut)
		output := buf.String()

		// verify error response
		require.Contains(t, output, "jsonrpc", "Error response should follow JSON-RPC format")
		require.Contains(t, output, "\"error\"", "Response should contain an error field")
		require.Contains(t, output, "unknown_tool", "Error should mention the unknown tool")
	})

	t.Run("runner failure", func(t *testing.T) {
		// prepare options for MCP server mode with a provider configured to fail
		opts := &options{
			MCP: mcpOpts{
				Server:     true,
				ServerName: "Test MCP Server",
			},
			// configure a custom provider that will fail (e.g., bad URL)
			Custom: customOpenAIProvider{
				Enabled:   true,
				URL:       "http://localhost:1", // use a port guaranteed to be unreachable
				Model:     "test-model",
				APIKey:    "test-key",
				MaxTokens: 16384,
			},
			Timeout: 2 * time.Second,
		}

		// simulate a valid MCP request
		validRequest := `{"jsonrpc": "2.0", "id": "test-runner-fail", "method": "tools/call", "params": {"name": "mpt_generate", "arguments": {"prompt": "this will fail"}}}`

		// redirect STDIN
		oldStdin := os.Stdin
		rIn, wIn, err := os.Pipe()
		require.NoError(t, err, "failed to create stdin pipe")
		os.Stdin = rIn

		// write the request
		go func() {
			wIn.WriteString(validRequest + "\n")
			wIn.Close()
		}()

		// redirect STDOUT
		oldStdout := os.Stdout
		rOut, wOut, err := os.Pipe()
		require.NoError(t, err, "failed to create stdout pipe")
		os.Stdout = wOut

		// call run()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // allow slightly longer for network error
		defer cancel()
		err = run(ctx, opts)
		require.NoError(t, err, "run() should handle runner errors internally and respond via MCP")

		// restore STDOUT and STDIN
		wOut.Close()
		os.Stdout = oldStdout
		os.Stdin = oldStdin

		// read the captured output
		var buf bytes.Buffer
		io.Copy(&buf, rOut)
		output := buf.String()

		// verify error response format follows JSON-RPC
		require.Contains(t, output, "jsonrpc", "Error response should follow JSON-RPC format")
		require.Contains(t, output, "\"error\"", "Response should contain an error field")
		require.Contains(t, output, "\"id\":\"test-runner-fail\"", "Response should echo back the request ID")
		// check for content indicating failure (the exact message might depend on mcp-go's error formatting)
		require.Contains(t, output, "failed to run prompt through MPT", "Error message should indicate MPT runner failure")
	})
}

// TestHumanSizeValueIntegration tests the human-readable size values in an integration test
func TestHumanSizeValueIntegration(t *testing.T) {
	// create a stub server that simulates the custom provider API
	requestData := make(map[string]any)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			// read and parse the request body to check max_tokens value
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err, "Failed to read request body")

			err = json.Unmarshal(body, &requestData)
			assert.NoError(t, err, "Failed to parse request body")

			// standard OpenAI response
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
				"choices": [
					{
						"message": {"role": "assistant", "content": "Human-size value test response"},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// set up command line arguments with human-readable size values
	os.Args = []string{
		"test",
		"--prompt", "test prompt",
		"--custom.enabled",
		"--custom.url", ts.URL,
		"--custom.model", "test-model",
		"--custom.api-key", "test-key",
		"--custom.max-tokens", "8k", // human-readable value (8 * 1024 = 8192)
		"--max-file-size", "2m", // human-readable value (2 * 1024 * 1024 = 2097152)
	}

	// create options and parse command line
	opts := &options{
		Timeout: 5 * time.Second,
	}

	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	_, err := p.Parse()
	require.NoError(t, err, "Failed to parse command line")

	// verify that human-readable values were correctly parsed
	require.Equal(t, SizeValue(8*1024), opts.Custom.MaxTokens,
		"MaxTokens should be 8k (8192)")
	require.Equal(t, SizeValue(2*1024*1024), opts.MaxFileSize,
		"MaxFileSize should be 2m (2097152)")

	// redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = run(ctx, opts)
	require.NoError(t, err, "run() returned an error")

	// restore stdout
	w.Close()
	os.Stdout = oldStdout

	// read the output
	var output strings.Builder
	io.Copy(&output, r)

	// check that the max_tokens value in the request matched our human-readable setting
	maxTokens, ok := requestData["max_tokens"]
	require.True(t, ok, "Request should include max_tokens")
	require.InDelta(t, float64(8192), maxTokens.(float64), 0.001, "max_tokens in API request should be 8192 (8k)")

	// verify the response
	require.Contains(t, output.String(), "Human-size value test response",
		"Output should contain the provider response")
}

// TestIntegrationJSONOutput tests the JSON output flag in an integration test
func TestIntegrationJSONOutput(t *testing.T) {
	// create a stub server that simulates the custom provider API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			resp := `{
				"id": "test-id",
				"object": "chat.completion",
				"created": 1677858242,
				"model": "test-model",
				"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
				"choices": [
					{
						"message": {"role": "assistant", "content": "This is a JSON output test response"},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// set up options for the custom provider with JSON output flag
	opts := &options{
		Prompt: "test prompt",
		Custom: customOpenAIProvider{
			Enabled:   true,
			URL:       ts.URL,
			Model:     "test-model",
			APIKey:    "test-key",
			MaxTokens: 16384,
		},
		Timeout: 5 * time.Second,
		JSON:    true, // enable JSON output
	}

	// redirect stdout to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err, "failed to create pipe")
	os.Stdout = wOut

	// call run() from main.go with the custom provider options
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err, "run() returned an error")

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify that the output is valid JSON
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// check that the JSON contains the expected response
	assert.Contains(t, result, "responses", "JSON should contain 'responses' field")
	assert.Contains(t, result, "timestamp", "JSON should contain 'timestamp' field")
	assert.NotContains(t, result, "result", "JSON should not contain 'result' field")

	// verify the content
	assert.Contains(t, output, "This is a JSON output test response", "Output should contain provider response")
}

// TestIntegrationMultipleCustomProviders tests multiple custom providers configured via CLI
func TestIntegrationMultipleCustomProviders(t *testing.T) {
	// create two test servers simulating different custom providers
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			resp := `{
				"id": "test-id-1",
				"object": "chat.completion",
				"model": "model1",
				"choices": [{
					"message": {"role": "assistant", "content": "Response from provider 1"},
					"finish_reason": "stop",
					"index": 0
				}]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		}
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			resp := `{
				"id": "test-id-2",
				"object": "chat.completion",
				"model": "model2",
				"choices": [{
					"message": {"role": "assistant", "content": "Response from provider 2"},
					"finish_reason": "stop",
					"index": 0
				}]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		}
	}))
	defer ts2.Close()

	// save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// set up command line arguments with multiple custom providers
	os.Args = []string{
		"test",
		"--prompt", "test prompt",
		"--customs", fmt.Sprintf("provider1:url=%s,model=model1,api-key=key1,enabled=true", ts1.URL),
		"--customs", fmt.Sprintf("provider2:url=%s,model=model2,api-key=key2,enabled=true", ts2.URL),
	}

	opts := &options{
		Timeout: 5 * time.Second,
		Customs: make(map[string]customSpec),
	}

	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	_, err := p.Parse()
	require.NoError(t, err)

	// verify options parsed correctly
	require.Len(t, opts.Customs, 2, "Should have 2 custom providers")
	require.Contains(t, opts.Customs, "provider1")
	require.Contains(t, opts.Customs, "provider2")
	assert.Equal(t, ts1.URL, opts.Customs["provider1"].URL)
	assert.Equal(t, ts2.URL, opts.Customs["provider2"].URL)

	// redirect stdout to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wOut

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err)

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify that output contains responses from both providers
	assert.Contains(t, output, "Response from provider 1")
	assert.Contains(t, output, "Response from provider 2")
	assert.Contains(t, output, "provider1")
	assert.Contains(t, output, "provider2")
}

// TestIntegrationCustomProvidersEnvironmentVariables tests custom providers configured via environment variables
func TestIntegrationCustomProvidersEnvironmentVariables(t *testing.T) {
	// clear any existing CUSTOM_* env vars
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CUSTOM_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) > 0 {
				os.Unsetenv(parts[0])
			}
		}
	}

	// create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			// check api key header
			apiKey := r.Header.Get("Authorization")
			content := "Response from environment-configured provider"
			if apiKey == "Bearer env-secret-key" {
				content = "Response with correct API key from environment"
			}

			resp := fmt.Sprintf(`{
				"id": "test-env",
				"object": "chat.completion",
				"model": "env-model",
				"choices": [{
					"message": {"role": "assistant", "content": "%s"},
					"finish_reason": "stop",
					"index": 0
				}]
			}`, content)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		}
	}))
	defer ts.Close()

	// set environment variables for custom provider
	os.Setenv("CUSTOM_ENVPROV_URL", ts.URL)
	os.Setenv("CUSTOM_ENVPROV_MODEL", "env-model")
	os.Setenv("CUSTOM_ENVPROV_API_KEY", "env-secret-key")
	os.Setenv("CUSTOM_ENVPROV_NAME", "Environment Provider")
	os.Setenv("CUSTOM_ENVPROV_ENABLED", "true")
	defer func() {
		os.Unsetenv("CUSTOM_ENVPROV_URL")
		os.Unsetenv("CUSTOM_ENVPROV_MODEL")
		os.Unsetenv("CUSTOM_ENVPROV_API_KEY")
		os.Unsetenv("CUSTOM_ENVPROV_NAME")
		os.Unsetenv("CUSTOM_ENVPROV_ENABLED")
	}()

	opts := &options{
		Prompt:  "test prompt",
		Timeout: 5 * time.Second,
		Customs: make(map[string]customSpec),
	}

	// redirect stdout to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wOut

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err)

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify that output contains response from environment-configured provider
	assert.Contains(t, output, "Response with correct API key from environment")
	// when there's only one provider, the runner doesn't show provider headers
	// so we don't check for "Environment Provider" in the output
}

// TestIntegrationCustomProvidersPrecedence tests precedence of custom provider configurations
func TestIntegrationCustomProvidersPrecedence(t *testing.T) {
	// create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			// check which model is being used to determine precedence
			body, _ := io.ReadAll(r.Body)
			content := "Unknown source"
			if strings.Contains(string(body), "cli-model") {
				content = "CLI configuration used (highest precedence)"
			} else if strings.Contains(string(body), "legacy-model") {
				content = "Legacy configuration used (middle precedence)"
			} else if strings.Contains(string(body), "env-model") {
				content = "Environment configuration used (lowest precedence)"
			}

			resp := fmt.Sprintf(`{
				"id": "test-precedence",
				"object": "chat.completion",
				"choices": [{
					"message": {"role": "assistant", "content": "%s"},
					"finish_reason": "stop",
					"index": 0
				}]
			}`, content)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resp))
		}
	}))
	defer ts.Close()

	// set environment variable (lowest precedence)
	os.Setenv("CUSTOM_TESTPROV_URL", ts.URL)
	os.Setenv("CUSTOM_TESTPROV_MODEL", "env-model")
	os.Setenv("CUSTOM_TESTPROV_ENABLED", "true")
	defer func() {
		os.Unsetenv("CUSTOM_TESTPROV_URL")
		os.Unsetenv("CUSTOM_TESTPROV_MODEL")
		os.Unsetenv("CUSTOM_TESTPROV_ENABLED")
	}()

	// save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// set up command line arguments with CLI custom provider (highest precedence)
	os.Args = []string{
		"test",
		"--prompt", "test prompt",
		"--customs", fmt.Sprintf("testprov:url=%s,model=cli-model,enabled=true", ts.URL),
	}

	opts := &options{
		Timeout: 5 * time.Second,
		Customs: make(map[string]customSpec),
	}

	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	_, err := p.Parse()
	require.NoError(t, err)

	// redirect stdout to capture output
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wOut

	// run the program
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, opts)
	require.NoError(t, err)

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read the captured output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify that CLI configuration was used (highest precedence)
	assert.Contains(t, output, "CLI configuration used (highest precedence)")
}
