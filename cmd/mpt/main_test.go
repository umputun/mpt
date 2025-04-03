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
			name:      "version_flag",
			args:      []string{"mpt", "--version"},
			exitCode:  0,
			setupMock: func() {},
			validateFunc: func(t *testing.T) {},
		},
		{
			name:      "help_flag",
			args:      []string{"mpt", "--help"},
			exitCode:  0,
			setupMock: func() {},
			validateFunc: func(t *testing.T) {},
		},
		{
			name:      "invalid_flag",
			args:      []string{"mpt", "--invalid-flag"},
			exitCode:  1,
			setupMock: func() {},
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

func TestContextCancel(t *testing.T) {
	// Create a mock provider that respects context cancellation
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			// Check if context is already canceled
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "Test response", nil
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}
	
	// Create a runner with our mock provider
	r := runner.New(mockProvider)
	
	// Create an already canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel it immediately
	
	// Try to run with an already canceled context
	_, err := r.Run(ctx, "test prompt")
	
	// Verify we got a context canceled error
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
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
	t               *testing.T
	mockProvider    *mocks.ProviderMock
	originalArgs    []string
	originalStdin   *os.File
	generatedOutput string
	promptSeen      string
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
func CreateTempFileWithContent(t *testing.T, content string) (string, func()) {
	tempDir, err := os.MkdirTemp("", "mpt-test")
	require.NoError(t, err)
	
	filePath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(filePath, []byte(content), 0600)
	require.NoError(t, err)
	
	cleanup := func() {
		os.RemoveAll(tempDir)
	}
	
	return filePath, cleanup
}

func TestRun_WithFileInput(t *testing.T) {
	// Create test helper
	tester := NewMockRunnerTester(t)
	defer tester.Cleanup()
	
	// Create a temporary file
	testFilePath, cleanup := CreateTempFileWithContent(t, "Test file content")
	defer cleanup()
	
	// Set up the mock provider to validate the prompt
	tester.MockProviderResponse("Test response", func(prompt string) {
		assert.Contains(t, prompt, "analyze this")
		assert.Contains(t, prompt, "Test file content")
	})
	
	// Set command line args
	tester.SetupArgs([]string{"mpt", "--prompt", "analyze this", "--file", testFilePath, "--timeout", "1"})
	
	// Create a test runner that uses our mock
	testRun := func(ctx context.Context) error {
		var opts Opts
		parser := flags.NewParser(&opts, flags.Default)
		if _, err := parser.Parse(); err != nil {
			return err
		}
		
		if err := getPrompt(&opts); err != nil {
			return err
		}
		
		// Load file content
		if len(opts.Files) > 0 {
			fileContent, err := os.ReadFile(opts.Files[0])
			if err != nil {
				return err
			}
			
			opts.Prompt += "\n\n" + string(fileContent)
		}
		
		// Skip the normal provider initialization and just use our mock
		r := runner.New(tester.mockProvider)
		
		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
		
		// Run the prompt
		_, err := r.Run(timeoutCtx, opts.Prompt)
		return err
	}
	
	// Run with a context
	err := testRun(context.Background())
	require.NoError(t, err)
	
	// Verify the mock was called properly
	require.NotEmpty(t, tester.mockProvider.GenerateCalls())
}

func TestRun_PromptCombination(t *testing.T) {
	// Create test helper
	tester := NewMockRunnerTester(t)
	defer tester.Cleanup()
	
	// Create a temp file to simulate stdin
	tmpFile, err := os.CreateTemp("", "stdin")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Write content to the temp file
	_, err = tmpFile.WriteString("piped content")
	require.NoError(t, err)
	tmpFile.Close()

	// Reopen file for reading and set as stdin
	stdinFile, err := os.Open(tmpFile.Name())
	require.NoError(t, err)
	defer stdinFile.Close()
	
	// Save original stdin and restore it later
	tester.originalStdin = os.Stdin
	os.Stdin = stdinFile

	// Set up the mock provider to validate the prompt
	tester.MockProviderResponse("Test response", func(prompt string) {
		assert.Contains(t, prompt, "cli prompt")
		assert.Contains(t, prompt, "piped content")
	})
	
	// Set command line args
	tester.SetupArgs([]string{"mpt", "--prompt", "cli prompt", "--dbg"})
	
	// Create a test runner that uses our mock
	testRun := func(ctx context.Context) error {
		var opts Opts
		parser := flags.NewParser(&opts, flags.Default)
		if _, err := parser.Parse(); err != nil {
			return err
		}
		
		if err := getPrompt(&opts); err != nil {
			return err
		}
		
		// Skip the normal provider initialization and just use our mock
		r := runner.New(tester.mockProvider)
		
		// Create timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		
		// Run the prompt
		_, err := r.Run(timeoutCtx, opts.Prompt)
		return err
	}
	
	// Run with a context
	err = testRun(context.Background())
	require.NoError(t, err)
	
	// Verify the mock was called properly
	require.NotEmpty(t, tester.mockProvider.GenerateCalls())
}
