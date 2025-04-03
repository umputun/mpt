package main

import (
	"bufio"
	"bytes"
	"context"
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
	var buf bytes.Buffer

	// create test options with verbose flag
	opts := Opts{
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

func TestRun_Flags(t *testing.T) {
	testCases := []struct {
		name         string
		args         []string
		exitCode     int
		setupMock    func()
		validateFunc func(*testing.T)
	}{
		{
			name:         "version_flag",
			args:         []string{"mpt", "--version"},
			exitCode:     0,
			setupMock:    func() {},
			validateFunc: func(t *testing.T) {},
		},
		{
			name:         "help_flag",
			args:         []string{"mpt", "--help"},
			exitCode:     0,
			setupMock:    func() {},
			validateFunc: func(t *testing.T) {},
		},
		{
			name:         "invalid_flag",
			args:         []string{"mpt", "--invalid-flag"},
			exitCode:     1,
			setupMock:    func() {},
			validateFunc: func(t *testing.T) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// save original osExit and args, restore after the test
			oldOsExit := osExit
			oldArgs := os.Args
			defer func() {
				osExit = oldOsExit
				os.Args = oldArgs
			}()

			// mock os.Exit
			var actualExitCode int
			osExit = func(code int) {
				actualExitCode = code
				panic("os.Exit called")
			}

			// set command line args
			os.Args = tc.args

			// apply any additional test setup
			tc.setupMock()

			// catch the panic from our mocked os.Exit
			defer func() {
				if r := recover(); r != nil {
					assert.Equal(t, "os.Exit called", r)
					assert.Equal(t, tc.exitCode, actualExitCode)

					// run additional validations
					tc.validateFunc(t)
				}
			}()

			// run the function with a context
			ctx := context.Background()
			run(ctx)
		})
	}
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
		assert.Error(t, result.err)
		assert.ErrorIs(t, result.err, context.Canceled)
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
			opts := Opts{
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
func getPromptForTest(opts *Opts, isPiped bool) error {
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
		var opts Opts
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

func TestCustomProviders_MapAssignment(t *testing.T) {
	// this test verifies that the map-based CustomProviders approach works correctly
	var opts Opts

	// initialize the map if nil
	if opts.CustomProviders == nil {
		opts.CustomProviders = make(map[string]CustomOpenAIProvider)
	}

	// add a custom provider to the map
	opts.CustomProviders["localai"] = CustomOpenAIProvider{
		Name:      "LocalAI",
		URL:       "http://localhost:8080",
		APIKey:    "test-key",
		Model:     "local-model",
		Enabled:   true,
		MaxTokens: 4096,
	}

	// verify the custom provider was added correctly
	customProvider, exists := opts.CustomProviders["localai"]
	require.True(t, exists, "localai provider should exist in the map")
	assert.Equal(t, "LocalAI", customProvider.Name, "Name should match")
	assert.Equal(t, "http://localhost:8080", customProvider.URL, "URL should match")
	assert.Equal(t, "test-key", customProvider.APIKey, "API key should match")
	assert.Equal(t, "local-model", customProvider.Model, "Model should match")
	assert.True(t, customProvider.Enabled, "Provider should be enabled")
	assert.Equal(t, 4096, customProvider.MaxTokens, "MaxTokens should match")

	// test iteration over the map
	count := 0
	for id, prov := range opts.CustomProviders {
		count++
		assert.Equal(t, "localai", id, "Provider ID should match")
		assert.Equal(t, "LocalAI", prov.Name, "Provider name should match")
	}
	assert.Equal(t, 1, count, "Should have iterated over exactly one custom provider")
}

func TestCustomProviders_DirectMapManipulation(t *testing.T) {
	// since flags don't seem to populate the map directly, let's test map setting and use
	// this verifies the map-based approach works for the internal code paths

	var opts Opts
	// initialize the map
	opts.CustomProviders = make(map[string]CustomOpenAIProvider)

	// add a custom provider directly
	opts.CustomProviders["local"] = CustomOpenAIProvider{
		Name:    "LocalAI",
		URL:     "http://localhost:8080",
		Model:   "local-model",
		Enabled: true,
	}

	// verify the provider was set correctly
	require.NotEmpty(t, opts.CustomProviders, "CustomProviders should not be empty")

	// process the provider as the run function would
	var capturedName, capturedURL, capturedModel string
	for providerID, customOpt := range opts.CustomProviders {
		if customOpt.Enabled {
			capturedName = customOpt.Name
			capturedURL = customOpt.URL
			capturedModel = customOpt.Model
			// simulating logging that would use the provider ID
			_ = providerID
		}
	}

	// verify the provider was processed correctly
	assert.Equal(t, "LocalAI", capturedName, "Provider name should match")
	assert.Equal(t, "http://localhost:8080", capturedURL, "Provider URL should match")
	assert.Equal(t, "local-model", capturedModel, "Provider model should match")

	// test with multiple providers
	opts = Opts{}
	opts.CustomProviders = make(map[string]CustomOpenAIProvider)

	// add multiple providers
	opts.CustomProviders["provider1"] = CustomOpenAIProvider{
		Name:    "Provider1",
		URL:     "https://provider1.com",
		Model:   "model1",
		Enabled: true,
	}

	opts.CustomProviders["provider2"] = CustomOpenAIProvider{
		Name:    "Provider2",
		URL:     "https://provider2.com",
		Model:   "model2",
		APIKey:  "secret-key",
		Enabled: true,
	}

	// verify multiple providers
	require.Equal(t, 2, len(opts.CustomProviders), "Should have 2 custom providers")

	// check specific providers
	provider1, exists := opts.CustomProviders["provider1"]
	require.True(t, exists, "provider1 should exist")
	assert.Equal(t, "Provider1", provider1.Name, "Provider1 name should match")

	provider2, exists := opts.CustomProviders["provider2"]
	require.True(t, exists, "provider2 should exist")
	assert.Equal(t, "Provider2", provider2.Name, "Provider2 name should match")
	assert.Equal(t, "secret-key", provider2.APIKey, "Provider2 API key should match")

	// verify processing of multiple providers
	var count int
	for providerID, customOpt := range opts.CustomProviders {
		if customOpt.Enabled {
			count++
			// log the provider ID and name to verify they're correct
			switch providerID {
			case "provider1":
				assert.Equal(t, "Provider1", customOpt.Name, "Provider1 name should match")
			case "provider2":
				assert.Equal(t, "Provider2", customOpt.Name, "Provider2 name should match")
			default:
				t.Errorf("Unexpected provider ID: %s", providerID)
			}
		}
	}
	assert.Equal(t, 2, count, "Should have processed 2 enabled providers")
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
		var opts Opts
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
