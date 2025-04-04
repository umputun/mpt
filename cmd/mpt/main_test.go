package main

import (
	"bufio"
	"context"
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
