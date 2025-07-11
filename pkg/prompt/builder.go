package prompt

import (
	"fmt"
	"strings"

	"github.com/go-pkgz/lgr"

	"github.com/umputun/mpt/pkg/files"
)

// Builder handles constructing prompts with optional file content using a builder pattern.
// It supports including content from files matched by glob patterns and excluding
// files that match specific exclusion patterns.
type Builder struct {
	baseText    string
	files       []string
	excludes    []string
	maxFileSize int64
	force       bool
}

// New creates a new prompt builder with the provided base text.
// The base text serves as the foundation of the prompt before any file content is added.
func New(baseText string) *Builder {
	return &Builder{
		baseText:    baseText,
		maxFileSize: files.DefaultMaxFileSize,
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
	finalPrompt := b.baseText

	// only process files if patterns were provided
	if len(b.files) > 0 {
		lgr.Printf("[DEBUG] loading files from patterns: %v", b.files)
		if len(b.excludes) > 0 {
			lgr.Printf("[DEBUG] excluding patterns: %v", b.excludes)
		}

		fileContent, err := files.LoadContent(b.files, b.excludes, b.maxFileSize, b.force)
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

// CombineWithInput combines a prompt with input text, adding a newline separator between them.
// If the prompt is empty, only the input text is returned without modification.
func CombineWithInput(prompt, input string) string {
	if prompt == "" {
		return input
	}
	return prompt + "\n" + input
}
