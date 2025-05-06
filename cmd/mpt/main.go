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
	"strconv"
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
	Git gitOpts `group:"git" namespace:"git" env-namespace:"GIT"`

	Prompt      string        `short:"p" long:"prompt" description:"prompt text (if not provided, will be read from stdin)"`
	Files       []string      `short:"f" long:"file" description:"files or glob patterns to include in the prompt context"`
	Excludes    []string      `short:"x" long:"exclude" description:"patterns to exclude from file matching (e.g., 'vendor/**', '**/mocks/*')"`
	Timeout     time.Duration `short:"t" long:"timeout" default:"60s" description:"timeout duration"`
	MaxFileSize SizeValue     `long:"max-file-size" env:"MAX_FILE_SIZE" default:"65536" description:"maximum size of individual files to process in bytes (default: 64KB, supports k/m suffixes)"`

	// mix options
	MixEnabled  bool   `long:"mix" env:"MIX" description:"enable mix (merge) results from all providers"`
	MixProvider string `long:"mix.provider" env:"MIX_PROVIDER" default:"openai" description:"provider used to mix results"`
	MixPrompt   string `long:"mix.prompt" env:"MIX_PROMPT" default:"merge results from all providers" description:"prompt used to mix results"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Verbose bool `short:"v" long:"verbose" description:"verbose output, shows prompt sent to models"`
	Version bool `short:"V" long:"version" description:"show version info"`
	JSON    bool `long:"json" description:"output in JSON format for scripting and automation"`
}

// openAIOpts defines options for OpenAI provider
type openAIOpts struct {
	Enabled     bool      `long:"enabled" env:"ENABLED" description:"enable OpenAI provider"`
	APIKey      string    `long:"api-key" env:"API_KEY" description:"OpenAI API key"`
	Model       string    `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-4.1"`
	MaxTokens   SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate (default: 16384, supports k/m suffixes)" default:"16384"`
	Temperature float32   `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-1, higher is more random)" default:"0.7"`
}

// anthropicOpts defines options for Anthropic provider
type anthropicOpts struct {
	Enabled   bool      `long:"enabled" env:"ENABLED" description:"enable Anthropic provider"`
	APIKey    string    `long:"api-key" env:"API_KEY" description:"Anthropic API key"`
	Model     string    `long:"model" env:"MODEL" description:"Anthropic model" default:"claude-3-7-sonnet-20250219"`
	MaxTokens SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate (default: 16384, supports k/m suffixes)" default:"16384"`
}

// googleOpts defines options for Google provider
type googleOpts struct {
	Enabled   bool      `long:"enabled" env:"ENABLED" description:"enable Google provider"`
	APIKey    string    `long:"api-key" env:"API_KEY" description:"Google API key"`
	Model     string    `long:"model" env:"MODEL" description:"Google model" default:"gemini-2.5-pro-exp-03-25"`
	MaxTokens SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate (default: 16384, supports k/m suffixes)" default:"16384"`
}

// mcpOpts defines options for MCP server mode
type mcpOpts struct {
	Server     bool   `long:"server" env:"SERVER" description:"run in MCP server mode"`
	ServerName string `long:"server-name" env:"SERVER_NAME" description:"MCP server name" default:"MPT MCP Server"`
}

// customOpenAIProvider defines options for a custom OpenAI-compatible provider
type customOpenAIProvider struct {
	Enabled     bool      `long:"enabled" env:"ENABLED" description:"enable custom provider"`
	Name        string    `long:"name" env:"NAME" description:"custom provider name" default:"custom"`
	URL         string    `long:"url" env:"URL" description:"Base URL for the custom provider API"`
	APIKey      string    `long:"api-key" env:"API_KEY" description:"API key for the custom provider (if needed)"`
	Model       string    `long:"model" env:"MODEL" description:"Model to use for the custom provider"`
	MaxTokens   SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"Maximum number of tokens to generate (default: 16384, supports k/m suffixes)" default:"16384"`
	Temperature float32   `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-1, higher is more random)" default:"0.7"`
}

// gitOpts defines options for Git integration
type gitOpts struct {
	Diff   bool   `long:"diff" env:"DIFF" description:"include git diff as context (uncommitted changes)"`
	Branch string `long:"branch" env:"BRANCH" description:"include git diff between given branch and master/main (for PR review)"`
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
		WithExcludes(opts.Excludes).
		WithMaxFileSize(int64(opts.MaxFileSize))

	// add git diff if requested
	var err error
	if opts.Git.Diff {
		builder, err = builder.WithGitDiff()
		if err != nil {
			return fmt.Errorf("failed to process git diff: %w", err)
		}
	}

	// add git branch diff if requested
	if opts.Git.Branch != "" {
		builder, err = builder.WithGitBranchDiff(opts.Git.Branch)
		if err != nil {
			return fmt.Errorf("failed to process git branch diff: %w", err)
		}
	}

	// build the prompt
	fullPrompt, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// schedule cleanup of git diff files when program exits
	defer prompt.CleanupGitDiffFiles()

	opts.Prompt = fullPrompt
	return nil
}

// initializeProvider creates a provider instance for a given provider type
func initializeProvider(provType enum.ProviderType, apiKey, model string, maxTokens int, temperature float32) (provider.Provider, error) {
	p, err := provider.CreateProvider(provType, provider.Options{
		APIKey:      apiKey,
		Model:       model,
		Enabled:     true,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	})

	if err != nil {
		return nil, err
	}
	return p, nil
}

// initializeProviders creates provider instances from the options
func initializeProviders(opts *options) ([]provider.Provider, error) {
	// create slices to hold enabled providers and errors
	providers := make([]provider.Provider, 0, 4) // pre-allocate for 4 providers (3 standard + 1 custom)
	providerErrors := make([]string, 0)

	// check if any providers are enabled
	if !anyProvidersEnabled(opts) {
		return nil, fmt.Errorf("no providers enabled. Use --<provider>.enabled flag to enable at least one provider (e.g., --openai.enabled)")
	}

	// initialize standard providers
	type providerConfig struct {
		enabled   bool
		provType  enum.ProviderType
		name      string
		apiKey    string
		model     string
		maxTokens int
		temp      float32
	}

	// define provider configurations
	standardProviders := []providerConfig{
		{
			enabled:   opts.OpenAI.Enabled,
			provType:  enum.ProviderTypeOpenAI,
			name:      "OpenAI",
			apiKey:    opts.OpenAI.APIKey,
			model:     opts.OpenAI.Model,
			maxTokens: int(opts.OpenAI.MaxTokens),
			temp:      opts.OpenAI.Temperature,
		},
		{
			enabled:   opts.Anthropic.Enabled,
			provType:  enum.ProviderTypeAnthropic,
			name:      "Anthropic",
			apiKey:    opts.Anthropic.APIKey,
			model:     opts.Anthropic.Model,
			maxTokens: int(opts.Anthropic.MaxTokens),
			temp:      0.7, // default temperature for Anthropic
		},
		{
			enabled:   opts.Google.Enabled,
			provType:  enum.ProviderTypeGoogle,
			name:      "Google",
			apiKey:    opts.Google.APIKey,
			model:     opts.Google.Model,
			maxTokens: int(opts.Google.MaxTokens),
			temp:      0.7, // default temperature for Google
		},
	}

	// initialize each enabled standard provider
	for _, config := range standardProviders {
		if !config.enabled {
			continue
		}

		p, err := initializeProvider(config.provType, config.apiKey, config.model, config.maxTokens, config.temp)
		if err != nil {
			lgr.Printf("[WARN] %s provider failed to initialize: %v", config.name, err)
			providerErrors = append(providerErrors, fmt.Sprintf("%s: %v", config.name, err))
			continue
		}

		providers = append(providers, p)
		lgr.Printf("[DEBUG] added %s provider, model: %s", config.name, config.model)
	}

	// initialize custom provider if enabled
	if opts.Custom.Enabled {
		p, err := initializeCustomProvider(opts)
		if err != nil {
			lgr.Printf("[WARN] %s", err)
			providerErrors = append(providerErrors, fmt.Sprintf("Custom (%s): %v", opts.Custom.Name, err))
		} else if p != nil {
			providers = append(providers, p)
		}
	}

	// check if any providers were successfully initialized
	if len(providers) == 0 {
		return nil, fmt.Errorf("all enabled providers failed to initialize:\n%s", strings.Join(providerErrors, "\n"))
	}

	// if mix mode is enabled, validate the configuration
	if opts.MixEnabled && len(providers) < 2 {
		lgr.Printf("[WARN] mix mode enabled but only one provider is active, mix feature will not be used")
	}

	return providers, nil
}

// anyProvidersEnabled checks if at least one provider is enabled in the options
func anyProvidersEnabled(opts *options) bool {
	return opts.OpenAI.Enabled || opts.Anthropic.Enabled ||
		opts.Google.Enabled || opts.Custom.Enabled
}


// initializeCustomProvider initializes the custom provider
func initializeCustomProvider(opts *options) (provider.Provider, error) {
	// validate required fields
	if opts.Custom.URL == "" {
		return nil, fmt.Errorf("custom provider %s failed to initialize: URL is required", opts.Custom.Name)
	}

	if opts.Custom.Model == "" {
		return nil, fmt.Errorf("custom provider %s failed to initialize: model is required", opts.Custom.Name)
	}

	// create custom provider
	customProvider := provider.NewCustomOpenAI(provider.CustomOptions{
		Name:        opts.Custom.Name,
		BaseURL:     opts.Custom.URL,
		APIKey:      opts.Custom.APIKey,
		Model:       opts.Custom.Model,
		Enabled:     true,
		MaxTokens:   int(opts.Custom.MaxTokens),
		Temperature: opts.Custom.Temperature,
	})

	// check if the provider was enabled successfully
	if customProvider.Enabled() {
		lgr.Printf("[DEBUG] added custom provider: %s, URL: %s, model: %s, temperature: %.2f",
			opts.Custom.Name, opts.Custom.URL, opts.Custom.Model, opts.Custom.Temperature)
		return customProvider, nil
	}
	
	// provider failed to initialize
	return nil, fmt.Errorf("custom provider %s failed to initialize", opts.Custom.Name)
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

	// handle mix mode if enabled and we have multiple results
	if opts.MixEnabled && len(providers) > 1 {
		rawResults := r.GetResults()
		// only process successful results
		var successfulResults []provider.Result
		for _, res := range rawResults {
			if res.Error == nil {
				successfulResults = append(successfulResults, res)
			}
		}

		// if we have more than one successful result, mix them
		if len(successfulResults) > 1 {
			mixedResult, err := mixResults(timeoutCtx, opts, providers, successfulResults)
			if err != nil {
				return fmt.Errorf("failed to mix results: %w", err)
			}
			result = mixedResult
		}
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

// mixResults takes multiple provider results and uses a selected provider to mix them
func mixResults(ctx context.Context, opts *options, providers []provider.Provider, results []provider.Result) (string, error) {
	// find the mix provider
	var mixProvider provider.Provider
	mixProviderName := strings.ToLower(opts.MixProvider)

	// try to find the specified mix provider among the enabled providers
	for _, p := range providers {
		if strings.ToLower(p.Name()) == mixProviderName && p.Enabled() {
			mixProvider = p
			break
		}
	}

	// if the specified mix provider isn't found, fall back to the first enabled provider
	if mixProvider == nil {
		for _, p := range providers {
			if p.Enabled() {
				mixProvider = p
				lgr.Printf("[INFO] specified mix provider '%s' not enabled, falling back to '%s'",
					opts.MixProvider, p.Name())
				break
			}
		}
	}

	if mixProvider == nil {
		return "", fmt.Errorf("no enabled provider found for mixing results")
	}

	// build a prompt with all results
	var mixPrompt strings.Builder
	mixPrompt.WriteString(opts.MixPrompt)
	mixPrompt.WriteString("\n\n")

	for i, result := range results {
		if result.Error != nil {
			continue
		}
		mixPrompt.WriteString(fmt.Sprintf("=== Result %d from %s ===\n", i+1, result.Provider))
		mixPrompt.WriteString(result.Text)
		mixPrompt.WriteString("\n\n")
	}

	// generate the mixed result
	mixedResult, err := mixProvider.Generate(ctx, mixPrompt.String())
	if err != nil {
		return "", fmt.Errorf("failed to generate mixed result using %s: %w", mixProvider.Name(), err)
	}

	return fmt.Sprintf("== mixed results by %s ==\n%s", mixProvider.Name(), mixedResult), nil
}

func outputJSON(finalResult string, results []provider.Result) error {
	// create json output structure
	type ProviderResponse struct {
		Provider string `json:"provider"`
		Text     string `json:"text,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	type JSONOutput struct {
		Responses []ProviderResponse `json:"responses"`       // individual provider responses
		Mixed     string             `json:"mixed,omitempty"` // mixed result if available
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

	// check if the finalResult is a mixed result (has the mixed header)
	if strings.Contains(finalResult, "== mixed results by ") {
		output.Mixed = finalResult
	}

	// encode to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("error encoding JSON output: %w", err)
	}

	return nil
}

// SizeValue is a custom type that supports human-readable size values with k/m suffixes
type SizeValue int64

// UnmarshalFlag implements the flags.Unmarshaler interface for human-readable sizes
func (v *SizeValue) UnmarshalFlag(value string) error {
	value = strings.TrimSpace(strings.ToLower(value))

	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(value, "kb"):
		multiplier = 1024
		value = value[:len(value)-2]
	case strings.HasSuffix(value, "k"):
		multiplier = 1024
		value = value[:len(value)-1]
	case strings.HasSuffix(value, "mb"):
		multiplier = 1024 * 1024
		value = value[:len(value)-2]
	case strings.HasSuffix(value, "m"):
		multiplier = 1024 * 1024
		value = value[:len(value)-1]
	}

	val, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid size value %q: %w", value, err)
	}

	*v = SizeValue(val * multiplier)
	return nil
}
