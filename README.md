# MPT - Multi-Provider Tool for AI Language Models

[![Build Status](https://github.com/umputun/mpt/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/mpt/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/umputun/mpt/badge.svg?branch=master)](https://coveralls.io/github/umputun/mpt?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/umputun/mpt)](https://goreportcard.com/report/github.com/umputun/mpt)

MPT is a command-line utility that sends prompts to multiple AI language model providers (OpenAI, Anthropic, Google, and custom providers) in parallel and combines the results. It enables easy file inclusion for context and supports flexible pattern matching to quickly include relevant code or documentation in your prompts.

## What MPT Does

MPT makes working with AI language models simpler and more powerful by:

1. **Querying multiple AI providers simultaneously**: Get responses from OpenAI, Claude, Gemini, and custom models all at once, with clear provider labeling.

2. **Including files as context using smart pattern matching**: 
   - Easily add code files to your prompt with flexible patterns: `--file "**/*.go"` or `--file "pkg/..."`
   - Filter out unwanted files with exclusion patterns: `--exclude "**/tests/**"`
   - Provide comprehensive context from your codebase without manual copying

3. **Streamlining your AI workflow**: 
   - Pipe content from other commands directly to MPT: `git diff | mpt --prompt "Review this code"`
   - Combine stdin with files: `cat error.log | mpt --file "app/server.go" --prompt "Why am I seeing this error?"`
   - Use environment variables to manage API keys

4. **Getting multiple perspectives**: Different AI models have different strengths—MPT lets you leverage them all at once to get more comprehensive insights.

5. **Providing cleaner single-provider output**: When using just one AI provider, MPT automatically removes headers for cleaner results.

For example, when reviewing code changes:
```
git diff HEAD~1 | mpt --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this code and identify potential bugs or security issues"
```

This will show you different perspectives on the same code, potentially catching issues that a single model might overlook.

You can also create cascading workflows, where multiple models analyze content first, and then another model synthesizes their insights:
```
# First gather multiple perspectives
git diff HEAD~1 | mpt --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this code thoroughly" > reviews.txt

# Then have a single model summarize the key points
cat reviews.txt | mpt --anthropic.enabled \
    --prompt "Synthesize these reviews into a concise summary of the key issues and improvements"
```

The second command will produce clean output without any provider headers, since only one provider is enabled.

## Key Features

- **Multi-Provider Support**: Run prompts in parallel across OpenAI, Anthropic (Claude), Google (Gemini), and custom LLMs
- **File Context Inclusion**: Easily add files, directories, or patterns to provide context for your prompts
- **Smart Pattern Matching**: Include files using standard glob patterns, directory paths, bash-style wildcards (`**/*.go`), or Go-style patterns (`pkg/...`)
- **Exclusion Filtering**: Filter out unwanted files with the same pattern matching syntax (`--exclude "**/tests/**"`)
- **Stdin Integration**: Pipe content directly from other tools (like `git diff`) for AI analysis
- **Customizable Execution**: Configure timeouts, token limits, and models per provider
- **Clean Output Formatting**: Provider-specific headers (or none when using a single provider)
- **Environment Variable Support**: Store API keys and settings in environment variables instead of flags

## Installation

```
go get -u github.com/umputun/mpt/cmd/mpt
```

Or download binary from [Releases](https://github.com/umputun/mpt/releases).

## Usage

```
mpt [options]
```

You can provide a prompt in the following ways:
1. Using the `--prompt` flag: `mpt --prompt "Your question here"`
2. Piping content: `echo "Your question" | mpt`
3. Combining flag and piped content: `echo "Additional context" | mpt --prompt "Main question"`
   - This combines both inputs with a newline separator
4. Interactive mode: If no prompt is provided via command line or pipe, you'll be prompted to enter one

### Provider Configuration

#### OpenAI

```
--openai.api-key      OpenAI API key (or OPENAI_API_KEY env var)
--openai.model        OpenAI model to use (default: gpt-4o)
--openai.enabled      Enable OpenAI provider
--openai.max-tokens   Maximum number of tokens to generate (default: 16384)
```

#### Anthropic (Claude)

```
--anthropic.api-key   Anthropic API key (or ANTHROPIC_API_KEY env var)
--anthropic.model     Anthropic model to use (default: claude-3-7-sonnet-20250219)
--anthropic.enabled   Enable Anthropic provider
--anthropic.max-tokens Maximum number of tokens to generate (default: 16384)
```

#### Google (Gemini)

```
--google.api-key      Google API key (or GOOGLE_API_KEY env var)
--google.model        Google model to use (default: gemini-2.5-pro-exp-03-25)
--google.enabled      Enable Google provider
--google.max-tokens   Maximum number of tokens to generate (default: 16384)
```

#### Custom OpenAI-Compatible Providers

You can add multiple custom providers that implement the OpenAI-compatible API. Use a unique identifier for each provider:

```
--custom.<provider-id>.name         Name for the custom provider (required)
--custom.<provider-id>.url          Base URL for the custom provider API (required)
--custom.<provider-id>.api-key      API key for the custom provider (if needed)
--custom.<provider-id>.model        Model to use (required)
--custom.<provider-id>.enabled      Enable this custom provider (default: true)
--custom.<provider-id>.max-tokens   Maximum number of tokens to generate (default: 16384)
```

Example for adding a single local LLM server:

```
mpt --custom.localai.name "LocalLLM" --custom.localai.url "http://localhost:1234/v1" \
    --custom.localai.model "mixtral-8x7b" --prompt "Explain quantum computing"
```

Example with multiple custom providers:

```
mpt --custom.localai.name "LocalLLM" --custom.localai.url "http://localhost:1234/v1" \
    --custom.localai.model "mixtral-8x7b" \
    --custom.together.name "Together" --custom.together.url "https://api.together.xyz/v1" \
    --custom.together.api-key "your-key" --custom.together.model "llama-3-70b" \
    --prompt "Compare the approaches to implementing recursion in different programming languages"
```

### General Options

```
-p, --prompt          Prompt text to send to providers (required)
-f, --file            Files or glob patterns to include in the prompt context (can be used multiple times)
                      Supports:
                      - Standard glob patterns like "*.go" or "cmd/*.js"
                      - Directories (traversed recursively)
                      - Bash-style recursive patterns like "**/*.go" or "pkg/**/*.js"
                      - Go-style recursive patterns like "pkg/..." or "cmd/.../*.go"
-x, --exclude         Patterns to exclude from file matching (can be used multiple times)
                      Uses the same pattern syntax as --file
-t, --timeout         Timeout duration (e.g., 60s, 2m) (default: 60s)
-v, --verbose         Verbose output, shows the complete prompt sent to models
--dbg                 Enable debug mode
-V, --version         Show version information
--no-color            Disable color output
```

### Examples

Basic usage with prompt flag:
```
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export GOOGLE_API_KEY="your-google-key"

mpt --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Explain the concept of recursion in programming"
```

Combining prompt flag with piped input:
```
git diff HEAD~1 | mpt --openai.enabled --prompt "Analyze this git diff and suggest improvements"
```

Including files in the prompt context:
```
mpt --anthropic.enabled --prompt "Explain this code" --file "*.go" --file "*.md"
```

Including entire directories recursively:
```
mpt --openai.enabled --prompt "Explain the architecture of this project" --file "cmd/" --file "pkg/"
```

Using Go-style recursive patterns:
```
mpt --anthropic.enabled --prompt "Find bugs in my Go code" --file "pkg/..." --file "cmd/.../*.go"
```

### File Pattern and Filtering Reference

MPT provides powerful file inclusion and exclusion capabilities to provide contextual information to AI models. You can easily include all the necessary files for your prompt while filtering out unwanted content.

#### Including Files with `--file`

Add relevant files to your prompt context using various pattern types:

1. **Specific Files**
   ```
   --file "README.md"              # Include a specific file
   --file "Makefile"               # Include another specific file
   ```

2. **Standard Glob Patterns**
   ```
   --file "*.go"                   # All Go files in current directory
   --file "cmd/*.go"               # All Go files in the cmd directory
   --file "pkg/*_test.go"          # All test files in the pkg directory
   ```

3. **Directories (Recursive)**
   ```
   --file "cmd/"                   # All files in cmd/ directory and subdirectories
   --file "pkg/api/"               # All files in pkg/api/ directory and subdirectories
   ```

4. **Bash-style Recursive Patterns**
   ```
   --file "**/*.go"                # All Go files in any directory recursively
   --file "pkg/**/*.js"            # All JavaScript files in pkg/ recursively
   --file "**/*_test.go"           # All test files in any directory recursively
   ```

5. **Go-style Recursive Patterns**
   ```
   --file "pkg/..."                # All files in pkg/ directory and subdirectories
   --file "./..."                  # All files in current directory and subdirectories
   --file "cmd/.../*.go"           # All Go files in cmd/ directory and subdirectories
   --file "pkg/.../*_test.go"      # All test files in pkg/ directory and subdirectories
   ```

#### Excluding Files with `--exclude`

Filter out unwanted files using the **same pattern syntax** as `--file`:

```
# Exclude all test files
--exclude "**/*_test.go"

# Exclude all mock files
--exclude "**/mocks/**"

# Exclude vendor directory
--exclude "vendor/**"

# Exclude generated files
--exclude "**/*.gen.go"
```

#### Common Pattern Examples

```bash
# Basic: Include Go files, exclude tests
mpt --anthropic.enabled --prompt "Explain this code" \
    --file "**/*.go" --exclude "**/*_test.go"

# Include code files from specific package, exclude mocks
mpt --openai.enabled --prompt "Document this API" \
    --file "pkg/api/..." --exclude "**/mocks/**"

# Include all code but exclude tests and generated files
mpt --google.enabled --prompt "Review code quality" \
    --file "**/*.go" --exclude "**/*_test.go" --exclude "**/*.gen.go"

# Include only model and controller files
mpt --anthropic.enabled --prompt "Explain architecture" \
    --file "**/*model.go" --file "**/*controller.go"
```

> **Tip:** You can use either bash-style patterns with `**` or Go-style patterns with `/...` for recursive matching—choose whichever syntax you prefer. The exclusion patterns use the same syntax as inclusion patterns.

### File Content Formatting

When files are included in the prompt, they are formatted with appropriate language-specific comment markers to identify each file:

```
// file: cmd/mpt/main.go
package main

import (
    "fmt"
)

# file: README.md
# MPT - Multi-Provider Tool

<!-- file: webpage.html -->
<html>
<body>
<h1>Hello World</h1>
</body>
</html>
```

This makes it easier for the LLM to understand where one file ends and another begins, as well as to identify the file types.

Complex example with files and piped input:
```
find . -name "*.go" -exec grep -l "TODO" {} \; | mpt --openai.enabled \
    --prompt "Find TODOs in my codebase and prioritize them" \
    --file "README.md" --file "CONTRIBUTING.md"
```

### Why Combine Inputs?

The ability to combine the prompt flag with piped stdin content is particularly powerful for workflows where you want to:

1. **Analyze code or text with specific instructions**: Pipe in code, logs, or data while specifying exactly what you want the AI to do with it.

2. **Code reviews**: Request detailed feedback on code changes by piping in the diff while using the prompt flag to specify what aspects to focus on:
   ```
   git diff HEAD~1 | mpt --openai.enabled --prompt "Review this code change and focus on potential performance issues"
   ```

3. **Contextual analysis**: Provide both the content and the specific analysis instructions:
   ```
   cat error_log.txt | mpt --anthropic.enabled --prompt "Identify the root cause of these errors and suggest solutions"
   ```

4. **File transformation**: Transform content according to specific rules:
   ```
   cat document.md | mpt --google.enabled --prompt "Reformat this markdown document to be more readable and fix any syntax issues"
   ```

This approach gives you much more flexibility than either using just stdin or just the prompt flag alone.

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
OPENAI_MODEL="gpt-4o"
OPENAI_ENABLED=true
OPENAI_MAX_TOKENS=16384

ANTHROPIC_API_KEY="your-anthropic-key"
ANTHROPIC_MODEL="claude-3-7-sonnet-20250219"
ANTHROPIC_ENABLED=true
ANTHROPIC_MAX_TOKENS=16384

GOOGLE_API_KEY="your-google-key"
GOOGLE_MODEL="gemini-2.5-pro-exp-03-25"
GOOGLE_ENABLED=true
GOOGLE_MAX_TOKENS=16384

# Custom OpenAI-compatible provider
CUSTOM_NAME="LocalLLM"
CUSTOM_URL="http://localhost:1234/v1"
CUSTOM_MODEL="mixtral-8x7b"
CUSTOM_ENABLED=true
CUSTOM_MAX_TOKENS=16384
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.