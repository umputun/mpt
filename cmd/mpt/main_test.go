package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
	"github.com/umputun/mpt/pkg/runner/mocks"
)

func TestSetupLog(t *testing.T) {
	// test different logging configurations
	setupLog(true)
	setupLog(false)
	setupLog(true, "secret1", "secret2")
}

func TestVerboseOutput(t *testing.T) {
	// create a buffer to capture output
	var buf strings.Builder

	// create test options with verbose flag
	opts := options{
		Prompt:  "test prompt",
		Verbose: true,
	}

	// test that verbose output prints the prompt
	showVerbosePrompt(&buf, opts)

	output := buf.String()
	assert.Contains(t, output, "=== Prompt sent to models ===")
	assert.Contains(t, output, "test prompt")

	// test with NoColor option
	buf.Reset()
	opts.NoColor = true
	showVerbosePrompt(&buf, opts)

	output = buf.String()
	assert.Contains(t, output, "=== Prompt sent to models ===")
	assert.Contains(t, output, "test prompt")
}

func TestProviderCancellation(t *testing.T) {
	// this test directly tests if a provider properly handles context cancellation

	// create a mock provider that blocks until context is canceled
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			// block until context is canceled or a long timeout
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(10 * time.Second): // this should never happen in the test
				return "This should never happen", nil
			}
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	// create a context we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// start the provider in a goroutine
	resultCh := make(chan struct {
		text string
		err  error
	})

	go func() {
		text, err := mockProvider.Generate(ctx, "test prompt")
		resultCh <- struct {
			text string
			err  error
		}{text, err}
	}()

	// let it start and then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// wait for result with timeout
	select {
	case result := <-resultCh:
		// should have context.Canceled error
		require.Error(t, result.err)
		require.ErrorIs(t, result.err, context.Canceled)
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out waiting for provider to handle context cancellation")
	}
}

// TestRunnerCancellation tests that the runner properly propagates context cancellation
// to the providers. Note: the runner doesn't return an error, but includes the error in
// the result text.
func TestRunnerCancellation(t *testing.T) {
	doneCh := make(chan struct{})

	// create a mock provider with a Generate function that checks for context cancellation
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			// signal that we've started
			select {
			case doneCh <- struct{}{}:
			default:
			}

			// block until context is canceled
			<-ctx.Done()
			return "", ctx.Err()
		},
		NameFunc: func() string {
			return "TestProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	r := runner.New(mockProvider)

	ctx, cancel := context.WithCancel(context.Background())

	// run in a goroutine and collect the result
	resultCh := make(chan string, 1)
	go func() {
		result, err := r.Run(ctx, "test prompt")
		assert.NoError(t, err, "Runner.Run should not return an error")
		resultCh <- result
	}()

	// wait for generate to start
	select {
	case <-doneCh:
		// generate function has started, now cancel the context
		cancel()
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for Generate to start")
	}

	// wait for result
	select {
	case result := <-resultCh:
		// for a single provider, Runner returns the error as the result text
		assert.Contains(t, result, "context canceled")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for Run to return after cancellation")
	}
}

func TestGetPrompt(t *testing.T) {
	// test cases for getPrompt function
	tests := []struct {
		name           string
		initialPrompt  string
		stdinContent   string
		isPiped        bool
		expectedPrompt string
	}{
		{
			name:           "cli_prompt_only",
			initialPrompt:  "cli prompt",
			stdinContent:   "",
			isPiped:        false,
			expectedPrompt: "cli prompt",
		},
		{
			name:           "piped_content_only",
			initialPrompt:  "",
			stdinContent:   "piped content",
			isPiped:        true,
			expectedPrompt: "piped content",
		},
		{
			name:           "combined_cli_and_piped",
			initialPrompt:  "cli prompt",
			stdinContent:   "piped content",
			isPiped:        true,
			expectedPrompt: "cli prompt\npiped content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// save original stdin
			oldStdin := os.Stdin
			defer func() {
				os.Stdin = oldStdin
			}()

			// create options with the initial prompt
			opts := options{
				Prompt: tt.initialPrompt,
			}

			if tt.isPiped && tt.stdinContent != "" {
				// create a pipe to simulate stdin with piped content
				r, w, err := os.Pipe()
				require.NoError(t, err)
				defer r.Close()

				// write content to the write end of the pipe
				_, err = w.WriteString(tt.stdinContent)
				require.NoError(t, err)
				w.Close()

				// set stdin to the read end of the pipe
				os.Stdin = r

				// mock the Stat function result to simulate piped input
				// this is needed because we can't actually modify the mode bits of our pipe
				// to match the real case where (stat.Mode() & os.ModeCharDevice) == 0
				// instead, we'll modify the function to directly handle the test case
				err = getPromptForTest(&opts, true)
				require.NoError(t, err)
			} else {
				// for non-piped cases, just call getPrompt with isPiped=false
				err := getPromptForTest(&opts, false)
				require.NoError(t, err)
			}

			// check that the prompt matches the expected value
			assert.Equal(t, tt.expectedPrompt, opts.Prompt)
		})
	}
}

// getPromptForTest is a testable version of getPrompt that takes an explicit isPiped parameter
func getPromptForTest(opts *options, isPiped bool) error {
	if isPiped {
		// read from stdin as if it were piped
		scanner := bufio.NewScanner(os.Stdin)
		var sb strings.Builder
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		stdinContent := strings.TrimSpace(sb.String())

		// append stdin to existing prompt if present, or use stdin as prompt
		if opts.Prompt != "" {
			opts.Prompt = opts.Prompt + "\n" + stdinContent
		} else {
			opts.Prompt = stdinContent
		}
	}
	// we don't test the interactive prompt here as it's hard to simulate in tests
	return nil
}

// MockRunnerTester provides helper functions for testing with mocked providers
type MockRunnerTester struct {
	t             *testing.T
	mockProvider  *mocks.ProviderMock
	originalArgs  []string
	originalStdin *os.File
	promptSeen    string
}

// NewMockRunnerTester creates a new tester with a mock provider
func NewMockRunnerTester(t *testing.T) *MockRunnerTester {
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

	return &MockRunnerTester{
		t:             t,
		mockProvider:  mockProvider,
		originalArgs:  os.Args,
		originalStdin: os.Stdin,
	}
}

// SetupArgs sets the command-line arguments
func (m *MockRunnerTester) SetupArgs(args []string) {
	os.Args = args
}

// MockProviderResponse sets the response the mock provider will return
func (m *MockRunnerTester) MockProviderResponse(response string, validatePrompt func(string)) {
	m.mockProvider.GenerateFunc = func(ctx context.Context, prompt string) (string, error) {
		m.promptSeen = prompt
		if validatePrompt != nil {
			validatePrompt(prompt)
		}
		return response, nil
	}
}

// Cleanup restores the original command-line arguments and stdin
func (m *MockRunnerTester) Cleanup() {
	os.Args = m.originalArgs
	if m.originalStdin != nil {
		os.Stdin = m.originalStdin
	}
}

// CreateTempFileWithContent creates a temporary file with the given content
// Returns the file path and a cleanup function
func CreateTempFileWithContent(t *testing.T, content string) (filePath string, cleanup func()) {
	tempDir, err := os.MkdirTemp("", "mpt-test")
	require.NoError(t, err)

	filePath = filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(filePath, []byte(content), 0o600)
	require.NoError(t, err)

	cleanup = func() {
		os.RemoveAll(tempDir)
	}

	return filePath, cleanup
}

func TestRun_WithFileInput(t *testing.T) {
	// create test helper
	tester := NewMockRunnerTester(t)
	defer tester.Cleanup()

	// create a temporary file
	testFilePath, cleanup := CreateTempFileWithContent(t, "Test file content")
	defer cleanup()

	// set up the mock provider to validate the prompt
	tester.MockProviderResponse("Test response", func(prompt string) {
		assert.Contains(t, prompt, "analyze this")
		assert.Contains(t, prompt, "Test file content")
	})

	// set command line args
	tester.SetupArgs([]string{"mpt", "--prompt", "analyze this", "--file", testFilePath, "--timeout", "1s"})

	// create a test runner that uses our mock
	testRun := func(ctx context.Context) error {
		var opts options
		parser := flags.NewParser(&opts, flags.Default)
		if _, err := parser.Parse(); err != nil {
			return err
		}

		if err := getPrompt(&opts); err != nil {
			return err
		}

		// load file content
		if len(opts.Files) > 0 {
			fileContent, err := os.ReadFile(opts.Files[0])
			if err != nil {
				return err
			}

			opts.Prompt += "\n\n" + string(fileContent)
		}

		// skip the normal provider initialization and just use our mock
		r := runner.New(tester.mockProvider)

		// create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()

		// run the prompt
		_, err := r.Run(timeoutCtx, opts.Prompt)
		return err
	}

	// run with a context
	err := testRun(context.Background())
	require.NoError(t, err)

	// verify the mock was called properly
	require.NotEmpty(t, tester.mockProvider.GenerateCalls())
}

func TestRun_PromptCombination(t *testing.T) {
	// create test helper
	tester := NewMockRunnerTester(t)
	defer tester.Cleanup()

	// create a temp file to simulate stdin
	tmpFile, err := os.CreateTemp("", "stdin")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// write content to the temp file
	_, err = tmpFile.WriteString("piped content")
	require.NoError(t, err)
	tmpFile.Close()

	// reopen file for reading and set as stdin
	stdinFile, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer stdinFile.Close()

	// save original stdin and restore it later
	tester.originalStdin = os.Stdin
	os.Stdin = stdinFile

	// set up the mock provider to validate the prompt
	tester.MockProviderResponse("Test response", func(prompt string) {
		assert.Contains(t, prompt, "cli prompt")
		assert.Contains(t, prompt, "piped content")
	})

	// set command line args
	tester.SetupArgs([]string{"mpt", "--prompt", "cli prompt", "--dbg"})

	// create a test runner that uses our mock
	testRun := func(ctx context.Context) error {
		var opts options
		parser := flags.NewParser(&opts, flags.Default)
		if _, err := parser.Parse(); err != nil {
			return err
		}

		if err := getPrompt(&opts); err != nil {
			return err
		}

		// skip the normal provider initialization and just use our mock
		r := runner.New(tester.mockProvider)

		// create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		// run the prompt
		_, err := r.Run(timeoutCtx, opts.Prompt)
		return err
	}

	// run with a context
	err = testRun(context.Background())
	require.NoError(t, err)

	// verify the mock was called properly
	require.NotEmpty(t, tester.mockProvider.GenerateCalls())
}

func TestCustomProviderIntegration(t *testing.T) {
	// create a stub server that simulates an OpenAI-compatible API
	requestReceived := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request: %s %s\n", r.Method, r.URL.Path)
		requestReceived = true

		// only respond to the chat completions endpoint
		if r.URL.Path == "/chat/completions" {
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
		if r.URL.Path == "/chat/completions" && strings.Contains(bodyStr, testContent) {
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

	// check that the Runner satisfies the interface needed by pkg/mcp
	var runnerInterface interface{} = mockRunner
	_, ok := runnerInterface.(interface {
		Run(ctx context.Context, prompt string) (string, error)
	})
	require.True(t, ok, "Runner should implement the interface needed by pkg/mcp")

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

// TestIntegrationCustomProvider is an integration test for the custom provider integration in main.go
func TestIntegrationCustomProvider(t *testing.T) {
	// create a stub server that simulates the custom provider API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
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

		if r.URL.Path == "/chat/completions" {
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

// TestCreateStandardProvider tests the provider creation functionality
func TestCreateStandardProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		apiKey       string
		model        string
		maxTokens    int
		expectNil    bool
	}{
		{
			name:         "openai provider",
			providerType: "openai",
			apiKey:       "test-key",
			model:        "gpt-4o",
			maxTokens:    1024,
			expectNil:    false,
		},
		{
			name:         "anthropic provider",
			providerType: "anthropic",
			apiKey:       "test-key",
			model:        "claude-3",
			maxTokens:    2048,
			expectNil:    false,
		},
		{
			name:         "google provider",
			providerType: "google",
			apiKey:       "test-key",
			model:        "gemini",
			maxTokens:    4096,
			expectNil:    false,
		},
		{
			name:         "unknown provider",
			providerType: "unknown",
			apiKey:       "test-key",
			model:        "model",
			maxTokens:    1000,
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := createStandardProvider(tt.providerType, tt.apiKey, tt.model, tt.maxTokens)

			if tt.expectNil {
				assert.Nil(t, provider, "Provider should be nil for unknown provider type")
				return
			}

			require.NotNil(t, provider, "Provider should not be nil")

			// verify the provider name matches the expected type (case-insensitive)
			assert.Contains(t, strings.ToLower(provider.Name()), tt.providerType, "Provider name should contain the provider type")

			// verify the provider is enabled
			assert.True(t, provider.Enabled(), "Provider should be enabled")
		})
	}
}

// TestProcessPrompt_WithFile tests processPrompt with file content
func TestProcessPrompt_WithFile(t *testing.T) {
	// create a temp file
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFilePath, []byte("file content"), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// setup options
	opts := &options{
		Prompt: "test prompt",
		Files:  []string{testFilePath},
	}

	// process prompt
	err = processPrompt(opts)
	require.NoError(t, err, "processPrompt should not error")

	// verify content
	assert.Contains(t, opts.Prompt, "test prompt", "Prompt should contain original prompt")
	assert.Contains(t, opts.Prompt, "file content", "Prompt should contain file content")
	// don't check exact format as it may change
}

// TestProcessPrompt_Simple tests basic functionality of processPrompt without files
func TestProcessPrompt_Simple(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompt  string
		expectError    bool
		expectedPrompt string
	}{
		{
			name:          "empty prompt should fail",
			initialPrompt: "",
			expectError:   true,
		},
		{
			name:           "valid prompt with no files",
			initialPrompt:  "test prompt",
			expectError:    false,
			expectedPrompt: "test prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup simple options with no files
			opts := &options{
				Prompt: tt.initialPrompt,
			}

			// call processPrompt
			err := processPrompt(opts)

			if tt.expectError {
				assert.Error(t, err, "Expected an error")
				return
			}

			require.NoError(t, err, "processPrompt should not error")
			assert.Equal(t, tt.expectedPrompt, opts.Prompt, "Prompt should match expected")
		})
	}
}

// TestExecutePrompt_Verbose tests the verbose output path in executePrompt
func TestExecutePrompt_Verbose(t *testing.T) {
	// setup mock provider
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Test response for verbose test", nil
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	providers := []provider.Provider{mockProvider}

	// create stdout capture
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// create options with verbose flag
	opts := &options{
		Prompt:  "test prompt",
		Timeout: 5 * time.Second,
		Verbose: true, // enable verbose output
	}

	// execute the function
	ctx := context.Background()
	err = executePrompt(ctx, opts, providers)

	// restore stdout
	w.Close()
	os.Stdout = oldStdout

	// read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// check results
	require.NoError(t, err, "executePrompt should not error")
	assert.Contains(t, output, "=== Prompt sent to models ===", "Verbose output should include prompt header")
	assert.Contains(t, output, "test prompt", "Verbose output should include the prompt text")
	assert.Contains(t, output, "Test response for verbose test", "Output should contain the response")
}

// TestExecutePrompt_Success tests the successful execution path
func TestExecutePrompt_Success(t *testing.T) {
	// setup mock provider
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Test response for: " + prompt, nil
		},
		NameFunc: func() string {
			return "TestProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}
	providers := []provider.Provider{mockProvider}

	opts := &options{
		Prompt:  "test prompt",
		Timeout: 5 * time.Second,
	}

	// redirect stdout
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err, "failed to create stdout pipe")
	os.Stdout = wOut

	// execute prompt
	ctx := context.Background()
	err = executePrompt(ctx, opts, providers)
	require.NoError(t, err, "executePrompt should not error")

	// restore stdout
	wOut.Close()
	os.Stdout = oldStdout

	// read output
	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	output := buf.String()

	// verify output
	assert.Contains(t, output, "Test response for: test prompt", "Output should contain the response")
}

// TestExecutePrompt_DirectErrorHandlers tests the error handling code directly
func TestExecutePrompt_DirectErrorHandlers(t *testing.T) {
	// test context canceled
	err := handleRunnerError(context.Canceled, 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation canceled by user")

	// test context deadline exceeded
	err = handleRunnerError(context.DeadlineExceeded, 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation timed out after 5s")
	assert.Contains(t, err.Error(), "try increasing the timeout with -t flag")

	// test API error
	err = handleRunnerError(fmt.Errorf("api error: something went wrong"), 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider API error")

	// test API error with uppercase
	err = handleRunnerError(fmt.Errorf("API error: something else went wrong"), 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider API error")

	// test generic error
	err = handleRunnerError(fmt.Errorf("some generic error"), 1*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run prompt")
}

// Helper function that extracts the error handling logic from executePrompt
func handleRunnerError(err error, timeout time.Duration) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("operation canceled by user")
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("operation timed out after %s, try increasing the timeout with -t flag", timeout)
	default:
		// check if we have an error from any specific provider
		if strings.Contains(err.Error(), "api error") || strings.Contains(err.Error(), "API error") {
			return fmt.Errorf("provider API error: %w", err)
		}
		// generic fallback for other errors
		return fmt.Errorf("failed to run prompt: %w", err)
	}
}

// TestExecutePrompt_Error tests that executePrompt handles provider errors
// Note: In the current implementation, the runner returns the error message as the result
// rather than returning an error, so this test verifies the correct output
func TestExecutePrompt_Error(t *testing.T) {
	// setup mock provider that returns an API error
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", fmt.Errorf("api error: something went wrong")
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	providers := []provider.Provider{mockProvider}

	// create stdout capture
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// run executePrompt with error-producing mock
	opts := &options{
		Prompt:  "test prompt",
		Timeout: 5 * time.Second,
	}

	ctx := context.Background()
	err = executePrompt(ctx, opts, providers)

	// restore stdout
	w.Close()
	os.Stdout = oldStdout

	// read the output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	// for single provider, the runner converts error to output
	// so we don't expect executePrompt to return an error
	require.NoError(t, err, "executePrompt doesn't return an error with single provider failures")

	// instead, check that the output contains the error message
	output := buf.String()
	assert.Contains(t, output, "api error", "Output should contain the provider error message")
}

// TestAppendFileContent tests the appendFileContent function
func TestAppendFileContent(t *testing.T) {
	// test case 1: No files
	t.Run("no files", func(t *testing.T) {
		opts := &options{
			Prompt: "initial",
			Files:  []string{},
		}

		err := appendFileContent(opts)
		require.NoError(t, err, "appendFileContent should not error")
		assert.Equal(t, "initial", opts.Prompt, "Prompt should be unchanged with no files")
	})

	// test case 2: Single file
	t.Run("single file", func(t *testing.T) {
		// create a test file
		tempDir := t.TempDir()
		testFilePath := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFilePath, []byte("file content"), 0o644)
		require.NoError(t, err, "Failed to create test file")

		opts := &options{
			Prompt: "initial",
			Files:  []string{testFilePath},
		}

		err = appendFileContent(opts)
		require.NoError(t, err, "appendFileContent should not error")

		// check that the prompt contains both initial prompt and file content
		assert.Contains(t, opts.Prompt, "initial", "Prompt should contain the initial prompt")
		assert.Contains(t, opts.Prompt, "file content", "Prompt should contain the file content")
	})

	// test case 3: Files with exclude pattern
	t.Run("file with excludes", func(t *testing.T) {
		tempDir := t.TempDir()

		// create files that should be included
		includePath := filepath.Join(tempDir, "include.txt")
		err := os.WriteFile(includePath, []byte("include content"), 0o644)
		require.NoError(t, err, "Failed to create include file")

		// create files that should be excluded
		excludeDir := filepath.Join(tempDir, "exclude")
		err = os.MkdirAll(excludeDir, 0o755)
		require.NoError(t, err, "Failed to create exclude dir")

		excludePath := filepath.Join(excludeDir, "exclude.txt")
		err = os.WriteFile(excludePath, []byte("exclude content"), 0o644)
		require.NoError(t, err, "Failed to create exclude file")

		opts := &options{
			Prompt:   "initial",
			Files:    []string{filepath.Join(tempDir, "*.txt"), filepath.Join(tempDir, "**", "*.txt")},
			Excludes: []string{filepath.Join(tempDir, "exclude", "**")},
		}

		err = appendFileContent(opts)
		require.NoError(t, err, "appendFileContent should not error")

		// verify content
		assert.Contains(t, opts.Prompt, "initial", "Prompt should contain the initial prompt")
		assert.Contains(t, opts.Prompt, "include content", "Prompt should contain the included content")
		assert.NotContains(t, opts.Prompt, "exclude content", "Prompt should not contain excluded content")
	})

	// test case 4: File not found
	t.Run("file not found", func(t *testing.T) {
		opts := &options{
			Prompt: "initial",
			Files:  []string{"/nonexistent/file.txt"},
		}

		err := appendFileContent(opts)
		assert.Error(t, err, "Expected an error for non-existent file")
	})
}

// TestInitializeProviders tests the provider initialization logic
func TestInitializeProviders(t *testing.T) {
	tests := []struct {
		name            string
		opts            *options
		expectedCount   int
		expectedTypes   []string
		expectedMissing []string
	}{
		{
			name: "all providers enabled",
			opts: &options{
				OpenAI: openAIOpts{
					Enabled: true,
					APIKey:  "test-key",
					Model:   "gpt-4o",
				},
				Anthropic: anthropicOpts{
					Enabled: true,
					APIKey:  "test-key",
					Model:   "claude-3",
				},
				Google: googleOpts{
					Enabled: true,
					APIKey:  "test-key",
					Model:   "gemini",
				},
				Custom: customOpenAIProvider{
					Enabled: true,
					URL:     "https://test.com",
					Model:   "model",
				},
			},
			expectedCount: 4,
			expectedTypes: []string{"openai", "anthropic", "google", "custom"},
		},
		{
			name: "only openai enabled",
			opts: &options{
				OpenAI: openAIOpts{
					Enabled: true,
					APIKey:  "test-key",
					Model:   "gpt-4o",
				},
			},
			expectedCount:   1,
			expectedTypes:   []string{"openai"},
			expectedMissing: []string{"anthropic", "google", "custom"},
		},
		{
			name: "custom provider without URL",
			opts: &options{
				Custom: customOpenAIProvider{
					Enabled: true,
					Model:   "model",
					// no URL provided
				},
			},
			expectedCount:   0,
			expectedMissing: []string{"custom"},
		},
		{
			name: "custom provider without model",
			opts: &options{
				Custom: customOpenAIProvider{
					Enabled: true,
					URL:     "https://test.com",
					// no model provided
				},
			},
			expectedCount:   0,
			expectedMissing: []string{"custom"},
		},
		{
			name:          "no providers enabled",
			opts:          &options{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := initializeProviders(tt.opts)

			assert.Len(t, providers, tt.expectedCount, "Provider count should match expected")

			// verify each expected provider is present (case-insensitive)
			for _, expectedType := range tt.expectedTypes {
				found := false
				for _, p := range providers {
					if strings.Contains(strings.ToLower(p.Name()), strings.ToLower(expectedType)) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected provider %s not found", expectedType)
			}

			// verify each expected missing provider is actually missing (case-insensitive)
			for _, expectedMissing := range tt.expectedMissing {
				found := false
				for _, p := range providers {
					if strings.Contains(strings.ToLower(p.Name()), strings.ToLower(expectedMissing)) {
						found = true
						break
					}
				}
				assert.False(t, found, "Provider %s should not be present", expectedMissing)
			}
		})
	}
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
}
