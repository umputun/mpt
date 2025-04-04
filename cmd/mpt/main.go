package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/mpt/pkg/files"
	"github.com/umputun/mpt/pkg/mcp"
	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

// options with all CLI options
type options struct {
	OpenAI    openAIOpts    `group:"openai" namespace:"openai" env-namespace:"OPENAI"`
	Anthropic anthropicOpts `group:"anthropic" namespace:"anthropic" env-namespace:"ANTHROPIC"`
	Google    googleOpts    `group:"google" namespace:"google" env-namespace:"GOOGLE"`

	Custom customOpenAIProvider `group:"custom" namespace:"custom" env-namespace:"CUSTOM"`

	MCP mcpOpts `group:"mcp" namespace:"mcp" env-namespace:"MCP"`

	Prompt   string        `short:"p" long:"prompt" description:"prompt text (if not provided, will be read from stdin)"`
	Files    []string      `short:"f" long:"file" description:"files or glob patterns to include in the prompt context"`
	Excludes []string      `short:"x" long:"exclude" description:"patterns to exclude from file matching (e.g., 'vendor/**', '**/mocks/*')"`
	Timeout  time.Duration `short:"t" long:"timeout" default:"30s" description:"timeout duration"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Verbose bool `short:"v" long:"verbose" description:"verbose output, shows prompt sent to models"`
	Version bool `short:"V" long:"version" description:"show version info"`
	NoColor bool `long:"no-color" env:"NO_COLOR" description:"disable color output"`
}

// openAIOpts defines options for OpenAI provider
type openAIOpts struct {
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable OpenAI provider"`
	APIKey    string `long:"api-key" env:"API_KEY" description:"OpenAI API key"`
	Model     string `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-4o"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// anthropicOpts defines options for Anthropic provider
type anthropicOpts struct {
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable Anthropic provider"`
	APIKey    string `long:"api-key" env:"API_KEY" description:"Anthropic API key"`
	Model     string `long:"model" env:"MODEL" description:"Anthropic model" default:"claude-3-7-sonnet-20250219"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// googleOpts defines options for Google provider
type googleOpts struct {
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable Google provider"`
	APIKey    string `long:"api-key" env:"API_KEY" description:"Google API key"`
	Model     string `long:"model" env:"MODEL" description:"Google model" default:"gemini-2.5-pro-exp-03-25"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// mcpOpts defines options for MCP server mode
type mcpOpts struct {
	Server     bool   `long:"server" env:"SERVER" description:"run in MCP server mode"`
	ServerName string `long:"server-name" env:"SERVER_NAME" description:"MCP server name" default:"MPT MCP Server"`
}

// customOpenAIProvider defines options for a custom OpenAI-compatible provider
type customOpenAIProvider struct {
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable custom provider"`
	Name      string `long:"name" env:"NAME" description:"custom provider name" default:"custom"`
	URL       string `long:"url" env:"URL" description:"Base URL for the custom provider API"`
	APIKey    string `long:"api-key" env:"API_KEY" description:"API key for the custom provider (if needed)"`
	Model     string `long:"model" env:"MODEL" description:"Model to use for the custom provider"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"Maximum number of tokens to generate" default:"16384"`
}

var revision = "unknown"

func main() {
	opts := &options{}
	p := flags.NewParser(opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	if _, err := p.Parse(); err != nil {
		if !errors.Is(err.(*flags.Error).Type, flags.ErrHelp) {
			fmt.Printf("%v", err)
		}
		os.Exit(1)
	}
	setupLog(opts.Debug, collectSecrets(opts)...)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, opts); err != nil {
		lgr.Printf("[ERROR] %v", err)              // log the error with detailed info for debugging
		fmt.Fprintf(os.Stderr, "Error: %v\n", err) // print a user-friendly error message to stderr

		os.Exit(1) //nolint:gocritic
	}
}

// run executes the main program logic and returns an error if it fails
func run(ctx context.Context, opts *options) error {
	// check if running in MCP server mode
	if opts.MCP.Server {
		return runMCPServer(ctx, opts)
	}

	// standard MPT mode

	// process the prompt (from CLI args or stdin)
	if err := processPrompt(opts); err != nil {
		return err
	}
	providers := initializeProviders(opts)
	return executePrompt(ctx, opts, providers)
}

// runMCPServer starts MPT in MCP server mode
func runMCPServer(_ context.Context, opts *options) error {
	// setup logging with API keys as secrets
	secrets := collectSecrets(opts)
	setupLog(opts.Debug, secrets...)

	// initialize all providers
	providers := initializeProviders(opts)
	if len(providers) == 0 {
		return fmt.Errorf("no providers enabled for MCP server mode")
	}

	// create runner with all providers
	r := runner.New(providers...)

	// create MCP server using our runner
	mcpServer := mcp.NewServer(r, mcp.ServerOptions{
		Name:    opts.MCP.ServerName,
		Version: revision,
	})

	lgr.Printf("[INFO] MCP server initialized with %d providers", len(providers))
	lgr.Printf("[INFO] server name: %s, version: %s", opts.MCP.ServerName, revision)

	// print enabled providers
	for _, p := range providers {
		lgr.Printf("[INFO] enabled provider: %s", p.Name())
	}

	// start the MCP server
	lgr.Printf("[INFO] starting MPT in MCP server mode with stdio transport")
	return mcpServer.Start()
}

// collectSecrets extracts all API keys for secure logging
func collectSecrets(opts *options) []string {
	var secrets []string

	// add API keys from built-in providers
	if opts.OpenAI.APIKey != "" {
		secrets = append(secrets, opts.OpenAI.APIKey)
	}
	if opts.Anthropic.APIKey != "" {
		secrets = append(secrets, opts.Anthropic.APIKey)
	}
	if opts.Google.APIKey != "" {
		secrets = append(secrets, opts.Google.APIKey)
	}

	// add API key from custom provider
	if opts.Custom.APIKey != "" {
		secrets = append(secrets, opts.Custom.APIKey)
	}

	return secrets
}

// processPrompt gets the prompt from stdin or command line and optionally adds file content
func processPrompt(opts *options) error {
	// get prompt from stdin (piped data or interactive input) or command line
	if err := getPrompt(opts); err != nil {
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	// check if we have a prompt after all attempts
	if opts.Prompt == "" {
		return fmt.Errorf("no prompt provided")
	}

	// append file content to prompt if requested
	if err := appendFileContent(opts); err != nil {
		return err
	}

	return nil
}

// appendFileContent loads content from specified files and appends to the prompt
func appendFileContent(opts *options) error {
	if len(opts.Files) == 0 {
		return nil
	}

	lgr.Printf("[DEBUG] loading files from patterns: %v", opts.Files)
	if len(opts.Excludes) > 0 {
		lgr.Printf("[DEBUG] excluding patterns: %v", opts.Excludes)
	}

	fileContent, err := files.LoadContent(opts.Files, opts.Excludes)
	if err != nil {
		return fmt.Errorf("failed to load files: %w", err)
	}

	if fileContent != "" {
		lgr.Printf("[DEBUG] loaded %d bytes of content from files", len(fileContent))
		opts.Prompt += "\n\n" + fileContent
	}

	return nil
}

// createStandardProvider creates a provider instance for standard providers (OpenAI, Anthropic, Google)
func createStandardProvider(providerType, apiKey, model string, maxTokens int) provider.Provider {
	// all standard providers use the same options structure
	opts := provider.Options{
		APIKey:    apiKey,
		Model:     model,
		Enabled:   true,
		MaxTokens: maxTokens,
	}

	switch providerType {
	case "openai":
		return provider.NewOpenAI(opts)
	case "anthropic":
		return provider.NewAnthropic(opts)
	case "google":
		return provider.NewGoogle(opts)
	default:
		lgr.Printf("[ERROR] unknown provider type: %s", providerType)
		return nil
	}
}

// initializeProviders creates provider instances from the options
func initializeProviders(opts *options) []provider.Provider {
	// create a slice to hold enabled providers
	providers := []provider.Provider{}

	// add OpenAI provider only if enabled
	if opts.OpenAI.Enabled {
		p := createStandardProvider("openai", opts.OpenAI.APIKey, opts.OpenAI.Model, opts.OpenAI.MaxTokens)
		providers = append(providers, p)
		lgr.Printf("[DEBUG] added OpenAI provider, model: %s", opts.OpenAI.Model)
	}

	// add Anthropic provider only if enabled
	if opts.Anthropic.Enabled {
		p := createStandardProvider("anthropic", opts.Anthropic.APIKey, opts.Anthropic.Model, opts.Anthropic.MaxTokens)
		providers = append(providers, p)
		lgr.Printf("[DEBUG] added Anthropic provider, model: %s", opts.Anthropic.Model)
	}

	// add Google provider only if enabled
	if opts.Google.Enabled {
		p := createStandardProvider("google", opts.Google.APIKey, opts.Google.Model, opts.Google.MaxTokens)
		providers = append(providers, p)
		lgr.Printf("[DEBUG] added Google provider, model: %s", opts.Google.Model)
	}

	// add custom provider if enabled (handled separately due to different options structure)
	if opts.Custom.Enabled && opts.Custom.URL != "" && opts.Custom.Model != "" {
		customProvider := provider.NewCustomOpenAI(provider.CustomOptions{
			Name:      opts.Custom.Name,
			BaseURL:   opts.Custom.URL,
			APIKey:    opts.Custom.APIKey,
			Model:     opts.Custom.Model,
			Enabled:   true, // if we got here, it's enabled
			MaxTokens: opts.Custom.MaxTokens,
		})
		providers = append(providers, customProvider)
		lgr.Printf("[INFO] added custom provider: %s, URL: %s, model: %s", "custom", opts.Custom.URL, opts.Custom.Model)
	}

	return providers
}

// executePrompt runs the prompt against the configured providers
func executePrompt(ctx context.Context, opts *options, providers []provider.Provider) error {
	// create runner with all providers
	r := runner.New(providers...)

	// create timeout context as a child of the passed ctx (which handles interrupts)
	timeoutCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// show prompt in verbose mode
	if opts.Verbose {
		showVerbosePrompt(os.Stdout, *opts)
	}

	// run the prompt
	result, err := r.Run(timeoutCtx, opts.Prompt)
	if err != nil {
		// handle different error types with more specific messages
		switch {
		case errors.Is(err, context.Canceled):
			return fmt.Errorf("operation canceled by user")
		case errors.Is(err, context.DeadlineExceeded):
			return fmt.Errorf("operation timed out after %s, try increasing the timeout with -t flag", opts.Timeout)
		default:
			// check if we have an error from any specific provider
			if strings.Contains(err.Error(), "api error") || strings.Contains(err.Error(), "API error") {
				return fmt.Errorf("provider API error: %w", err)
			}
			// generic fallback for other errors
			return fmt.Errorf("failed to run prompt: %w", err)
		}
	}

	// print the result
	fmt.Println(strings.TrimSpace(result))
	return nil
}

// showVerbosePrompt displays the prompt text that will be sent to the models
func showVerbosePrompt(w io.Writer, opts options) {
	// use colored output if not disabled
	if opts.NoColor {
		fmt.Fprintln(w, "=== Prompt sent to models ===")
		fmt.Fprintln(w, opts.Prompt)
		fmt.Fprintln(w, "============================")
	} else {
		headerColor := color.New(color.FgCyan, color.Bold)
		fmt.Fprintln(w, headerColor.Sprint("=== Prompt sent to models ==="))
		fmt.Fprintln(w, opts.Prompt)
		fmt.Fprintln(w, headerColor.Sprint("============================"))
	}
	fmt.Fprintln(w)
}

// getPrompt handles reading the prompt from stdin (piped or interactive) or command line
func getPrompt(opts *options) error {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// data is being piped in
		scanner := bufio.NewScanner(os.Stdin)
		var sb strings.Builder
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading from stdin: %w", err)
		}
		stdinContent := strings.TrimSpace(sb.String())

		// append stdin to existing prompt if present, or use stdin as prompt
		if opts.Prompt != "" {
			opts.Prompt = opts.Prompt + "\n" + stdinContent
		} else {
			opts.Prompt = stdinContent
		}
	} else if opts.Prompt == "" {
		// no data piped, no prompt provided, interactive mode
		fmt.Print("Enter prompt: ")
		reader := bufio.NewReader(os.Stdin)
		prompt, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error reading prompt: %w", err)
		}
		opts.Prompt = strings.TrimSpace(prompt)
	}
	return nil
}

func setupLog(dbg bool, secs ...string) {
	logOpts := []lgr.Option{lgr.Out(io.Discard), lgr.Err(io.Discard)} // default to discard
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}

	colorizer := lgr.Mapper{
		ErrorFunc:  func(s string) string { return color.New(color.FgHiRed).Sprint(s) },
		WarnFunc:   func(s string) string { return color.New(color.FgRed).Sprint(s) },
		InfoFunc:   func(s string) string { return color.New(color.FgYellow).Sprint(s) },
		DebugFunc:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
		CallerFunc: func(s string) string { return color.New(color.FgBlue).Sprint(s) },
		TimeFunc:   func(s string) string { return color.New(color.FgCyan).Sprint(s) },
	}
	logOpts = append(logOpts, lgr.Map(colorizer))
	if len(secs) > 0 {
		logOpts = append(logOpts, lgr.Secret(secs...))
	}
	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}
