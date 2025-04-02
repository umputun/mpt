# MPT - Multi-Provider Tool

[![Build Status](https://github.com/umputun/mpt/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/mpt/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/umputun/mpt/badge.svg?branch=master)](https://coveralls.io/github/umputun/mpt?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/umputun/mpt)](https://goreportcard.com/report/github.com/umputun/mpt)

MPT is a command-line utility that sends prompts to multiple AI language model providers in parallel and combines the results.

## Features

- Parallel execution of prompts across multiple LLM providers
- Support for OpenAI, Anthropic (Claude), and Google (Gemini) models
- Easy configuration via command-line flags or environment variables
- Customizable timeout for requests
- Clear results formatting with provider-specific headers

## Installation

```
go get -u github.com/umputun/mpt/cmd/mpt
```

Or download binary from [Releases](https://github.com/umputun/mpt/releases).

## Usage

```
mpt --prompt "Your prompt here" [options]
```

### Provider Configuration

#### OpenAI

```
--openai.api-key      OpenAI API key (or OPENAI_API_KEY env var)
--openai.model        OpenAI model to use (default: gpt-4-turbo-preview)
--openai.enabled      Enable OpenAI provider
```

#### Anthropic (Claude)

```
--anthropic.api-key   Anthropic API key (or ANTHROPIC_API_KEY env var)
--anthropic.model     Anthropic model to use (default: claude-3-sonnet-20240229)
--anthropic.enabled   Enable Anthropic provider
```

#### Google (Gemini)

```
--google.api-key      Google API key (or GOOGLE_API_KEY env var)
--google.model        Google model to use (default: gemini-1.5-pro)
--google.enabled      Enable Google provider
```

### General Options

```
-p, --prompt          Prompt text to send to providers (required)
-t, --timeout         Timeout in seconds (default: 60)
--dbg                 Enable debug mode
-V, --version         Show version information
--no-color            Disable color output
```

### Example

```
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export GOOGLE_API_KEY="your-google-key"

mpt --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Explain the concept of recursion in programming"
```

Output:

```
== generated by OpenAI ==
Recursion is a programming concept where a function calls itself...

== generated by Anthropic ==
Recursion is a technique in programming where a function solves a problem by...

== generated by Google ==
Recursion in programming is when a function calls itself during its execution...
```

## Using Environment Variables

You can use environment variables instead of command-line flags:

```
OPENAI_API_KEY="your-openai-key"
OPENAI_MODEL="gpt-4"
OPENAI_ENABLED=true

ANTHROPIC_API_KEY="your-anthropic-key"
ANTHROPIC_MODEL="claude-3-opus-20240229"
ANTHROPIC_ENABLED=true

GOOGLE_API_KEY="your-google-key"
GOOGLE_MODEL="gemini-1.5-pro"
GOOGLE_ENABLED=true
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.