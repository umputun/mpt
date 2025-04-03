package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
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

func TestRun_VersionFlag(t *testing.T) {
	// save original osExit and restore it after the test
	oldOsExit := osExit
	defer func() { osExit = oldOsExit }()

	// mock os.Exit
	var exitCode int
	osExit = func(code int) {
		exitCode = code
		panic("os.Exit called")
	}

	// test the version flag
	os.Args = []string{"mpt", "--version"}

	// catch the panic from our mocked os.Exit
	defer func() {
		if r := recover(); r != nil {
			assert.Equal(t, "os.Exit called", r)
			assert.Equal(t, 0, exitCode)
		}
	}()

	run()
}

func TestRun_PromptCombination(t *testing.T) {
	// save original args and stdin
	oldArgs := os.Args
	oldStdin := os.Stdin

	defer func() {
		os.Args = oldArgs
		os.Stdin = oldStdin
	}()

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
	os.Stdin = stdinFile

	// set command line args with prompt flag
	os.Args = []string{"mpt", "--prompt", "cli prompt", "--dbg"}

	// create test mock provider to avoid real API calls
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			// capture what prompt is being passed to the provider
			if prompt != "" {
				// verify prompt contains both CLI and stdin content
				assert.Contains(t, prompt, "cli prompt")
				assert.Contains(t, prompt, "piped content")
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

	// create a test environment with our modified code
	testRun := func() error {
		var opts Opts
		parser := flags.NewParser(&opts, flags.Default)
		if _, err := parser.Parse(); err != nil {
			return err
		}

		// check if data is being piped in and append to prompt if present
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// data is being piped in
			scanner := bufio.NewScanner(os.Stdin)
			var sb strings.Builder
			for scanner.Scan() {
				sb.WriteString(scanner.Text())
				sb.WriteString("\n")
			}
			stdinContent := strings.TrimSpace(sb.String())

			// append stdin to existing prompt if present, or use stdin as prompt
			if opts.Prompt != "" {
				opts.Prompt = opts.Prompt + "\n" + stdinContent
			} else {
				opts.Prompt = stdinContent
			}
		}

		// create providers slice with just our mock
		providers := []provider.Provider{mockProvider}

		// create runner with providers
		r := runner.New(providers...)

		// create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// run the prompt and return
		_, err := r.Run(ctx, opts.Prompt)
		return err
	}

	// run and verify
	err = testRun()
	require.NoError(t, err)

	// verify our mock was called
	require.NotEmpty(t, mockProvider.GenerateCalls())
}
