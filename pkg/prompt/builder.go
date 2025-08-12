package prompt

import (
	"fmt"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/files"
)

//go:generate moq -out mocks/git_diff_processor.go -pkg mocks -skip-ensure -fmt goimports . GitDiffProcessor

// GitDiffProcessor handles git diff operations
type GitDiffProcessor interface {
	ProcessGitDiff(isDiff bool, branchName string) (tempFilePath, diffDescription string, err error)
	TryBranchDiff() (tempFile, description string, err error)
	Cleanup()
}

// Builder handles constructing prompts with optional file content using a builder pattern.
// It supports including content from files matched by glob patterns and excluding
// files that match specific exclusion patterns.
type Builder struct {
	baseText    string
	files       []string
	excludes    []string
	maxFileSize int64
	force       bool
	gitDiffer   GitDiffProcessor
}

// New creates a new prompt builder with the provided base text.
// The base text serves as the foundation of the prompt before any file content is added.
func New(baseText string, gitDiffer GitDiffProcessor) *Builder {
	return &Builder{
		baseText:    baseText,
		maxFileSize: files.DefaultMaxFileSize,
		gitDiffer:   gitDiffer,
	}
}

// WithFiles adds file glob patterns to include in the prompt.
// These patterns will be used to find and load file content.
// Supports standard glob, bash-style, and go-style recursive patterns.
func (b *Builder) WithFiles(filePatterns []string) *Builder {
	b.files = filePatterns
	return b
}

// WithExcludes adds file patterns to exclude from the prompt.
// Files matching these patterns will be skipped even if they match include patterns.
func (b *Builder) WithExcludes(excludePatterns []string) *Builder {
	b.excludes = excludePatterns
	return b
}

// WithMaxFileSize sets the maximum size of individual files to process.
func (b *Builder) WithMaxFileSize(maxFileSize int64) *Builder {
	b.maxFileSize = maxFileSize
	return b
}

// WithForce enables force mode to skip all exclusion patterns.
func (b *Builder) WithForce(force bool) *Builder {
	b.force = force
	return b
}

// Build constructs the final prompt string by combining the base text with
// content from the matched files. Returns an error if file loading fails.
func (b *Builder) Build() (string, error) {
	// ensure cleanup happens after build if gitDiffer is not nil
	if b.gitDiffer != nil {
		defer b.gitDiffer.Cleanup()
	}

	finalPrompt := b.baseText

	// only process files if patterns were provided
	if len(b.files) > 0 {
		lgr.Printf("[DEBUG] loading files from patterns: %v", b.files)
		if len(b.excludes) > 0 {
			lgr.Printf("[DEBUG] excluding patterns: %v", b.excludes)
		}

		fileContent, err := files.LoadContent(files.LoadRequest{
			Patterns:        b.files,
			ExcludePatterns: b.excludes,
			MaxFileSize:     b.maxFileSize,
			Force:           b.force,
		})
		if err != nil {
			return "", fmt.Errorf("failed to load files: %w", err)
		}

		if fileContent != "" {
			lgr.Printf("[DEBUG] loaded %d bytes of content from files", len(fileContent))
			finalPrompt += "\n\n" + fileContent
		}
	}

	return strings.TrimSpace(finalPrompt), nil
}

// WithGitDiff adds uncommitted changes from git diff to the prompt
// Creates a temporary file with the diff output and adds it to the files to process
func (b *Builder) WithGitDiff() (*Builder, error) {
	if b.gitDiffer == nil {
		return b, fmt.Errorf("git diff requested but git differ not initialized")
	}

	// first try to get uncommitted changes
	tempFile, description, err := b.gitDiffer.ProcessGitDiff(true, "")
	if err != nil {
		return b, err
	}

	// early return if we have uncommitted changes
	if tempFile != "" {
		return b.addGitDiffFile(tempFile, description), nil
	}

	// no uncommitted changes, try branch diff
	tempFile, description, err = b.gitDiffer.TryBranchDiff()
	if err != nil {
		return b, err
	}

	if tempFile != "" {
		return b.addGitDiffFile(tempFile, description), nil
	}

	return b, nil
}

// WithGitBranchDiff adds git diff between the specified branch and the default branch
func (b *Builder) WithGitBranchDiff(branch string) (*Builder, error) {
	if b.gitDiffer == nil {
		return b, fmt.Errorf("git branch diff requested but git differ not initialized")
	}

	tempFile, description, err := b.gitDiffer.ProcessGitDiff(false, branch)
	if err != nil {
		return b, err
	}

	if tempFile != "" {
		return b.addGitDiffFile(tempFile, description), nil
	}

	return b, nil
}

// addGitDiffFile adds the git diff file to the builder
func (b *Builder) addGitDiffFile(tempFile, description string) *Builder {
	// add the file to the list of files to include
	b.files = append(b.files, tempFile)

	// prepend a description of the git diff to the prompt
	if b.baseText != "" {
		b.baseText = fmt.Sprintf("I'm providing %s for context.\n\n%s", description, b.baseText)
	}

	return b
}

// CombineWithInput combines a prompt with input text, adding a newline separator between them.
// If the prompt is empty, only the input text is returned without modification.
func CombineWithInput(prompt, input string) string {
	if prompt == "" {
		return input
	}
	return prompt + "\n" + input
}
