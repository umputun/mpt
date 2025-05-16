# MPT - multi-provider tool for LLMs

[![Build Status](https://github.com/umputun/mpt/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/mpt/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/umputun/mpt/badge.svg?branch=master)](https://coveralls.io/github/umputun/mpt?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/umputun/mpt)](https://goreportcard.com/report/github.com/umputun/mpt)

MPT is a command-line utility that sends prompts to multiple AI language model providers (OpenAI, Anthropic, Google, and custom providers) in parallel and combines the results. It enables easy file inclusion for context and supports flexible pattern matching to quickly include relevant code or documentation in your prompts.

<div align="center">  
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="site/docs/logo-inverted.png">
    <img class="logo" src="site/docs/logo.png" width="400px" alt="MPT">
  </picture>
</div>

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

6. **Acting as an MCP server**: For advanced usage, MPT can run as a Model Context Protocol server, making multiple providers available through a single unified interface to MCP-compatible clients.

For example, when reviewing code changes:
```
# Using native git integration
mpt --git.diff --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this code and identify potential bugs or security issues"

# Or using the traditional pipe approach
git diff HEAD~1 | mpt --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this code and identify potential bugs or security issues"
```

This will show you different perspectives on the same code, potentially catching issues that a single model might overlook.

You can create cascading workflows in two ways:

**Option 1: Manual cascading with files (traditional approach):**
```
# First gather multiple perspectives using the built-in git integration
# (Shows uncommitted changes, or branch diff if no uncommitted changes exist)
mpt --git.diff --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review these uncommitted changes thoroughly" > reviews.txt

# Or with a specific branch comparison
mpt --git.branch=feature-branch --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this pull request thoroughly" > reviews.txt

# Then have a single model summarize the key points
cat reviews.txt | mpt --anthropic.enabled \
    --prompt "Synthesize these reviews into a concise summary of the key issues and improvements"
```

The second command will produce clean output without any provider headers, since only one provider is enabled.

**Option 2: Built-in mixing of results (new approach):**
```
# Use mix mode to automatically combine the results in a single command
mpt --git.diff --openai.enabled --anthropic.enabled --google.enabled --mix \
    --prompt "Review these uncommitted changes thoroughly"
```

With the `--mix` flag enabled, MPT will:
1. Send the prompt to all enabled providers
2. Collect their responses
3. Pass all responses to a designated provider (by default OpenAI) with a mixing prompt
4. Return the synthesized result

This integrated approach is more efficient and convenient than the manual process above.

## Key Features

- **Multi-Provider Support**: Run prompts in parallel across OpenAI, Anthropic (Claude), Google (Gemini), and custom LLMs
- **Mix Mode**: Combine results from multiple providers using a single provider for synthesis
- **File Context Inclusion**: Easily add files, directories, or patterns to provide context for your prompts
- **Native Git Integration**: Include git diffs from uncommitted changes or between branches with simple flags
- **Smart Pattern Matching**: Include files using standard glob patterns, directory paths, bash-style wildcards (`**/*.go`), or Go-style patterns (`pkg/...`)
- **Exclusion Filtering**: Filter out unwanted files with the same pattern matching syntax (`--exclude "**/tests/**"`)
- **Smart Exclusions**: Automatically respects .gitignore patterns and commonly ignored directories
- **Stdin Integration**: Pipe content directly from other tools for AI analysis
- **Customizable Execution**: Configure timeouts, token limits, and models per provider
- **Clean Output Formatting**: Provider-specific headers (or none when using a single provider)
- **Environment Variable Support**: Store API keys and settings in environment variables instead of flags
- **MCP Server Mode**: Run as a Model Context Protocol server to make your providers accessible to MCP-compatible clients

## Installation

```
go install github.com/umputun/mpt/cmd/mpt@latest
```

<details markdown>
  <summary>Other install methods</summary>

**Install from binary release**

Download the appropriate binary for your platform from [Releases](https://github.com/umputun/mpt/releases).

**Install from homebrew (macOS)**

```bash
brew tap umputun/apps
brew install umputun/apps/mpt
```

**Install from deb package (Ubuntu/Debian)**

1. Download the latest version of the package by running: `wget https://github.com/umputun/mpt/releases/download/<version>/mpt_<version>_linux_<arch>.deb` (replace `<version>` and `<arch>` with the actual values).
2. Install the package by running: `sudo dpkg -i mpt_<version>_linux_<arch>.deb`

Example for the latest version and amd64 architecture:

```bash
# Replace v0.1.0 with the actual version
wget https://github.com/umputun/mpt/releases/download/v0.1.0/mpt_v0.1.0_linux_x86_64.deb
sudo dpkg -i mpt_v0.1.0_linux_x86_64.deb
```

**Install from rpm package (CentOS/RHEL/Fedora/AWS Linux)**

```bash
# Replace v0.1.0 with the actual version
wget https://github.com/umputun/mpt/releases/download/v0.1.0/mpt_v0.1.0_linux_x86_64.rpm
sudo rpm -i mpt_v0.1.0_linux_x86_64.rpm
```

**Install from apk package (Alpine)**

```bash
# Replace v0.1.0 with the actual version
wget https://github.com/umputun/mpt/releases/download/v0.1.0/mpt_v0.1.0_linux_x86_64.apk
sudo apk add mpt_v0.1.0_linux_x86_64.apk
```

</details>

## Usage

MPT is primarily used to query multiple providers and get responses:

```
mpt [options]
```

You can provide a prompt in the following ways:
1. Using the `--prompt` flag: `mpt --prompt "Your question here"`
2. Piping content: `echo "Your question" | mpt`
3. Combining flag and piped content: `echo "Additional context" | mpt --prompt "Main question"`
   - When both are provided, MPT automatically combines them with a newline separator
   - The CLI prompt (`--prompt` flag) appears first, followed by the piped stdin content
   - This is especially useful for adding instructions to process piped data (see [Why Combine Inputs?](#why-combine-inputs) section)
4. Interactive mode: If no prompt is provided via command line or pipe, you'll be prompted to enter one

### Provider Configuration

#### OpenAI

```
--openai.api-key      OpenAI API key (or OPENAI_API_KEY env var)
--openai.model        OpenAI model to use (default: gpt-4o)
--openai.enabled      Enable OpenAI provider
--openai.max-tokens   Maximum number of tokens to generate (default: 16384, 0 for model maximum)
--openai.temperature  Controls randomness (0-1, higher is more random) (default: 0.7)
```

#### Anthropic (Claude)

```
--anthropic.api-key   Anthropic API key (or ANTHROPIC_API_KEY env var)
--anthropic.model     Anthropic model to use (default: claude-3-7-sonnet-20250219)
--anthropic.enabled   Enable Anthropic provider
--anthropic.max-tokens Maximum number of tokens to generate (default: 16384, 0 for model maximum)
```

#### Google (Gemini)

```
--google.api-key      Google API key (or GOOGLE_API_KEY env var)
--google.model        Google model to use (default: gemini-2.5-pro-exp-03-25)
--google.enabled      Enable Google provider
--google.max-tokens   Maximum number of tokens to generate (default: 16384, 0 for model maximum)
```

#### Custom OpenAI-Compatible Providers

You can add multiple custom providers that implement the OpenAI-compatible API. Use a unique identifier for each provider:

```
--custom.<provider-id>.name         Name for the custom provider (required)
--custom.<provider-id>.url          Base URL for the custom provider API (required)
--custom.<provider-id>.api-key      API key for the custom provider (if needed)
--custom.<provider-id>.model        Model to use (required)
--custom.<provider-id>.enabled      Enable this custom provider (default: true)
--custom.<provider-id>.max-tokens   Maximum number of tokens to generate (default: 16384, 0 for model maximum)
--custom.<provider-id>.temperature  Controls randomness (0-1, higher is more random) (default: 0.7)
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
--git.diff            Include git diff (uncommitted changes) in the prompt context
--git.branch          Include git diff between given branch and main/master (for PR review)
-t, --timeout         Timeout duration (e.g., 60s, 2m) (default: 60s)
--max-file-size       Maximum size of individual files to process in bytes (default: 64KB)
--mix                 Enable mix mode to combine results from all providers
--mix.provider        Provider to use for mixing results (default: "openai")
--mix.prompt          Prompt used for mixing results (default: "merge results from all providers")
-v, --verbose         Verbose output, shows the complete prompt sent to models
--json                Output results in JSON format for scripting and automation
--dbg                 Enable debug mode
-V, --version         Show version information
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

### Git Integration

MPT provides built-in git integration, allowing you to easily incorporate git diffs into your prompts without manual piping:

```bash
# Include uncommitted changes in the prompt context
# If no uncommitted changes exist, automatically shows diff between current branch and main/master
mpt --git.diff --anthropic.enabled --prompt "Review my changes and suggest improvements"

# Include diff between a specific branch and the default branch (main or master)
mpt --git.branch=feature-branch --openai.enabled --prompt "Review this PR"

# Combine with other files for additional context
mpt --git.diff --file "README.md" --prompt "Explain what these changes do"

# Automatic branch diff detection: if you're on a feature branch with no uncommitted changes,
# --git.diff will automatically show the diff between your branch and main/master
git checkout feature-branch
git add . && git commit -m "all changes committed"
mpt --git.diff --prompt "Review this branch"  # Shows diff between feature-branch and main/master
```

This is more convenient than the traditional pipe approach (`git diff | mpt ...`) because:

1. MPT handles all the temporary file creation and cleanup
2. The diffs are clearly labeled in the context
3. You can easily combine git diffs with other context files
4. It works well in shell scripts and aliases
5. Automatically detects when to show uncommitted changes vs branch differences

#### Git Integration Options

```
--git.diff            Include git diff as context (uncommitted changes)
                      If no uncommitted changes exist, automatically shows diff
                      between current branch and main/master (if applicable)
--git.branch=BRANCH   Include git diff between given branch and master/main (for PR review)
```

### File Pattern and Filtering Reference

MPT provides powerful file inclusion and exclusion capabilities to provide contextual information to AI models. You can easily include all the necessary files for your prompt while filtering out unwanted content.

> **Security Warning**: File glob patterns in MPT have access to the entire file system. Be careful when using patterns like `**/*` or `/...` as they can potentially include sensitive files from anywhere on your system. Always review the matched files when using broad patterns, especially in environments with sensitive data. Consider using more specific patterns and using exclusion filters for sensitive directories.

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

#### Built-in Smart Exclusions

MPT automatically excludes common directories and files you typically don't want to include:

1. **Common Ignored Directories** - Always excluded by default:
   - Version control: `.git`, `.svn`, `.hg`, `.bzr`
   - Build outputs and dependencies: `vendor`, `node_modules`, `.venv`, `__pycache__`, etc.
   - IDE files: `.idea`, `.vscode`, `.vs`
   - Logs and metadata files: `logs`, `*.log`, `.DS_Store`, etc.

2. **.gitignore Integration**:
   - All patterns from the `.gitignore` file in the current directory are converted to glob patterns
   - Files matching those patterns are automatically excluded from the results
   - This works transparently with all file inclusion methods
   
3. **Priority Rules**:
   - Explicit `--exclude` patterns take precedence over both common patterns and `.gitignore` patterns

This means you don't need to manually exclude common directories like `.git`, `node_modules`, or build artifacts - they're automatically filtered out even without a `.gitignore` file.

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

### Using MPT for Code Reviews

MPT is particularly effective for code reviews. You can use the built-in git integration for a streamlined experience:

```bash
# Review uncommitted changes
mpt --git.diff --openai.enabled --timeout=5m \
    -p "Perform a comprehensive code review of these changes"

# Review a pull request by comparing branches
mpt --git.branch=feature-xyz --anthropic.enabled --timeout=5m \
    -p "Perform a comprehensive code review of this PR"
```

For more detailed reviews with multiple providers:

```bash
# Review uncommitted changes with multiple providers
mpt --git.diff --openai.enabled --google.enabled --anthropic.enabled --timeout=5m \
    -p "Perform a comprehensive code review of these changes. Analyze the design patterns and architecture. Identify any security vulnerabilities or risks. Evaluate code readability, maintainability, and idiomatic usage. Suggest specific improvements where needed."
```

The traditional approach also works by saving git diff output to a file:

```bash
# Save changes to a file
git diff > changes.diff

# Run review with file input
mpt -f changes.diff --openai.enabled --google.enabled --anthropic.enabled --timeout=5m \
    -p "Perform a comprehensive code review of these changes. Analyze the design patterns and architecture. Identify any security vulnerabilities or risks. Evaluate code readability, maintainability, and idiomatic usage. Suggest specific improvements where needed."
```

See [CODE-REVIEW-GUIDE.md](CODE-REVIEW-GUIDE.md) for a detailed guide on using MPT for code reviews, including:

- Step-by-step process for reviewing code changes
- Special prompts optimized for different types of reviews
- A template for organizing review results
- Examples of specific improvement recommendations

This approach gives you insights from multiple AI models, helping you catch issues that any single model might miss.

### Why Combine Inputs?

When you provide both a CLI prompt (using the `--prompt` flag) and piped stdin content, MPT combines them as follows:

```
[CLI Prompt from --prompt flag]
[Piped content from stdin]
```

The two are always combined in this order with a newline separator. This automatic combination behavior is particularly powerful for workflows where you want to:

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

#### Example of Combined Input

If you run:
```bash
echo "function calculateTotal(items) {
  return items.reduce((sum, item) => sum + item.price, 0);
}" | mpt --openai.enabled --prompt "Review this JavaScript function for potential bugs and edge cases"
```

MPT will send the following combined prompt to the AI model:
```
Review this JavaScript function for potential bugs and edge cases
function calculateTotal(items) {
  return items.reduce((sum, item) => sum + item.price, 0);
}
```

The model can then analyze the code while following your specific instructions.

### JSON Output Format

When using the `--json` flag, MPT outputs results in a structured JSON format that's easy to parse in scripts or other programs:

```bash
mpt --openai.enabled --anthropic.enabled --prompt "Explain quantum computing" --json
```

This produces JSON output like:

```json
{
  "responses": [
    {
      "provider": "OpenAI (gpt-4o)",
      "text": "Quantum computing is a type of computing that..."
    },
    {
      "provider": "Anthropic (claude-3-7-sonnet)",
      "text": "Quantum computing leverages the principles of quantum mechanics..."
    }
  ],
  "timestamp": "2025-04-15T12:34:56Z"
}
```

When using mix mode with JSON output, an additional `mixed` field is included:

```bash
mpt --openai.enabled --anthropic.enabled --mix --prompt "Explain quantum computing" --json
```

```json
{
  "responses": [
    {
      "provider": "OpenAI (gpt-4o)",
      "text": "Quantum computing is a type of computing that..."
    },
    {
      "provider": "Anthropic (claude-3-7-sonnet)",
      "text": "Quantum computing leverages the principles of quantum mechanics..."
    }
  ],
  "mixed": "== mixed results by OpenAI ==\nQuantum computing is a revolutionary approach that combines...",
  "timestamp": "2025-04-15T12:34:56Z"
}
```

The JSON output includes:
- `responses`: An array of individual provider responses, including:
  - `provider`: The name of the provider
  - `text`: The response text
  - `error`: Error message if the provider failed (field only present for failed providers)
- `mixed`: Combined result when mix mode is enabled (only present with `--mix`)
- `timestamp`: ISO-8601 timestamp when the response was generated

This format is particularly useful for:
- Processing MPT results in scripts
- Storing responses in a database
- Programmatic comparison of responses from different providers
- Integration with other tools in automation pipelines

### Standard Text Output Format

By default, MPT outputs results in a human-readable text format:

```
== generated by OpenAI ==
Recursion is a programming concept where a function calls itself...

== generated by Anthropic ==
Recursion is a technique in programming where a function solves a problem by...

== generated by Google ==
Recursion in programming is when a function calls itself during its execution...
```

When only one provider is enabled, the header is omitted for cleaner output. 

When using mix mode, an additional section is added with the mixed results:

```
== generated by OpenAI ==
Recursion is a programming concept where a function calls itself...

== generated by Anthropic ==
Recursion is a technique in programming where a function solves a problem by...

== mixed results by OpenAI ==
Recursion is a fundamental programming concept where a function calls itself during execution...
```

## Advanced Usage: MCP Server Mode

In addition to the standard prompt-based usage, MPT can also run as an [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server:

```
mpt --mcp.server [other options]
```

### What is MCP Server Mode?

MCP server mode allows MPT to act as a bridge between MCP-compatible clients and multiple LLM providers. This enables:

- Using MPT as a tool within MCP-compatible applications
- Accessing multiple LLM providers through a single unified interface
- Abstracting provider-specific details behind the MCP protocol

### MCP Server Mode Options

```
--mcp.server          Run in MCP server mode
--mcp.server-name     MCP server name (default: "MPT MCP Server")
```

### Using MPT as an MCP Tool

MPT can be used as an MCP tool that MCP-compatible clients can invoke:

```bash
# Set up provider API keys
export OPENAI_API_KEY="your-openai-key"
export ANTHROPIC_API_KEY="your-anthropic-key"
export GOOGLE_API_KEY="your-google-key"

# Start MPT in MCP server mode with your chosen providers
mpt --mcp.server --openai.enabled --anthropic.enabled --google.enabled
```

When run in MCP server mode, MPT communicates with the MCP client using the Model Context Protocol over standard input/output. The client can then use MPT's multiple providers as if they were a single provider, with MPT handling all the provider-specific details.

## Using Environment Variables

You can use environment variables instead of command-line flags:

```
OPENAI_API_KEY="your-openai-key"
OPENAI_MODEL="gpt-4o"
OPENAI_ENABLED=true
OPENAI_MAX_TOKENS=16384
OPENAI_TEMPERATURE=0.7

ANTHROPIC_API_KEY="your-anthropic-key"
ANTHROPIC_MODEL="claude-3-7-sonnet-20250219"
ANTHROPIC_ENABLED=true
ANTHROPIC_MAX_TOKENS=16384

GOOGLE_API_KEY="your-google-key"
GOOGLE_MODEL="gemini-2.5-pro-exp-03-25"
GOOGLE_ENABLED=true
GOOGLE_MAX_TOKENS=16384

# Git options
GIT_DIFF=true            # Include git diff (uncommitted changes)
GIT_BRANCH="feature-xyz" # Include diff between feature-xyz and main/master

# MCP Server Mode
MCP_SERVER=true
MCP_SERVER_NAME="My MPT MCP Server"

# Custom OpenAI-compatible provider
CUSTOM_NAME="LocalLLM"
CUSTOM_URL="http://localhost:1234/v1"
CUSTOM_MODEL="mixtral-8x7b"
CUSTOM_ENABLED=true
CUSTOM_MAX_TOKENS=16384
CUSTOM_TEMPERATURE=0.7

# Mix options
MIX=true                # Enable mix mode
MIX_PROVIDER="openai"   # Provider to use for mixing results
MIX_PROMPT="merge results from all providers" # Custom prompt for mixing
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
