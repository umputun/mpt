package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

// Opts with all CLI options
type Opts struct {
	OpenAI          OpenAIOpts             `group:"openai" namespace:"openai" env-namespace:"OPENAI"`
	Anthropic       AnthropicOpts          `group:"anthropic" namespace:"anthropic" env-namespace:"ANTHROPIC"`
	Google          GoogleOpts             `group:"google" namespace:"google" env-namespace:"GOOGLE"`
	CustomProviders []CustomOpenAIProvider `group:"custom" namespace:"custom" env-namespace:"CUSTOM"`

	Prompt  string   `short:"p" long:"prompt" description:"prompt text (if not provided, will be read from stdin)"`
	Files   []string `short:"f" long:"file" description:"files or glob patterns to include in the prompt context"`
	Timeout int      `short:"t" long:"timeout" description:"timeout in seconds" default:"60"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Version bool `short:"V" long:"version" description:"show version info"`
	NoColor bool `long:"no-color" env:"NO_COLOR" description:"disable color output"`
}

// OpenAIOpts defines options for OpenAI provider
type OpenAIOpts struct {
	APIKey    string `long:"api-key" env:"API_KEY" description:"OpenAI API key"`
	Model     string `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-4o"`
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable OpenAI provider"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// AnthropicOpts defines options for Anthropic provider
type AnthropicOpts struct {
	APIKey    string `long:"api-key" env:"API_KEY" description:"Anthropic API key"`
	Model     string `long:"model" env:"MODEL" description:"Anthropic model" default:"claude-3-7-sonnet-20250219"`
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable Anthropic provider"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// GoogleOpts defines options for Google provider
type GoogleOpts struct {
	APIKey    string `long:"api-key" env:"API_KEY" description:"Google API key"`
	Model     string `long:"model" env:"MODEL" description:"Google model" default:"gemini-2.5-pro-exp-03-25"`
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"enable Google provider"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"maximum number of tokens to generate" default:"16384"`
}

// CustomOpenAIProvider defines options for custom OpenAI-compatible providers
type CustomOpenAIProvider struct {
	Name      string `long:"name" env:"NAME" description:"Name for the custom provider" required:"true"`
	URL       string `long:"url" env:"URL" description:"Base URL for the custom provider API" required:"true"`
	APIKey    string `long:"api-key" env:"API_KEY" description:"API key for the custom provider (if needed)"`
	Model     string `long:"model" env:"MODEL" description:"Model to use" required:"true"`
	Enabled   bool   `long:"enabled" env:"ENABLED" description:"Enable this custom provider" default:"true"`
	MaxTokens int    `long:"max-tokens" env:"MAX_TOKENS" description:"Maximum number of tokens to generate" default:"16384"`
}

var revision = "unknown"

// osExit is a variable for testing to mock os.Exit
var osExit = os.Exit

func main() {
	if err := run(); err != nil {
		lgr.Printf("[ERROR] %v", err)
		osExit(1)
	}
}

// run executes the main program logic and returns an error if it fails
func run() error {
	var opts Opts
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			osExit(0)
		}
		return err
	}

	if opts.Version {
		fmt.Printf("Version: %s\n", revision)
		osExit(0)
	}

	setupLog(opts.Debug) // set up logging

	// check if data is being piped in, and append it to the prompt if present
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

	// check if we have a prompt after all attempts
	if opts.Prompt == "" {
		return fmt.Errorf("no prompt provided")
	}

	// load files if specified and append to prompt
	if len(opts.Files) > 0 {
		lgr.Printf("[DEBUG] loading files from patterns: %v", opts.Files)
		fileContent, err := loadFiles(opts.Files)
		if err != nil {
			return fmt.Errorf("failed to load files: %w", err)
		}

		if fileContent != "" {
			lgr.Printf("[DEBUG] loaded %d bytes of content from files", len(fileContent))
			opts.Prompt += "\n\n" + fileContent
		}
	}

	// initialize providers
	openaiProvider := provider.NewOpenAI(provider.Options{
		APIKey:    opts.OpenAI.APIKey,
		Model:     opts.OpenAI.Model,
		Enabled:   opts.OpenAI.Enabled,
		MaxTokens: opts.OpenAI.MaxTokens,
	})

	anthropicProvider := provider.NewAnthropic(provider.Options{
		APIKey:    opts.Anthropic.APIKey,
		Model:     opts.Anthropic.Model,
		Enabled:   opts.Anthropic.Enabled,
		MaxTokens: opts.Anthropic.MaxTokens,
	})

	googleProvider := provider.NewGoogle(provider.Options{
		APIKey:    opts.Google.APIKey,
		Model:     opts.Google.Model,
		Enabled:   opts.Google.Enabled,
		MaxTokens: opts.Google.MaxTokens,
	})

	// create a slice to hold all providers
	providers := []provider.Provider{openaiProvider, anthropicProvider, googleProvider}

	// add custom providers
	for _, customOpt := range opts.CustomProviders {
		if customOpt.Enabled {
			customProvider := provider.NewCustomOpenAI(provider.CustomOptions{
				Name:      customOpt.Name,
				BaseURL:   customOpt.URL,
				APIKey:    customOpt.APIKey,
				Model:     customOpt.Model,
				Enabled:   customOpt.Enabled,
				MaxTokens: customOpt.MaxTokens,
			})
			providers = append(providers, customProvider)
			lgr.Printf("[INFO] added custom provider: %s, URL: %s, model: %s",
				customOpt.Name, customOpt.URL, customOpt.Model)
		}
	}

	// create runner with all providers
	r := runner.New(providers...)

	// create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	// run the prompt
	result, err := r.Run(ctx, opts.Prompt)
	if err != nil {
		return fmt.Errorf("failed to run prompt: %w", err)
	}

	// print the result
	fmt.Println(strings.TrimSpace(result))
	return nil
}

// loadFiles loads content from files matching the given patterns and returns a formatted string
// with file names as comments and their contents. Supports recursive directory traversal.
func loadFiles(patterns []string) (string, error) {
	if len(patterns) == 0 {
		return "", nil
	}

	// map to store all matched file paths
	matchedFiles := make(map[string]struct{})

	// expand all patterns and collect unique file paths
	for _, pattern := range patterns {
		// check for Go-style recursive pattern: dir/...
		if strings.Contains(pattern, "/...") {
			basePath, filter := parseRecursivePattern(pattern)

			// check if base directory exists
			info, err := os.Stat(basePath)
			if err != nil || !info.IsDir() {
				lgr.Printf("[WARN] invalid base directory for pattern %s: %v", pattern, err)
				continue
			}

			// walk the directory tree filtering by the specified pattern
			matchCount := 0
			err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil // skip files that can't be accessed
				}

				if !info.IsDir() {
					// if filter is specified, check if file matches
					if filter == "" {
						// no filter, include all files
						matchedFiles[path] = struct{}{}
						matchCount++
					} else if strings.HasPrefix(filter, "*.") {
						// extension filter (*.go, *.js, etc.)
						ext := filter[1:] // remove *
						if strings.HasSuffix(path, ext) {
							matchedFiles[path] = struct{}{}
							matchCount++
						}
					} else if matched, _ := filepath.Match(filter, filepath.Base(path)); matched {
						// standard glob pattern
						matchedFiles[path] = struct{}{}
						matchCount++
					}
				}
				return nil
			})

			if err != nil {
				lgr.Printf("[WARN] failed to walk directory for pattern %s: %v", pattern, err)
			}

			if matchCount == 0 {
				lgr.Printf("[WARN] no files matched pattern: %s", pattern)
			} else {
				lgr.Printf("[DEBUG] matched %d files for pattern: %s", matchCount, pattern)
			}

			continue
		}

		// standard glob pattern
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("failed to glob pattern %s: %w", pattern, err)
		}

		if len(matches) == 0 {
			lgr.Printf("[WARN] no files matched pattern: %s", pattern)
			continue
		}

		matchCount := 0
		for _, match := range matches {
			// check if it's a file
			info, err := os.Stat(match)
			if err != nil {
				return "", fmt.Errorf("failed to stat file %s: %w", match, err)
			}

			if !info.IsDir() {
				matchedFiles[match] = struct{}{}
				matchCount++
			} else {
				// if it's a directory, walk it recursively
				dirMatchCount := 0
				err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil // skip files that can't be accessed
					}

					if !info.IsDir() {
						matchedFiles[path] = struct{}{}
						dirMatchCount++
					}
					return nil
				})

				if err != nil {
					lgr.Printf("[WARN] failed to walk directory %s: %v", match, err)
				}

				matchCount += dirMatchCount
			}
		}

		if matchCount == 0 {
			lgr.Printf("[WARN] no files matched after directory traversal: %s", pattern)
		} else {
			lgr.Printf("[DEBUG] matched %d files for pattern: %s", matchCount, pattern)
		}
	}

	// convert to slice and sort for consistent ordering
	sortedFiles := make([]string, 0, len(matchedFiles))
	for file := range matchedFiles {
		sortedFiles = append(sortedFiles, file)
	}
	sort.Strings(sortedFiles)

	if len(sortedFiles) == 0 {
		return "", fmt.Errorf("no files matched the provided patterns")
	}

	// build the formatted content
	var sb strings.Builder
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	for _, file := range sortedFiles {
		content, err := os.ReadFile(file) // #nosec G304 - file paths are validated earlier
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", file, err)
		}

		// get relative path if possible, otherwise use absolute
		relPath, err := filepath.Rel(cwd, file)
		if err != nil {
			relPath = file
		}

		// determine the appropriate comment style based on file extension
		fileHeader := getFileHeader(relPath)

		sb.WriteString(fileHeader)
		sb.Write(content)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}

// parseRecursivePattern parses a Go-style recursive pattern like "pkg/..." or "cmd/.../*.go"
// returns basePath and filter (file extension or pattern to match)
func parseRecursivePattern(pattern string) (basePath, filter string) {
	// split at /...
	parts := strings.SplitN(pattern, "/...", 2)
	basePath = parts[0]
	filter = ""

	// check if there's a filter after /...
	if len(parts) > 1 && parts[1] != "" {
		// pattern like pkg/.../*.go
		if strings.HasPrefix(parts[1], "/") {
			filter = parts[1][1:] // remove leading slash
		} else {
			filter = parts[1]
		}
	}

	return basePath, filter
}

// getFileHeader returns an appropriate comment header for a file based on its extension
func getFileHeader(filePath string) string {
	ext := filepath.Ext(filePath)

	// define comment styles for different file types
	switch ext {
	// hash-style comments (#)
	case ".py", ".rb", ".pl", ".pm", ".sh", ".bash", ".zsh", ".fish", ".tcl", ".r",
		".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg", ".properties", ".mk", ".makefile":
		return fmt.Sprintf("# file: %s\n", filePath)

	// Double-slash comments (//)
	case ".js", ".ts", ".jsx", ".tsx", ".java", ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp",
		".hxx", ".cs", ".php", ".go", ".swift", ".kt", ".rs", ".scala", ".dart", ".groovy", ".d":
		return fmt.Sprintf("// file: %s\n", filePath)

	// HTML/XML style comments
	case ".html", ".xml", ".svg", ".xaml", ".jsp", ".asp", ".aspx", ".jsf", ".vue":
		return fmt.Sprintf("<!-- file: %s -->\n", filePath)

	// CSS style comments
	case ".css", ".scss", ".sass", ".less":
		return fmt.Sprintf("/* file: %s */\n", filePath)

	// SQL comments
	case ".sql":
		return fmt.Sprintf("-- file: %s\n", filePath)

	// lisp/Clojure comments
	case ".lisp", ".cl", ".el", ".clj", ".cljs", ".cljc":
		return fmt.Sprintf(";; file: %s\n", filePath)

	// haskell/VHDL comments
	case ".hs", ".lhs", ".vhdl", ".vhd":
		return fmt.Sprintf("-- file: %s\n", filePath)

	// PowerShell comments
	case ".ps1", ".psm1", ".psd1":
		return fmt.Sprintf("# file: %s\n", filePath)

	// batch file comments
	case ".bat", ".cmd":
		return fmt.Sprintf(":: file: %s\n", filePath)

	// fortran comments
	case ".f", ".f90", ".f95", ".f03":
		return fmt.Sprintf("! file: %s\n", filePath)

	// Default to // for unknown types
	default:
		return fmt.Sprintf("// file: %s\n", filePath)
	}
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
