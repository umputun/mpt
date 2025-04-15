package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/mpt/pkg/mcp"
	"github.com/umputun/mpt/pkg/prompt"
	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/provider/enum"
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
	Timeout  time.Duration `short:"t" long:"timeout" default:"60s" description:"timeout duration"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Verbose bool `short:"v" long:"verbose" description:"verbose output, shows prompt sent to models"`
	Version bool `short:"V" long:"version" description:"show version info"`
	JSON    bool `long:"json" description:"output in JSON format for scripting and automation"`
}

// openAIOpts defines options for OpenAI provider
type openAIOpts struct {
	Enabled     bool    `long:"enabled" env:"ENABLED" description:"enable OpenAI provider"`
	APIKey      string  `long:"api-key" env:"API_KEY" description:"OpenAI API key"`
	Model       string  `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-4.1"`
	MaxTokens   int     `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
	Temperature float32 `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-1, higher is more random)" default:"0.7"`
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
	Enabled     bool    `long:"enabled" env:"ENABLED" description:"enable custom provider"`
	Name        string  `long:"name" env:"NAME" description:"custom provider name" default:"custom"`
	URL         string  `long:"url" env:"URL" description:"Base URL for the custom provider API"`
	APIKey      string  `long:"api-key" env:"API_KEY" description:"API key for the custom provider (if needed)"`
	Model       string  `long:"model" env:"MODEL" description:"Model to use for the custom provider"`
	MaxTokens   int     `long:"max-tokens" env:"MAX_TOKENS" description:"Maximum number of tokens to generate" default:"16384"`
	Temperature float32 `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-1, higher is more random)" default:"0.7"`
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

	// if version flag is set, print version and exit
	if opts.Version {
		fmt.Printf("MPT version %s\n", revision)
		os.Exit(0)
	}

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

	// initialize providers and handle errors
	providers, err := initializeProviders(opts)
	if err != nil {
		return err
	}

	return executePrompt(ctx, opts, providers)
}

// runMCPServer starts MPT in MCP server mode
func runMCPServer(_ context.Context, opts *options) error {
	// setup logging with API keys as secrets
	secrets := collectSecrets(opts)
	setupLog(opts.Debug, secrets...)

	// initialize all providers and handle errors
	providers, err := initializeProviders(opts)
	if err != nil {
		return fmt.Errorf("failed to initialize providers for MCP server mode: %w", err)
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
	if err := buildFullPrompt(opts); err != nil {
		return err
	}

	return nil
}

// buildFullPrompt loads content from specified files and builds the complete prompt
func buildFullPrompt(opts *options) error {
	// use the prompt builder to handle file loading and prompt construction
	builder := prompt.New(opts.Prompt).
		WithFiles(opts.Files).
		WithExcludes(opts.Excludes)

	fullPrompt, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	opts.Prompt = fullPrompt
	return nil
}

// createStandardProvider creates a provider instance for standard providers (OpenAI, Anthropic, Google)
func createStandardProvider(providerType, apiKey, model string, maxTokens int, temperature float32) provider.Provider {
	// all standard providers use the same options structure
	opts := provider.Options{
		APIKey:      apiKey,
		Model:       model,
		Enabled:     true,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}

	// parse provider type string to enum
	pType, err := enum.ParseProviderType(providerType)
	if err != nil {
		lgr.Printf("[ERROR] unknown provider type: %s", providerType)
		return nil
	}

	p, err := provider.CreateProvider(pType, opts)
	if err != nil {
		lgr.Printf("[ERROR] failed to create %s provider: %v", providerType, err)
		return nil
	}
	return p
}

// initializeProviders creates provider instances from the options
func initializeProviders(opts *options) ([]provider.Provider, error) {
	// create a slice to hold enabled providers
	providers := []provider.Provider{}
	providerErrors := []string{}

	// check if any providers are enabled
	if !anyProvidersEnabled(opts) {
		return nil, fmt.Errorf("no providers enabled. Use --<provider>.enabled flag to enable at least one provider (e.g., --openai.enabled)")
	}

	// initialize each enabled provider
	if opts.OpenAI.Enabled {
		initializeOpenAIProvider(opts, &providers, &providerErrors)
	}

	if opts.Anthropic.Enabled {
		initializeAnthropicProvider(opts, &providers, &providerErrors)
	}

	if opts.Google.Enabled {
		initializeGoogleProvider(opts, &providers, &providerErrors)
	}

	if opts.Custom.Enabled {
		initializeCustomProvider(opts, &providers, &providerErrors)
	}

	// check if any providers were successfully initialized
	if len(providers) == 0 {
		return nil, fmt.Errorf("all enabled providers failed to initialize:\n%s", strings.Join(providerErrors, "\n"))
	}

	return providers, nil
}

// anyProvidersEnabled checks if at least one provider is enabled in the options
func anyProvidersEnabled(opts *options) bool {
	return opts.OpenAI.Enabled || opts.Anthropic.Enabled ||
		opts.Google.Enabled || opts.Custom.Enabled
}

// initializeOpenAIProvider initializes the OpenAI provider and adds it to the providers list
func initializeOpenAIProvider(opts *options, providers *[]provider.Provider, providerErrors *[]string) {
	p, err := provider.CreateProvider(enum.ProviderTypeOpenAI, provider.Options{
		APIKey:      opts.OpenAI.APIKey,
		Model:       opts.OpenAI.Model,
		Enabled:     true,
		MaxTokens:   opts.OpenAI.MaxTokens,
		Temperature: opts.OpenAI.Temperature,
	})

	if err != nil {
		lgr.Printf("[WARN] OpenAI provider failed to initialize: %v", err)
		*providerErrors = append(*providerErrors, fmt.Sprintf("OpenAI: %v", err))
		return
	}

	*providers = append(*providers, p)
	lgr.Printf("[DEBUG] added OpenAI provider, model: %s, temperature: %.2f",
		opts.OpenAI.Model, opts.OpenAI.Temperature)
}

// initializeAnthropicProvider initializes the Anthropic provider and adds it to the providers list
func initializeAnthropicProvider(opts *options, providers *[]provider.Provider, providerErrors *[]string) {
	p, err := provider.CreateProvider(enum.ProviderTypeAnthropic, provider.Options{
		APIKey:      opts.Anthropic.APIKey,
		Model:       opts.Anthropic.Model,
		Enabled:     true,
		MaxTokens:   opts.Anthropic.MaxTokens,
		Temperature: 0.7, // default temperature
	})

	if err != nil {
		lgr.Printf("[WARN] Anthropic provider failed to initialize: %v", err)
		*providerErrors = append(*providerErrors, fmt.Sprintf("Anthropic: %v", err))
		return
	}

	*providers = append(*providers, p)
	lgr.Printf("[DEBUG] added Anthropic provider, model: %s", opts.Anthropic.Model)
}

// initializeGoogleProvider initializes the Google provider and adds it to the providers list
func initializeGoogleProvider(opts *options, providers *[]provider.Provider, providerErrors *[]string) {
	p, err := provider.CreateProvider(enum.ProviderTypeGoogle, provider.Options{
		APIKey:      opts.Google.APIKey,
		Model:       opts.Google.Model,
		Enabled:     true,
		MaxTokens:   opts.Google.MaxTokens,
		Temperature: 0.7, // default temperature
	})

	if err != nil {
		lgr.Printf("[WARN] Google provider failed to initialize: %v", err)
		*providerErrors = append(*providerErrors, fmt.Sprintf("Google: %v", err))
		return
	}

	*providers = append(*providers, p)
	lgr.Printf("[DEBUG] added Google provider, model: %s", opts.Google.Model)
}

// initializeCustomProvider initializes the custom provider and adds it to the providers list
func initializeCustomProvider(opts *options, providers *[]provider.Provider, providerErrors *[]string) {
	// first validate required fields and collect errors
	customErr := validateCustomProvider(opts, providerErrors)
	if customErr != nil {
		lgr.Printf("[WARN] %s", customErr)
		return
	}

	// all validation passed, create and add the provider
	addCustomProvider(opts, providers, providerErrors)
}

// validateCustomProvider checks if the custom provider has all required fields
func validateCustomProvider(opts *options, providerErrors *[]string) error {
	// check if URL is missing
	if opts.Custom.URL == "" {
		*providerErrors = append(*providerErrors, fmt.Sprintf("Custom (%s): URL is required", opts.Custom.Name))
		return fmt.Errorf("custom provider %s failed to initialize: URL is required", opts.Custom.Name)
	}

	// check if model is missing
	if opts.Custom.Model == "" {
		*providerErrors = append(*providerErrors, fmt.Sprintf("Custom (%s): Model is required", opts.Custom.Name))
		return fmt.Errorf("custom provider %s failed to initialize: model is required", opts.Custom.Name)
	}

	return nil
}

// addCustomProvider creates and adds a custom provider to the providers list
func addCustomProvider(opts *options, providers *[]provider.Provider, providerErrors *[]string) {
	customProvider := provider.NewCustomOpenAI(provider.CustomOptions{
		Name:        opts.Custom.Name,
		BaseURL:     opts.Custom.URL,
		APIKey:      opts.Custom.APIKey,
		Model:       opts.Custom.Model,
		Enabled:     true,
		MaxTokens:   opts.Custom.MaxTokens,
		Temperature: opts.Custom.Temperature,
	})

	// check if the provider was enabled successfully
	if customProvider.Enabled() {
		*providers = append(*providers, customProvider)
		lgr.Printf("[DEBUG] added custom provider: %s, URL: %s, model: %s, temperature: %.2f",
			opts.Custom.Name, opts.Custom.URL, opts.Custom.Model, opts.Custom.Temperature)
	} else {
		*providerErrors = append(*providerErrors, fmt.Sprintf("Custom (%s): failed to initialize", opts.Custom.Name))
		lgr.Printf("[WARN] custom provider %s failed to initialize", opts.Custom.Name)
	}
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
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("operation timed out after %s, try increasing the timeout with -t flag", opts.Timeout)
		}
		return err
	}

	if opts.JSON {
		// output results in JSON format for scripting
		return outputJSON(result, r.GetResults())
	}

	// standard text output
	fmt.Println(strings.TrimSpace(result))
	return nil
}

// showVerbosePrompt displays the prompt text that will be sent to the models
func showVerbosePrompt(w io.Writer, opts options) {
	fmt.Fprintln(w, "=== Prompt sent to models ===")
	fmt.Fprintln(w, opts.Prompt)
	fmt.Fprintln(w, "=============================")
	fmt.Fprintln(w)
}

// getPrompt handles reading the prompt from stdin (piped or interactive) or command line
func getPrompt(opts *options) error {
	// check if input is coming from a pipe
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0

	if isPiped {
		// handle piped input
		stdinContent, err := readFromStdin()
		if err != nil {
			return err
		}

		// combine with existing prompt or use as prompt
		opts.Prompt = prompt.CombineWithInput(opts.Prompt, stdinContent)

	} else if opts.Prompt == "" {
		// no data piped, no prompt provided, interactive mode
		fmt.Print("Enter prompt: ")
		reader := bufio.NewReader(os.Stdin)
		promptText, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("error reading prompt: %w", err)
		}
		opts.Prompt = strings.TrimSpace(promptText)
	}
	return nil
}

func setupLog(dbg bool, secs ...string) {
	logOpts := []lgr.Option{lgr.Out(io.Discard), lgr.Err(io.Discard)} // default to discard
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}
	if len(secs) > 0 {
		logOpts = append(logOpts, lgr.Secret(secs...))
	}
	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}

// readFromStdin reads content from stdin and returns it as a trimmed string
func readFromStdin() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading from stdin: %w", err)
	}
	return strings.TrimSpace(sb.String()), nil
}

// outputJSON formats the results as JSON and prints them to stdout
func outputJSON(_ string, results []provider.Result) error {
	// create json output structure
	type ProviderResponse struct {
		Provider string `json:"provider"`
		Text     string `json:"text,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	type JSONOutput struct {
		Responses []ProviderResponse `json:"responses"` // individual provider responses
		Timestamp string             `json:"timestamp"`
	}

	// build responses array
	responses := make([]ProviderResponse, 0, len(results))
	for _, r := range results {
		resp := ProviderResponse{
			Provider: r.Provider,
			Text:     r.Text,
		}

		if r.Error != nil {
			resp.Error = r.Error.Error()
		}

		responses = append(responses, resp)
	}

	// create the output structure
	output := JSONOutput{
		Responses: responses,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// encode to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("error encoding JSON output: %w", err)
	}

	return nil
}
