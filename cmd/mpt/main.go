package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	OpenAI    OpenAIOpts    `group:"openai" namespace:"openai" env-namespace:"OPENAI"`
	Anthropic AnthropicOpts `group:"anthropic" namespace:"anthropic" env-namespace:"ANTHROPIC"`
	Google    GoogleOpts    `group:"google" namespace:"google" env-namespace:"GOOGLE"`

	Prompt  string `short:"p" long:"prompt" description:"prompt text" required:"true"`
	Timeout int    `short:"t" long:"timeout" description:"timeout in seconds" default:"60"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Version bool `short:"V" long:"version" description:"show version info"`
	NoColor bool `long:"no-color" env:"NO_COLOR" description:"disable color output"`
}

// OpenAIOpts defines options for OpenAI provider
type OpenAIOpts struct {
	APIKey  string `long:"api-key" env:"API_KEY" description:"OpenAI API key"`
	Model   string `long:"model" env:"MODEL" description:"OpenAI model" default:"gpt-4-turbo-preview"`
	Enabled bool   `long:"enabled" env:"ENABLED" description:"enable OpenAI provider"`
}

// AnthropicOpts defines options for Anthropic provider
type AnthropicOpts struct {
	APIKey  string `long:"api-key" env:"API_KEY" description:"Anthropic API key"`
	Model   string `long:"model" env:"MODEL" description:"Anthropic model" default:"claude-3-sonnet-20240229"`
	Enabled bool   `long:"enabled" env:"ENABLED" description:"enable Anthropic provider"`
}

// GoogleOpts defines options for Google provider
type GoogleOpts struct {
	APIKey  string `long:"api-key" env:"API_KEY" description:"Google API key"`
	Model   string `long:"model" env:"MODEL" description:"Google model" default:"gemini-1.5-pro"`
	Enabled bool   `long:"enabled" env:"ENABLED" description:"enable Google provider"`
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

	// initialize providers
	openaiProvider := provider.NewOpenAI(provider.Options{
		APIKey:  opts.OpenAI.APIKey,
		Model:   opts.OpenAI.Model,
		Enabled: opts.OpenAI.Enabled,
	})

	anthropicProvider := provider.NewAnthropic(provider.Options{
		APIKey:  opts.Anthropic.APIKey,
		Model:   opts.Anthropic.Model,
		Enabled: opts.Anthropic.Enabled,
	})

	googleProvider := provider.NewGoogle(provider.Options{
		APIKey:  opts.Google.APIKey,
		Model:   opts.Google.Model,
		Enabled: opts.Google.Enabled,
	})

	// create runner with all providers
	r := runner.New(openaiProvider, anthropicProvider, googleProvider)

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
