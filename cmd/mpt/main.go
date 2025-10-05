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

	"github.com/umputun/mpt/pkg/config"
	"github.com/umputun/mpt/pkg/mcp"
	"github.com/umputun/mpt/pkg/mix"
	"github.com/umputun/mpt/pkg/prompt"
	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

// options with all CLI options
type options struct {
	OpenAI    openAIOpts    `group:"openai" namespace:"openai" env-namespace:"OPENAI"`
	Anthropic anthropicOpts `group:"anthropic" namespace:"anthropic" env-namespace:"ANTHROPIC"`
	Google    googleOpts    `group:"google" namespace:"google" env-namespace:"GOOGLE"`

	Custom customOpenAIProvider `group:"custom" namespace:"custom" env-namespace:"CUSTOM"`

	// new map for multiple custom providers
	Customs map[string]customSpec `long:"customs" description:"Add custom OpenAI-compatible provider as 'id:key=value[,key=value,...]' (e.g., openrouter:url=https://openrouter.ai/api/v1,model=claude-3.5)" key-value-delimiter:":" value-name:"ID:SPEC"`

	MCP   mcpOpts   `group:"mcp" namespace:"mcp" env-namespace:"MCP"`
	Git   gitOpts   `group:"git" namespace:"git" env-namespace:"GIT"`
	Retry retryOpts `group:"retry" namespace:"retry" env-namespace:"RETRY"`

	Prompt      string        `short:"p" long:"prompt" description:"prompt text (if not provided, will be read from stdin)"`
	Files       []string      `short:"f" long:"file" description:"files or glob patterns to include in the prompt context"`
	Excludes    []string      `short:"x" long:"exclude" description:"patterns to exclude from file matching (e.g., 'vendor/**', '**/mocks/*')"`
	Timeout     time.Duration `short:"t" long:"timeout" default:"60s" description:"timeout duration"`
	MaxFileSize SizeValue     `long:"max-file-size" env:"MAX_FILE_SIZE" default:"65536" description:"maximum size of individual files to process in bytes (default: 64KB, supports k/kb/m/mb/g/gb suffixes)"`
	Force       bool          `long:"force" description:"force loading files by skipping all exclusion patterns (including .gitignore and common patterns)"`

	// mix options
	MixEnabled  bool   `long:"mix" env:"MIX" description:"enable mix (merge) results from all providers"`
	MixProvider string `long:"mix.provider" env:"MIX_PROVIDER" default:"openai" description:"provider used to mix results"`
	MixPrompt   string `long:"mix.prompt" env:"MIX_PROMPT" default:"merge results from all providers" description:"prompt used to mix results"`

	// consensus options - works with mix mode
	ConsensusEnabled  bool `long:"consensus" env:"CONSENSUS" description:"enable consensus checking when using mix"`
	ConsensusAttempts int  `long:"consensus.attempts" env:"CONSENSUS_ATTEMPTS" default:"1" description:"max consensus attempts (1-5)"`

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
	Model       string    `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-5"`
	MaxTokens   SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate (default: 16384, supports k/kb/m/mb/g/gb suffixes)" default:"16384"`
	Temperature float32   `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-2, higher is more random)" default:"0.1"`
}

// anthropicOpts defines options for Anthropic provider
type anthropicOpts struct {
	Enabled   bool      `long:"enabled" env:"ENABLED" description:"enable Anthropic provider"`
	APIKey    string    `long:"api-key" env:"API_KEY" description:"Anthropic API key"`
	Model     string    `long:"model" env:"MODEL" description:"Anthropic model" default:"claude-sonnet-4-5"`
	MaxTokens SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate (default: 16384, supports k/m suffixes)" default:"16384"`
}

// googleOpts defines options for Google provider
type googleOpts struct {
	Enabled   bool      `long:"enabled" env:"ENABLED" description:"enable Google provider"`
	APIKey    string    `long:"api-key" env:"API_KEY" description:"Google API key"`
	Model     string    `long:"model" env:"MODEL" description:"Google model" default:"gemini-2.5-pro-preview-06-05"`
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
	MaxTokens   SizeValue `long:"max-tokens" env:"MAX_TOKENS" description:"Maximum number of tokens to generate (default: 16384, supports k/kb/m/mb/g/gb suffixes)" default:"16384"`
	Temperature float32   `long:"temperature" env:"TEMPERATURE" description:"controls randomness (0-2, higher is more random)" default:"0.7"`
}

// gitOpts defines options for Git integration
type gitOpts struct {
	Diff   bool   `long:"diff" env:"DIFF" description:"include git diff as context (uncommitted changes)"`
	Branch string `long:"branch" env:"BRANCH" description:"include git diff between given branch and master/main (for PR review)"`
}

// retryOpts defines options for retry behavior
type retryOpts struct {
	Attempts int           `long:"attempts" env:"ATTEMPTS" default:"1" description:"max attempts (1=no retry, 3=up to 2 retries)"`
	Delay    time.Duration `long:"delay" env:"DELAY" default:"1s" description:"base delay between retries"`
	MaxDelay time.Duration `long:"max-delay" env:"MAX_DELAY" default:"30s" description:"max delay between retries"`
	Factor   float64       `long:"factor" env:"FACTOR" default:"2" description:"backoff multiplier"`
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

// validateOptions validates the command-line options
func validateOptions(opts *options) error {
	// validate consensus options
	if opts.ConsensusEnabled {
		if opts.ConsensusAttempts < 1 || opts.ConsensusAttempts > 5 {
			return fmt.Errorf("consensus attempts must be between 1 and 5, got %d", opts.ConsensusAttempts)
		}
		// consensus requires mix mode
		if !opts.MixEnabled {
			return fmt.Errorf("consensus mode requires mix mode to be enabled (use --mix)")
		}
	}
	return nil
}

// run executes the main program logic and returns an error if it fails
func run(ctx context.Context, opts *options) error {
	// validate options first
	if err := validateOptions(opts); err != nil {
		return err
	}
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

	result, err := executePrompt(ctx, opts, providers)
	if err != nil {
		return err
	}

	// output results
	if opts.JSON {
		return outputJSON(result)
	}
	fmt.Println(strings.TrimSpace(result.Text))
	return nil
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
	secretsMap := make(map[string]bool) // use map to avoid duplicates

	// add API keys from built-in providers
	if opts.OpenAI.APIKey != "" {
		secretsMap[opts.OpenAI.APIKey] = true
	}
	if opts.Anthropic.APIKey != "" {
		secretsMap[opts.Anthropic.APIKey] = true
	}
	if opts.Google.APIKey != "" {
		secretsMap[opts.Google.APIKey] = true
	}

	// add API keys from custom providers
	customSecrets := createCustomManager(opts).CollectSecrets()
	for _, secret := range customSecrets {
		if secret != "" {
			secretsMap[secret] = true
		}
	}

	// convert map to slice
	secrets := make([]string, 0, len(secretsMap))
	for secret := range secretsMap {
		secrets = append(secrets, secret)
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
	// only create git diff processor if git features are requested
	var gitDiffer prompt.GitDiffProcessor
	if opts.Git.Diff || opts.Git.Branch != "" {
		gitDiffer = prompt.NewGitDiffer()
	}

	// use the prompt builder to handle file loading and prompt construction
	builder := prompt.New(opts.Prompt, gitDiffer).
		WithFiles(opts.Files).
		WithExcludes(opts.Excludes).
		WithMaxFileSize(int64(opts.MaxFileSize)).
		WithForce(opts.Force)

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

	opts.Prompt = fullPrompt
	return nil
}

// providerConfig holds configuration for a provider
type providerConfig struct {
	enabled   bool
	provType  provider.ProviderType
	name      string
	apiKey    string
	model     string
	maxTokens int
	temp      float32
}

// initializeProviders creates provider instances from the options
func initializeProviders(opts *options) ([]provider.Provider, error) {
	// check if any providers are enabled
	if !anyProvidersEnabled(opts) {
		return nil, fmt.Errorf("no providers enabled. Use --<provider>.enabled flag to enable at least one provider (e.g., --openai.enabled)")
	}

	providers := make([]provider.Provider, 0, 4) // pre-allocate for 4 providers (3 standard + 1 custom)
	providerErrors := make([]string, 0)

	// initialize standard providers
	standardProviders := getStandardProviderConfigs(opts)
	for _, config := range standardProviders {
		if !config.enabled {
			continue
		}

		p, err := provider.CreateProvider(config.provType, provider.Options{
			APIKey:      config.apiKey,
			Model:       config.model,
			Enabled:     true,
			MaxTokens:   config.maxTokens,
			Temperature: config.temp,
		})
		if err != nil {
			lgr.Printf("[WARN] %s provider failed to initialize: %v", config.name, err)
			providerErrors = append(providerErrors, fmt.Sprintf("%s: %v", config.name, err))
			continue
		}

		providers = append(providers, p)
		lgr.Printf("[DEBUG] added %s provider, model: %s", config.name, config.model)
	}

	// initialize multiple custom providers (handles legacy custom too)
	customProviders, customErrors := createCustomManager(opts).InitializeProviders()
	providers = append(providers, customProviders...)
	providerErrors = append(providerErrors, customErrors...)

	// check if any providers were successfully initialized
	if len(providers) == 0 {
		return nil, fmt.Errorf("all enabled providers failed to initialize:\n%s", strings.Join(providerErrors, "\n"))
	}

	// wrap providers with retry logic if configured
	if opts.Retry.Attempts > 1 {
		retryOpts := provider.RetryOptions{
			Attempts: opts.Retry.Attempts,
			Delay:    opts.Retry.Delay,
			MaxDelay: opts.Retry.MaxDelay,
			Factor:   opts.Retry.Factor,
		}
		providers = provider.WrapProvidersWithRetry(providers, retryOpts)
		lgr.Printf("[INFO] wrapped %d providers with retry logic (attempts=%d)", len(providers), opts.Retry.Attempts)
	}

	// if mix mode is enabled, validate the configuration
	if opts.MixEnabled && len(providers) < 2 {
		lgr.Printf("[WARN] mix mode enabled but only one provider is active, mix feature will not be used")
	}

	return providers, nil
}

// getStandardProviderConfigs returns configurations for all standard providers
func getStandardProviderConfigs(opts *options) []providerConfig {
	return []providerConfig{
		{
			enabled:   opts.OpenAI.Enabled,
			provType:  provider.ProviderTypeOpenAI,
			name:      "OpenAI",
			apiKey:    opts.OpenAI.APIKey,
			model:     opts.OpenAI.Model,
			maxTokens: int(opts.OpenAI.MaxTokens),
			temp:      opts.OpenAI.Temperature,
		},
		{
			enabled:   opts.Anthropic.Enabled,
			provType:  provider.ProviderTypeAnthropic,
			name:      "Anthropic",
			apiKey:    opts.Anthropic.APIKey,
			model:     opts.Anthropic.Model,
			maxTokens: int(opts.Anthropic.MaxTokens),
			temp:      0, // anthropic doesn't use temperature parameter
		},
		{
			enabled:   opts.Google.Enabled,
			provType:  provider.ProviderTypeGoogle,
			name:      "Google",
			apiKey:    opts.Google.APIKey,
			model:     opts.Google.Model,
			maxTokens: int(opts.Google.MaxTokens),
			temp:      0, // google doesn't use temperature parameter
		},
	}
}

// anyProvidersEnabled checks if at least one provider is enabled in the options
func anyProvidersEnabled(opts *options) bool {
	// check standard providers
	if opts.OpenAI.Enabled || opts.Anthropic.Enabled || opts.Google.Enabled {
		return true
	}

	// check if any custom providers are enabled
	return createCustomManager(opts).AnyEnabled()
}

// ExecutionResult holds the structured result of executing a prompt
type ExecutionResult struct {
	Text        string            // final text output (with headers for CLI display)
	MixedText   string            // raw mixed text without headers (for JSON)
	MixUsed     bool              // whether mix mode was used
	MixProvider string            // provider that performed the mixing (if any)
	Results     []provider.Result // individual provider results
	// consensus fields
	ConsensusAttempted bool // whether consensus was attempted
	ConsensusAchieved  bool // whether consensus was achieved
	ConsensusAttempts  int  // number of consensus attempts made
}

// executePrompt runs the prompt against the configured providers
func executePrompt(ctx context.Context, opts *options, providers []provider.Provider) (*ExecutionResult, error) {
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
			return nil, fmt.Errorf("operation timed out after %s, try increasing the timeout with -t flag", opts.Timeout)
		}
		return nil, err
	}

	// prepare execution result
	execResult := &ExecutionResult{
		Text:    result,
		Results: r.GetResults(),
	}

	// handle mix mode if enabled
	if opts.MixEnabled && len(providers) > 1 {
		mixRequest := mix.Request{
			Prompt:            opts.Prompt,
			MixPrompt:         opts.MixPrompt,
			MixProvider:       opts.MixProvider,
			ConsensusEnabled:  opts.ConsensusEnabled,
			ConsensusAttempts: opts.ConsensusAttempts,
			Providers:         providers,
			Results:           r.GetResults(),
		}

		mixResult, err := processMixMode(timeoutCtx, mixRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to mix results: %w", err)
		}
		if mixResult.TextWithHeader != "" {
			execResult.Text = mixResult.TextWithHeader
			execResult.MixedText = mixResult.RawText
			execResult.MixUsed = true
			execResult.MixProvider = mixResult.MixProvider
		}
		// set consensus metadata
		if opts.ConsensusEnabled {
			execResult.ConsensusAttempted = true
			execResult.ConsensusAchieved = mixResult.ConsensusAchieved
			execResult.ConsensusAttempts = mixResult.ConsensusAttempts
		}
	}

	return execResult, nil
}

// processMixMode handles mixing results from multiple providers
func processMixMode(ctx context.Context, req mix.Request) (*mix.Response, error) {
	// create mix manager
	mixer := mix.New(lgr.Default())

	// process the mix request
	return mixer.Process(ctx, req)
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
	stat, err := os.Stdin.Stat()
	if err != nil {
		// if we can't stat stdin, assume it's not piped
		stat = nil
	}
	isPiped := stat != nil && (stat.Mode()&os.ModeCharDevice) == 0

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
		logOpts = []lgr.Option{lgr.Debug, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError, lgr.Out(os.Stderr)}
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

func outputJSON(result *ExecutionResult) error {
	// create json output structure
	type ProviderResponse struct {
		Provider string `json:"provider"`
		Text     string `json:"text,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	type JSONOutput struct {
		Final              string             `json:"final"`                         // final text shown in cli mode
		Responses          []ProviderResponse `json:"responses"`                     // individual provider responses
		Mixed              string             `json:"mixed,omitempty"`               // raw mixed result without headers
		MixUsed            bool               `json:"mix_used"`                      // explicit flag for mix mode usage
		MixProvider        string             `json:"mix_provider,omitempty"`        // provider that performed mixing
		ConsensusAttempted bool               `json:"consensus_attempted,omitempty"` // whether consensus was attempted
		ConsensusAchieved  bool               `json:"consensus_achieved,omitempty"`  // whether consensus was achieved
		ConsensusAttempts  int                `json:"consensus_attempts,omitempty"`  // number of consensus attempts made
		Timestamp          string             `json:"timestamp"`
	}

	// build responses array
	responses := make([]ProviderResponse, 0, len(result.Results))
	for _, r := range result.Results {
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
		Final:              result.Text,
		Responses:          responses,
		MixUsed:            result.MixUsed,
		ConsensusAttempted: result.ConsensusAttempted,
		ConsensusAchieved:  result.ConsensusAchieved,
		ConsensusAttempts:  result.ConsensusAttempts,
		Timestamp:          time.Now().Format(time.RFC3339),
	}

	// add mixed result info if mixing was used
	if result.MixUsed {
		output.Mixed = result.MixedText // use raw text without headers
		output.MixProvider = result.MixProvider
	}

	// encode to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("error encoding JSON output: %w", err)
	}

	return nil
}

// SizeValue is a custom type that supports human-readable size values with k/kb/m/mb/g/gb suffixes
type SizeValue int64

// UnmarshalFlag implements the flags.Unmarshaler interface for human-readable sizes
func (v *SizeValue) UnmarshalFlag(value string) error {
	size, err := config.ParseSize(value)
	if err != nil {
		return fmt.Errorf("invalid size value %q: %w", value, err)
	}
	*v = SizeValue(size)
	return nil
}

// customSpec is a CLI wrapper around config.CustomSpec that implements UnmarshalFlag for go-flags
type customSpec struct {
	config.CustomSpec
}

// UnmarshalFlag parses "url=https://...,model=xxx,api-key=xxx" format for go-flags
func (c *customSpec) UnmarshalFlag(value string) error {
	spec, err := config.ParseCustomSpec(value)
	if err != nil {
		return err
	}
	c.CustomSpec = spec
	return nil
}

// createCustomManager creates a CustomProviderManager from CLI options
func createCustomManager(opts *options) *config.CustomProviderManager {
	// convert CLI customSpec to config.CustomSpec
	configCustoms := make(map[string]config.CustomSpec)
	for id, spec := range opts.Customs {
		configCustoms[id] = spec.CustomSpec
	}

	// convert legacy custom to config.CustomSpec pointer if enabled
	var legacyCustom *config.CustomSpec
	if opts.Custom.Enabled {
		legacyCustom = &config.CustomSpec{
			Name:        opts.Custom.Name,
			URL:         opts.Custom.URL,
			APIKey:      opts.Custom.APIKey,
			Model:       opts.Custom.Model,
			MaxTokens:   int(opts.Custom.MaxTokens),
			Temperature: opts.Custom.Temperature,
			Enabled:     opts.Custom.Enabled,
		}
	}

	return config.NewCustomProviderManager(configCustoms, legacyCustom)
}
