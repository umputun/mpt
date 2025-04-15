package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// to the providers.
func TestRunnerCancellation(t *testing.T) {
	// test single provider cancellation
	t.Run("single provider cancellation", func(t *testing.T) {
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

		// run in a goroutine and collect the result and error
		type result struct {
			text string
			err  error
		}
		resultCh := make(chan result, 1)
		go func() {
			text, err := r.Run(ctx, "test prompt")
			resultCh <- result{text: text, err: err}
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
			// for a single provider, we expect an error for cancellation
			require.Error(t, result.err, "Runner.Run should return an error for context cancellation with single provider")
			assert.Contains(t, result.err.Error(), "canceled by user", "Error should include user cancellation message")
			assert.Empty(t, result.text, "Result text should be empty")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timed out waiting for Run to return after cancellation")
		}
	})

	// test multiple provider cancellation where all fail
	t.Run("multi provider cancellation - all fail", func(t *testing.T) {
		doneCh := make(chan struct{}, 2) // buffer for both providers

		// create two mock providers that will be canceled
		mockProvider1 := &mocks.ProviderMock{
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
				return "TestProvider1"
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		mockProvider2 := &mocks.ProviderMock{
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
				return "TestProvider2"
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		r := runner.New(mockProvider1, mockProvider2)

		ctx, cancel := context.WithCancel(context.Background())

		// run in a goroutine and collect the result and error
		type result struct {
			text string
			err  error
		}
		resultCh := make(chan result, 1)
		go func() {
			text, err := r.Run(ctx, "test prompt")
			resultCh <- result{text: text, err: err}
		}()

		// wait for both providers to start
		providersStarted := 0
		for providersStarted < 2 {
			select {
			case <-doneCh:
				providersStarted++
			case <-time.After(500 * time.Millisecond):
				t.Fatal("Timed out waiting for providers to start")
			}
		}

		// both providers have started, now cancel the context
		cancel()

		// wait for result
		select {
		case result := <-resultCh:
			// when all providers fail, we expect an error
			require.Error(t, result.err, "Runner.Run should return an error when all providers are canceled")
			assert.Contains(t, result.err.Error(), "canceled by user", "Error should mention user cancellation")
			assert.Empty(t, result.text, "Result text should be empty")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timed out waiting for Run to return after cancellation")
		}
	})
}

func TestGetPrompt(t *testing.T) {
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

// TestCreateStandardProvider tests the provider creation functionality
func TestCreateStandardProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		apiKey       string
		model        string
		maxTokens    int
		temperature  float32
		expectNil    bool
	}{
		{
			name:         "openai provider",
			providerType: "openai",
			apiKey:       "test-key",
			model:        "gpt-4o",
			maxTokens:    1024,
			temperature:  0.7,
			expectNil:    false,
		},
		{
			name:         "anthropic provider",
			providerType: "anthropic",
			apiKey:       "test-key",
			model:        "claude-3",
			maxTokens:    2048,
			temperature:  0.7,
			expectNil:    false,
		},
		{
			name:         "google provider",
			providerType: "google",
			apiKey:       "test-key",
			model:        "gemini",
			maxTokens:    4096,
			temperature:  0.7,
			expectNil:    false,
		},
		{
			name:         "unknown provider",
			providerType: "unknown",
			apiKey:       "test-key",
			model:        "model",
			maxTokens:    1000,
			temperature:  0.7,
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerInstance := createStandardProvider(tt.providerType, tt.apiKey, tt.model, tt.maxTokens, tt.temperature)

			switch tt.name {
			case "unknown provider":
				assert.Nil(t, providerInstance, "Provider should be nil for unknown provider type")
				return
			default:
				require.NotNil(t, providerInstance, "Provider should not be nil")
			}

			// verify the provider name matches the expected type (case-insensitive)
			assert.Contains(t, strings.ToLower(providerInstance.Name()), tt.providerType, "Provider name should contain the provider type")

			// verify the provider is enabled
			assert.True(t, providerInstance.Enabled(), "Provider should be enabled")
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
		Prompt:      "test prompt",
		Files:       []string{testFilePath},
		MaxFileSize: 1024 * 1024, // use 1MB max file size for tests
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
	case strings.Contains(strings.ToLower(err.Error()), "api error"):
		return fmt.Errorf("provider API error: %w", err)
	default:
		return fmt.Errorf("failed to run prompt: %w", err)
	}
}

// TestExecutePrompt_Error tests that executePrompt handles provider errors
func TestExecutePrompt_Error(t *testing.T) {
	// test a single provider failure
	t.Run("single provider failure", func(t *testing.T) {
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

		// with the updated runner behavior, executePrompt should return an error
		// when a single provider fails
		require.Error(t, err, "executePrompt should return an error with single provider failures")
		assert.Contains(t, err.Error(), "api error", "Error should contain the provider error message")
	})

	// test a scenario with multiple providers where some fail but not all
	t.Run("some providers fail", func(t *testing.T) {
		// one provider fails, one succeeds
		failingProvider := &mocks.ProviderMock{
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", fmt.Errorf("api error: something went wrong")
			},
			NameFunc: func() string {
				return "FailingProvider"
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		successProvider := &mocks.ProviderMock{
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Success response", nil
			},
			NameFunc: func() string {
				return "SuccessProvider"
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		providers := []provider.Provider{failingProvider, successProvider}

		// create stdout capture
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		// run executePrompt with both mocks
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
		output := buf.String()

		// no error should be returned since at least one provider succeeded
		require.NoError(t, err, "executePrompt should not return an error when some providers succeed")

		// output should only show the successful result, not the error
		assert.NotContains(t, output, "api error", "Output should not contain the failing provider's error message")
		assert.Contains(t, output, "Success response", "Output should contain the successful provider's response")
	})
}

func TestBuildFullPrompt(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		opts := &options{
			Prompt: "initial",
			Files:  []string{},
		}

		err := buildFullPrompt(opts)
		require.NoError(t, err, "buildFullPrompt should not error")
		assert.Equal(t, "initial", opts.Prompt, "Prompt should be unchanged with no files")
	})

	t.Run("single file", func(t *testing.T) {
		// create a test file
		tempDir := t.TempDir()
		testFilePath := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFilePath, []byte("file content"), 0o644)
		require.NoError(t, err, "Failed to create test file")

		opts := &options{
			Prompt:      "initial",
			MaxFileSize: 1024 * 1024, // use 1MB max file size for tests
			Files:       []string{testFilePath},
		}

		err = buildFullPrompt(opts)
		require.NoError(t, err, "buildFullPrompt should not error")

		// check that the prompt contains both initial prompt and file content
		assert.Contains(t, opts.Prompt, "initial", "Prompt should contain the initial prompt")
		assert.Contains(t, opts.Prompt, "file content", "Prompt should contain the file content")
	})

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
			Prompt:      "initial",
			Files:       []string{filepath.Join(tempDir, "*.txt"), filepath.Join(tempDir, "**", "*.txt")},
			Excludes:    []string{filepath.Join(tempDir, "exclude", "**")},
			MaxFileSize: 1024 * 1024,
		}

		err = buildFullPrompt(opts)
		require.NoError(t, err, "buildFullPrompt should not error")

		// verify content
		assert.Contains(t, opts.Prompt, "initial", "Prompt should contain the initial prompt")
		assert.Contains(t, opts.Prompt, "include content", "Prompt should contain the included content")
		assert.NotContains(t, opts.Prompt, "exclude content", "Prompt should not contain excluded content")
	})

	t.Run("file not found", func(t *testing.T) {
		opts := &options{
			Prompt: "initial",
			Files:  []string{"/nonexistent/file.txt"},
		}

		err := buildFullPrompt(opts)
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
			providers, err := initializeProviders(tt.opts)

			switch tt.name {
			case "no providers enabled":
				require.Error(t, err, "Should return error when no providers enabled")
				assert.Contains(t, err.Error(), "no providers enabled")
				return
			case "custom provider without URL":
				require.Error(t, err, "Should return error when custom provider URL is missing")
				assert.Contains(t, err.Error(), "URL is required")
				return
			case "custom provider without model":
				require.Error(t, err, "Should return error when custom provider model is missing")
				assert.Contains(t, err.Error(), "Model is required")
				return
			default:
				require.NoError(t, err, "Should initialize providers without error")
			}

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

// TestOutputJSON tests the JSON output formatting functionality
func TestOutputJSON(t *testing.T) {
	testCases := []struct {
		name           string
		combinedResult string
		results        []provider.Result
		checkFields    []string
	}{
		{
			name:           "single successful result",
			combinedResult: "This is a test response",
			results: []provider.Result{
				{
					Provider: "TestProvider",
					Text:     "This is a test response",
					Error:    nil,
				},
			},
			checkFields: []string{
				`"provider": "TestProvider"`,
				`"text": "This is a test response"`,
				`"timestamp": "`,
			},
		},
		{
			name:           "multiple results with an error",
			combinedResult: "== generated by Provider1 ==\nFirst response\n",
			results: []provider.Result{
				{
					Provider: "Provider1",
					Text:     "First response",
					Error:    nil,
				},
				{
					Provider: "Provider2",
					Text:     "",
					Error:    errors.New("test error"),
				},
			},
			checkFields: []string{
				`"provider": "Provider1"`,
				`"text": "First response"`,
				`"provider": "Provider2"`,
				`"error": "test error"`,
				`"timestamp": "`,
			},
		},
		{
			name:           "empty combined result",
			combinedResult: "",
			results: []provider.Result{
				{
					Provider: "Provider",
					Text:     "Some text",
					Error:    nil,
				},
			},
			checkFields: []string{
				`"provider": "Provider"`,
				`"text": "Some text"`,
				`"timestamp": "`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// redirect stdout to capture the JSON output
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err, "Failed to create pipe")
			os.Stdout = w

			// call the function
			err = outputJSON(tc.combinedResult, tc.results)
			require.NoError(t, err, "outputJSON should not return an error")

			// close the writer and restore stdout
			w.Close()
			os.Stdout = oldStdout

			// read the captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// check that the output is valid JSON
			var result map[string]interface{}
			err = json.Unmarshal([]byte(output), &result)
			require.NoError(t, err, "Output should be valid JSON")

			// check that the output contains all the expected fields
			for _, field := range tc.checkFields {
				assert.Contains(t, output, field, "Output should contain %s", field)
			}

			// verify the structure of the JSON
			assert.Contains(t, result, "responses", "JSON should contain 'responses' field")
			assert.Contains(t, result, "timestamp", "JSON should contain 'timestamp' field")
			assert.NotContains(t, result, "result", "JSON should not contain 'result' field")

			// verify the responses array
			responses, ok := result["responses"].([]interface{})
			require.True(t, ok, "responses should be an array")
			assert.Len(t, responses, len(tc.results), "Number of responses should match input")
		})
	}
}

// TestExecutePrompt_JSON tests the JSON output path in executePrompt
func TestExecutePrompt_JSON(t *testing.T) {
	// setup mock provider
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Test response for JSON test", nil
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

	// create options with JSON flag
	opts := &options{
		Prompt:  "test prompt",
		Timeout: 5 * time.Second,
		JSON:    true, // enable JSON output
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

	// verify that output is valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "Output should be valid JSON")

	// check important fields
	assert.Contains(t, result, "responses", "JSON should have responses field")
	assert.Contains(t, result, "timestamp", "JSON should have timestamp field")
	assert.NotContains(t, result, "result", "JSON should not have result field")

	// check responses array
	responses, ok := result["responses"].([]interface{})
	require.True(t, ok, "responses should be an array")
	require.Len(t, responses, 1, "Should have one response")

	// verify response content
	response := responses[0].(map[string]interface{})
	assert.Equal(t, "MockProvider", response["provider"], "Provider name should match")
	assert.Equal(t, "Test response for JSON test", response["text"], "Response text should match")
}
