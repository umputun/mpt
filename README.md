# MPT - Multi-Provider Tool

[![Build Status](https://github.com/umputun/mpt/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/mpt/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/umputun/mpt/badge.svg?branch=master)](https://coveralls.io/github/umputun/mpt?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/umputun/mpt)](https://goreportcard.com/report/github.com/umputun/mpt)

MPT is a command-line utility that sends prompts to multiple AI language model providers in parallel and combines the results.

## Why MPT?

When working with AI language models, different providers often have unique strengths and perspectives. MPT allows you to:

1. **Compare responses across providers**: See how different models approach the same problem, revealing insights that a single model might miss.

2. **Leverage specialized capabilities**: Each AI system has strengths - one might excel at coding tasks while another provides clearer explanations or more creative solutions.

3. **Get multiple perspectives quickly**: Instead of sequentially querying each provider, get all responses in a single operation, saving time and effort.

4. **Improve reliability**: By not relying on a single provider, you reduce the risk of service outages or rate limiting affecting your workflow.

5. **Customize for your needs**: With support for local LLMs via custom OpenAI-compatible endpoints, you can combine private and public models in one interface.

6. **Cleaner single-provider output**: When only one provider is enabled, MPT automatically skips the provider headers for cleaner, more usable output.

7. **Cascade and summarize responses**: Use the output from multiple models as input for a secondary analysis.

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

## Features

- Parallel execution of prompts across multiple LLM providers
- Support for OpenAI, Anthropic (Claude), and Google (Gemini) models
- Support for custom OpenAI-compatible providers (local LLMs, alternative APIs)
- Easy configuration via command-line flags or environment variables
- Customizable timeout for requests
- Configurable token limits for each provider
- Clear results formatting with provider-specific headers

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
--openai.model        OpenAI model to use (default: gpt-4-turbo-preview)
--openai.enabled      Enable OpenAI provider
--openai.max-tokens   Maximum number of tokens to generate (default: 1024)
```

#### Anthropic (Claude)

```
--anthropic.api-key   Anthropic API key (or ANTHROPIC_API_KEY env var)
--anthropic.model     Anthropic model to use (default: claude-3-sonnet-20240229)
--anthropic.enabled   Enable Anthropic provider
--anthropic.max-tokens Maximum number of tokens to generate (default: 1024)
```

#### Google (Gemini)

```
--google.api-key      Google API key (or GOOGLE_API_KEY env var)
--google.model        Google model to use (default: gemini-1.5-pro)
--google.enabled      Enable Google provider
--google.max-tokens   Maximum number of tokens to generate (default: 1024)
```

#### Custom OpenAI-Compatible Providers

You can add multiple custom providers that implement the OpenAI-compatible API:

```
--custom.name         Name for the custom provider (required)
--custom.url          Base URL for the custom provider API (required)
--custom.api-key      API key for the custom provider (if needed)
--custom.model        Model to use (required)
--custom.enabled      Enable this custom provider (default: true)
--custom.max-tokens   Maximum number of tokens to generate (default: 1024)
```

Example for adding a local LLM server:

```
mpt --custom.name "LocalLLM" --custom.url "http://localhost:1234/v1" \
    --custom.model "mixtral-8x7b" --prompt "Explain quantum computing"
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
-t, --timeout         Timeout in seconds (default: 60)
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

### File Pattern Reference

MPT supports several types of file patterns for the `--file` flag:

1. **Specific Files**
   ```
   --file "README.md"              # A single file
   --file "Makefile"               # Another single file
   ```

2. **Standard Glob Patterns**
   ```
   --file "*.go"                   # All Go files in current directory
   --file "cmd/*.go"               # All Go files in cmd directory
   --file "pkg/*_test.go"          # All Go test files in pkg directory
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

> **Note:** You can use either bash-style patterns with `**` or Go-style patterns with `/...` for recursive matching. Choose whichever syntax you prefer.

When multiple patterns are provided, all matching files are included in the prompt context:

```
# Include all Go files and all markdown files
mpt --anthropic.enabled --prompt "Explain this codebase" --file "**/*.go" --file "*.md"

# Include all code files from pkg and documentation
mpt --openai.enabled --prompt "Document this API" --file "pkg/..." --file "*.md"

# Show the prompt that's being sent to the models
mpt --anthropic.enabled --prompt "Explain this code" --file "*.go" -v

# Include Go files but exclude test files and mocks
mpt --openai.enabled --prompt "Explain this code" --file "**/*.go" --exclude "**/*_test.go" --exclude "**/mocks/**"
```

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
OPENAI_MODEL="gpt-4"
OPENAI_ENABLED=true
OPENAI_MAX_TOKENS=1024

ANTHROPIC_API_KEY="your-anthropic-key"
ANTHROPIC_MODEL="claude-3-opus-20240229"
ANTHROPIC_ENABLED=true
ANTHROPIC_MAX_TOKENS=1024

GOOGLE_API_KEY="your-google-key"
GOOGLE_MODEL="gemini-1.5-pro"
GOOGLE_ENABLED=true
GOOGLE_MAX_TOKENS=1024

# Custom OpenAI-compatible provider
CUSTOM_NAME="LocalLLM"
CUSTOM_URL="http://localhost:1234/v1"
CUSTOM_MODEL="mixtral-8x7b"
CUSTOM_ENABLED=true
CUSTOM_MAX_TOKENS=1024
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.