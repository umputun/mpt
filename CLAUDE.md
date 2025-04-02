# Development Guidelines

## Build & Test Commands
- Build Go projects: `go build ./...`
- Run tests: `go test ./...`
- Run specific test: `go test -run TestName ./path/to/package`
- Run tests with coverage: `go test -cover ./...`
- Run linting: `golangci-lint run ./...`
- Format code: `gofmt -s -w .`
- Run code generation: `go generate ./...`
- Coverage report: `go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`
- Normalize code comments: `command -v unfuck-ai-comments >/dev/null || go install github.com/umputun/unfuck-ai-comments@latest; unfuck-ai-comments run --fmt --skip=mocks ./...`
- On completion, run: formatting, tests, and code generation
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

## Libraries
- Logging: `github.com/go-pkgz/lgr`
- CLI flags: `github.com/jessevdk/go-flags`
- HTTP/REST: `github.com/go-pkgz/rest` with `github.com/go-pkgz/routegroup`
- Middleware: `github.com/didip/tollbooth/v8`
- Database: `github.com/jmoiron/sqlx` with `modernc.org/sqlite`
- Testing: `github.com/stretchr/testify`
- Testing helpers: `github.com/go-pkgz/testutils`
- Mock generation: `github.com/matryer/moq`
- OpenAI: `github.com/sashabaranov/go-openai`
- Frontend: HTMX v2. Try to avoid using JS.
- For containerized tests use `github.com/go-pkgz/testutils`
- To access libraries, figure how to use ang check their documentation, use `go doc` command and `gh` tool