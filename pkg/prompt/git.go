package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/go-pkgz/lgr"
)

//go:generate moq -out mocks/git_executor.go -pkg mocks -skip-ensure -fmt goimports . GitExecutor

// GitExecutor defines operations for executing git commands
type GitExecutor interface {
	LookPath(file string) (string, error)
	Command(name string, args ...string) *exec.Cmd
	CommandOutput(cmd *exec.Cmd) ([]byte, error)
	CommandRun(cmd *exec.Cmd) error
}

// default implementation
type defaultGitExecutor struct{}

func (e *defaultGitExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (e *defaultGitExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (e *defaultGitExecutor) CommandOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func (e *defaultGitExecutor) CommandRun(cmd *exec.Cmd) error {
	return cmd.Run()
}

// default executor instance
var executor GitExecutor = &defaultGitExecutor{}

// WithGitDiff adds uncommitted changes from git diff to the prompt
// Creates a temporary file with the diff output and adds it to the files to process
func (b *Builder) WithGitDiff() (*Builder, error) {
	tempFile, description, err := processGitDiff(true, "")
	if err != nil {
		return b, err
	}

	if tempFile != "" {
		// add temporary file to cleanup list
		gitCleanupFiles = append(gitCleanupFiles, tempFile)

		// add the file to the list of files to include
		b.files = append(b.files, tempFile)

		// prepend a description of the git diff to the prompt
		if b.baseText != "" {
			b.baseText = fmt.Sprintf("I'm providing %s for context.\n\n%s", description, b.baseText)
		}
	}

	return b, nil
}

// WithGitBranchDiff adds git diff between the specified branch and the default branch
func (b *Builder) WithGitBranchDiff(branch string) (*Builder, error) {
	tempFile, description, err := processGitDiff(false, branch)
	if err != nil {
		return b, err
	}

	if tempFile != "" {
		// add temporary file to cleanup list
		gitCleanupFiles = append(gitCleanupFiles, tempFile)

		// add the file to the list of files to include
		b.files = append(b.files, tempFile)

		// prepend a description of the git diff to the prompt
		if b.baseText != "" {
			b.baseText = fmt.Sprintf("I'm providing %s for context.\n\n%s", description, b.baseText)
		}
	}

	return b, nil
}

// var to store files to cleanup
var gitCleanupFiles []string

// CleanupGitDiffFiles removes all temporary git diff files
func CleanupGitDiffFiles() {
	for _, filePath := range gitCleanupFiles {
		if err := os.Remove(filePath); err != nil {
			if !os.IsNotExist(err) {
				lgr.Printf("[WARN] failed to remove temporary git diff file: %v", err)
			}
		} else {
			lgr.Printf("[DEBUG] removed temporary git diff file: %s", filePath)
		}
	}
	// clear the list
	gitCleanupFiles = []string{}
}

// processGitDiff handles git diff extraction and returns a file path with the diff content
// isDiff indicates whether to get uncommitted changes, if false branchName is used for branch comparison
func processGitDiff(isDiff bool, branchName string) (tempFilePath, diffDescription string, err error) {
	// verify git is available in the system
	if _, err := executor.LookPath("git"); err != nil {
		return "", "", fmt.Errorf("git executable not found: %w", err)
	}

	// create temporary directory if it doesn't exist
	tempDir := os.TempDir()
	if _, err := os.Stat(tempDir); err != nil {
		return "", "", fmt.Errorf("failed to access temp directory: %w", err)
	}

	// generate a unique filename for the diff output
	timestamp := time.Now().Format("20060102-150405")
	tempFile := filepath.Join(tempDir, fmt.Sprintf("mpt-git-diff-%s.txt", timestamp))

	// generate diff based on the provided option
	var diffCmd *exec.Cmd

	switch {
	case isDiff:
		// get uncommitted changes
		diffCmd = executor.Command("git", "diff")
		diffDescription = "git diff (uncommitted changes)"

	case branchName != "":
		// try to find the default branch (main or master)
		defaultBranch := getDefaultBranch()
		// sanitize branch name to prevent command injection
		sanitizedBranch := sanitizeBranchName(branchName)
		if sanitizedBranch == "" {
			return "", "", fmt.Errorf("invalid branch name: %s", branchName)
		}
		// use separate args for diff command with branch comparison
		diffCmd = executor.Command("git", "diff", defaultBranch+"..."+sanitizedBranch) // #nosec G204 - sanitizeBranchName ensures the input is safe
		diffDescription = fmt.Sprintf("git diff between %s and %s", defaultBranch, sanitizedBranch)
	}

	// execute the git command and capture output
	diffOutput, err := executor.CommandOutput(diffCmd)
	if err != nil {
		return "", "", fmt.Errorf("git command failed: %w", err)
	}

	// skip if no differences found
	if len(diffOutput) == 0 {
		lgr.Printf("[INFO] no git differences found, skipping git context")
		return "", "", nil
	}

	// write the diff output to the temporary file
	if err := os.WriteFile(tempFile, diffOutput, 0o600); err != nil {
		return "", "", fmt.Errorf("failed to write git diff to temporary file: %w", err)
	}

	lgr.Printf("[INFO] wrote git diff to temporary file: %s", tempFile)
	return tempFile, diffDescription, nil
}

// getDefaultBranch tries to determine the default branch (main or master) for the repository
func getDefaultBranch() string {
	// check if main branch exists
	if checkBranchExists("main") {
		return "main"
	}

	// fallback to master
	return "master"
}

// checkBranchExists checks if a branch exists in the repository
func checkBranchExists(branch string) bool {
	cmd := executor.Command("git", "rev-parse", "--verify", branch)
	return executor.CommandRun(cmd) == nil
}

// sanitizeBranchName ensures the branch name is a valid git reference
// Returns empty string if the name is invalid or potentially unsafe
func sanitizeBranchName(branch string) string {
	// check for common unsafe characters
	if strings.ContainsAny(branch, ";&|<>$()[]{}!#`\\\"'") {
		return ""
	}

	// additional validation: only allow alphanumeric, dash, underscore, period, and forward slash
	for _, c := range branch {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' && c != '.' && c != '/' {
			return ""
		}
	}

	// verify it's a valid git reference by checking if it exists
	if !isValidGitRef(branch) {
		return ""
	}

	return branch
}

// isValidGitRef checks if a git reference is valid without executing it
func isValidGitRef(ref string) bool {
	cmd := executor.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+ref) // #nosec G204 - only called after sanitization
	if executor.CommandRun(cmd) == nil {
		return true
	}

	// also check if it's a valid remote branch
	cmd = executor.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/"+ref) // #nosec G204 - only called after sanitization
	return executor.CommandRun(cmd) == nil
}
