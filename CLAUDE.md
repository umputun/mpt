# MPT Development Guidelines

## Project Overview
MPT (Multi-Provider Tool) is a command-line utility that sends prompts to multiple AI language model providers (OpenAI, Anthropic, Google, and custom providers) in parallel and combines the results. It enables easy file inclusion for context and supports flexible pattern matching to quickly include relevant code or documentation in your prompts.

**Module**: `github.com/umputun/mpt`  
**Go Version**: 1.24+  
**License**: MIT

## Architecture

### Core Components
1. **Providers** (`pkg/provider/`): Interfaces and implementations for AI model providers
   - OpenAI provider (`openai.go`)
   - Anthropic/Claude provider (`anthropic.go`)
   - Google/Gemini provider (`google.go`)
   - Custom OpenAI-compatible providers (`custom_openai.go`)
   - Common provider interface and result types (`provider.go`)

2. **Runner** (`pkg/runner/`): Parallel execution of prompts across providers
   - Manages concurrent provider calls
   - Collects and formats results
   - Handles mix mode for result synthesis

3. **Prompt Builder** (`pkg/prompt/`): Constructs prompts with file context
   - File pattern matching and loading
   - Git diff integration
   - Smart exclusion patterns

4. **File Handler** (`pkg/files/`): Advanced file pattern matching
   - Supports glob, bash-style, and Go-style patterns
   - Respects .gitignore and common exclusion patterns

5. **MCP Server** (`pkg/mcp/`): Model Context Protocol server implementation
   - Makes providers available through unified interface
   - Compatible with MCP clients

6. **CLI** (`cmd/mpt/`): Main command-line interface
   - Flag parsing and configuration
   - Input handling (stdin, files, flags)
   - Output formatting

### Key Features
- Multi-provider parallel execution
- Mix mode for result synthesis
- Native git integration (diffs, branch comparisons)
- Smart file inclusion with pattern matching
- Force mode to override exclusions
- Environment variable support for API keys
- JSON output for scripting
- MCP server mode

## Directory Structure
```
mpt/
├── cmd/mpt/              # CLI application
│   ├── main.go          # Entry point and flag parsing
│   └── *_test.go        # CLI tests
├── pkg/                 # Core packages
│   ├── provider/        # Provider implementations
│   ├── runner/          # Parallel execution engine
│   ├── prompt/          # Prompt building and git integration
│   ├── files/           # File pattern matching
│   └── mcp/             # MCP server implementation
├── vendor/              # Vendored dependencies
├── site/                # Documentation site
├── CLAUDE.md           # This file - AI tool guidelines
├── README.md           # User documentation
├── CONTRIBUTING.md     # Contribution guidelines
├── CODE-REVIEW-GUIDE.md # Code review process
├── Makefile            # Build automation
├── go.mod              # Go module definition
└── go.sum              # Dependency checksums
```

## Build & Test Commands
- Build project: `go build -o mpt ./cmd/mpt`
- Build with version info: `go build -ldflags "-X main.revision=$(git describe --tags --always) -s -w" -o mpt ./cmd/mpt`
- Install locally: `go install ./cmd/mpt`
- Run tests: `go test -race ./...`
- Run specific test: `go test -run TestName ./path/to/package`
- Run tests with coverage: `go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`
- Run linting: `golangci-lint run ./...`
- Format code: `gofmt -s -w $(find . -type f -name "*.go" -not -path "./vendor/*" -not -path "./mocks/*" -not -path "**/mocks/*")`
- Run code generation: `go generate ./...`
- Normalize code comments: `command -v unfuck-ai-comments >/dev/null || go install github.com/umputun/unfuck-ai-comments@latest; unfuck-ai-comments run --fmt --skip=mocks ./...`
- On completion, run: formatting, tests, linting, and code generation
- Never commit without running completion sequence


## Important Workflow Notes
- Always run tests, linter and normalize comments before committing
- For linter use `golangci-lint run`
- Run tests and linter after making significant changes to verify functionality
- Go version: 1.24+
- Don't add "Generated with Claude Code" or "Co-Authored-By: Claude" to commit messages or PRs
- Do not include "Test plan" sections in PR descriptions
- Do not add comments that describe changes, progress, or historical modifications. Avoid comments like "new function," "added test," "now we changed this," or "previously used X, now using Y." Comments should only describe the current state and purpose of the code, not its history or evolution.
- Use `go:generate` for generating mocks, never modify generated files manually. Mocks are generated with `moq` and stored in the `mocks` package.
- After important functionality added, update README.md accordingly
- When merging master changes to an active branch, make sure both branches are pulled and up to date first
- Don't add "Test plan" section to PRs

## Code Style Guidelines
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use snake_case for filenames, camelCase for variables, PascalCase for exported names
- Group imports: standard library, then third-party, then local packages
- Error handling: check errors immediately and return them with context
- Use meaningful variable names; avoid single-letter names except in loops
- Validate function parameters at the start before processing
- Return early when possible to avoid deep nesting
- Prefer composition over inheritance
- Interfaces: Define interfaces in consumer packages
- Function size preferences:
  - Aim for functions around 50-60 lines when possible
  - Don't break down functions too small as it can reduce readability
  - Maintain focus on a single responsibility per function
- Comment style: in-function comments should be lowercase sentences
- Code width: keep lines under 130 characters when possible
- Format: Use `gofmt`

### Error Handling
- Use `fmt.Errorf("context: %w", err)` to wrap errors with context
- Check errors immediately after function calls
- Return detailed error information through wrapping

### Comments
- All comments inside functions should be lowercase
- Document all exported items with proper casing
- Use inline comments for complex logic
- Start comments with the name of the thing being described

### Testing
- Use table-driven tests where apropriate
- Use subtest with `t.Run()` to make test more structured
- Use `require` for fatal assertions, `assert` for non-fatal ones
- Use mock interfaces for dependency injection
- Test names follow pattern: `Test<Type>_<method>`


## Git Workflow

### After merging a PR
```bash
# Switch back to the master branch
git checkout master

# Pull latest changes including the merged PR
git pull

# Delete the temporary branch (might need -D for force delete if squash merged)
git branch -D feature-branch-name
```

## CLI Usage and Configuration

### Basic Usage
```bash
mpt --openai.enabled --prompt "Your question here"
mpt --anthropic.enabled --google.enabled --prompt "Compare approaches to X"
echo "Content to analyze" | mpt --openai.enabled --prompt "Analyze this"
```

### Key Command-Line Options
- Provider flags: `--{provider}.enabled`, `--{provider}.api-key`, `--{provider}.model`
- File inclusion: `-f/--file` (supports multiple, glob patterns)
- Exclusion: `-x/--exclude` (patterns to exclude)
- Git integration: `--git.diff`, `--git.branch=<branch>`
- Mix mode: `--mix` (combine results), `--mix.provider`, `--mix.prompt`
- Output: `--json` (JSON format), `-v/--verbose` (show full prompt)
- Timeout: `-t/--timeout` (default: 60s)
- Force mode: `--force` (bypass all exclusions)

### Environment Variables
All provider API keys can be set via environment:
- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GOOGLE_API_KEY`
- Custom providers: `CUSTOM_<ID>_API_KEY`

### Important Patterns and Conventions

#### Provider Pattern
- All providers implement the `Provider` interface
- Providers are configured via options structs
- Parallel execution managed by `Runner`
- Results formatted with provider headers (unless single provider)

#### File Pattern Matching
- Standard glob: `*.go`, `cmd/*.js`
- Bash-style recursive: `**/*.go`, `pkg/**/*.js`
- Go-style recursive: `pkg/...`, `cmd/.../*.go`
- Directory traversal: `pkg/` (recursive)

#### Error Handling
- Always wrap errors with context using `fmt.Errorf`
- Check errors immediately after function calls
- Return errors to caller, don't panic

#### Testing Patterns
- Mock generation: `//go:generate moq -out mocks/interface.go -pkg mocks -skip-ensure -fmt goimports . InterfaceName`
- Table-driven tests with subtests using `t.Run`
- Test files follow `*_test.go` convention
- Integration tests: `*_integr_test.go`

## Libraries
- Logging: `github.com/go-pkgz/lgr`
- CLI flags: `github.com/jessevdk/go-flags`
- Testing: `github.com/stretchr/testify`
- Mock generation: `github.com/matryer/moq`
- AI Providers:
  - OpenAI: `github.com/sashabaranov/go-openai`
  - Anthropic: `github.com/anthropics/anthropic-sdk-go`
  - Google: `github.com/google/generative-ai-go`
- File patterns: `github.com/bmatcuk/doublestar/v4`
- MCP server: `github.com/mark3labs/mcp-go`

## Technical Implementation Details

### Provider Implementation
Each provider must:
1. Implement the `Provider` interface (Name(), Generate(), Enabled())
2. Handle API authentication and configuration
3. Manage context cancellation
4. Format errors appropriately
5. Support configurable models and parameters

### Runner Execution Flow
1. Filter enabled providers
2. Launch goroutines for each provider
3. Collect results via channels
4. Handle timeouts and cancellation
5. Format combined output
6. Optionally mix results with designated provider

### Prompt Building Process
1. Start with base prompt text
2. Process git diff if requested
3. Load files matching patterns
4. Apply exclusion filters
5. Check file size limits
6. Combine all content with separators

### MCP Server Mode
When running as MCP server (`--mcp.enabled`):
- Exposes all enabled providers through MCP protocol
- Handles tool discovery and execution
- Compatible with Claude Desktop and other MCP clients

## Default Configuration

### Default Models
- OpenAI: `gpt-4.1` 
- Anthropic: `claude-3-7-sonnet-20250219`
- Google: `gemini-2.5-pro-exp-03-25`
- Custom: Must be specified via `--custom.<id>.model`

### Default Parameters
- Max tokens: 16384 (0 for model maximum)
- Temperature: 0.7 (OpenAI and custom providers)
- Timeout: 60s
- Max file size: 64KB (configurable with --max-file-size)
- Mix provider: OpenAI
- Mix prompt: "merge results from all providers"

### Common Exclusion Patterns
Automatically excluded unless `--force` is used:
- Version control: `.git/`, `.svn/`, `.hg/`
- Dependencies: `vendor/`, `node_modules/`
- Build artifacts: `dist/`, `build/`, `.bin/`
- IDE/Editor: `.idea/`, `.vscode/`
- OS files: `.DS_Store`, `Thumbs.db`
- Respects `.gitignore` patterns

## Common Use Cases

### Code Review
```bash
mpt --git.diff --openai.enabled --anthropic.enabled --google.enabled \
    --prompt "Review this code for bugs, security issues, and improvements"
```

### Architecture Analysis
```bash
mpt --file "pkg/..." --file "cmd/..." --anthropic.enabled \
    --prompt "Analyze the architecture and suggest improvements"
```

### Documentation Generation
```bash
mpt --file "**/*.go" --exclude "*_test.go" --openai.enabled \
    --prompt "Generate API documentation for public functions"
```

### Mixed Provider Synthesis
```bash
mpt --openai.enabled --anthropic.enabled --google.enabled --mix \
    --prompt "Complex question requiring multiple perspectives"
```