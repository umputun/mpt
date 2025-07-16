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

// gitDiffer handles git diff operations and temporary file management
type gitDiffer struct {
	executor GitExecutor
	tempDir  string
}

// newGitDiffer creates a new gitDiffer with the default executor (for internal use)
func newGitDiffer() *gitDiffer {
	// create a unique temp directory for this session
	tempDir, err := os.MkdirTemp("", "mpt-git-diff-*")
	if err != nil {
		lgr.Printf("[WARN] failed to create temp directory, using system temp: %v", err)
		tempDir = os.TempDir()
	}

	return &gitDiffer{
		executor: executor,
		tempDir:  tempDir,
	}
}

// NewGitDiffer creates a new GitDiffProcessor with the default executor
func NewGitDiffer() GitDiffProcessor {
	return newGitDiffer()
}

// Cleanup removes the temporary directory and all its contents
func (g *gitDiffer) Cleanup() {
	// skip if using system temp dir (fallback case)
	if g.tempDir == os.TempDir() {
		return
	}

	if err := os.RemoveAll(g.tempDir); err != nil {
		lgr.Printf("[WARN] failed to remove temporary directory %s: %v", g.tempDir, err)
	} else {
		lgr.Printf("[DEBUG] removed temporary directory: %s", g.tempDir)
	}
}

// TryBranchDiff attempts to get diff between current branch and default branch
func (g *gitDiffer) TryBranchDiff() (tempFile, description string, err error) {
	currentBranch := g.getCurrentBranch()
	defaultBranch := g.getDefaultBranch()

	// sanitize and validate current branch
	if currentBranch != "" {
		currentBranch = g.sanitizeBranchName(currentBranch)
		if currentBranch == "" {
			lgr.Printf("[WARN] invalid current branch name, skipping branch comparison")
			return "", "", nil
		}
	}

	// sanitize and validate default branch
	if defaultBranch != "" {
		defaultBranch = g.sanitizeBranchName(defaultBranch)
		if defaultBranch == "" {
			lgr.Printf("[WARN] invalid default branch name, skipping branch comparison")
			return "", "", nil
		}
	}

	// check if we're on a different branch from the default
	if currentBranch == "" || defaultBranch == "" || currentBranch == defaultBranch {
		return "", "", nil
	}

	lgr.Printf("[DEBUG] no uncommitted changes, showing diff between %s and %s", defaultBranch, currentBranch)
	return g.ProcessGitDiff(false, currentBranch)
}

// getCommandOutputTrimmed executes the given command and returns trimmed output
func (g *gitDiffer) getCommandOutputTrimmed(cmd *exec.Cmd, errorContext string) (string, error) {
	output, err := g.executor.CommandOutput(cmd)
	if err != nil {
		lgr.Printf("[WARN] %s: %v", errorContext, err)
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ProcessGitDiff handles git diff extraction and returns a file path with the diff content
// isDiff indicates whether to get uncommitted changes, if false branchName is used for branch comparison
func (g *gitDiffer) ProcessGitDiff(isDiff bool, branchName string) (tempFilePath, diffDescription string, err error) {
	// verify git is available in the system
	if _, err := g.executor.LookPath("git"); err != nil {
		return "", "", fmt.Errorf("git executable not found: %w", err)
	}

	// generate a unique filename for the diff output
	timestamp := time.Now().Format("20060102-150405")
	tempFile := filepath.Join(g.tempDir, fmt.Sprintf("mpt-git-diff-%s.txt", timestamp))

	// generate diff based on the provided option
	var diffCmd *exec.Cmd

	switch {
	case isDiff:
		// get uncommitted changes
		diffCmd = g.executor.Command("git", "diff")
		diffDescription = "git diff (uncommitted changes)"

	case branchName != "":
		// try to find the default branch (main or master)
		defaultBranch := g.getDefaultBranch()
		// sanitize branch name to prevent command injection
		sanitizedBranch := g.sanitizeBranchName(branchName)
		if sanitizedBranch == "" {
			return "", "", fmt.Errorf("invalid branch name: %s", branchName)
		}
		// use separate args for diff command with branch comparison
		diffCmd = g.executor.Command("git", "diff", defaultBranch+"..."+sanitizedBranch) // #nosec G204 - sanitizeBranchName ensures the input is safe
		diffDescription = fmt.Sprintf("git diff between %s and %s branches", defaultBranch, sanitizedBranch)
	}

	// execute the git command and capture output
	diffOutput, err := g.executor.CommandOutput(diffCmd)
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

// getDefaultBranch tries to determine the default branch (main or master) for the repository.
// It first checks git config for init.defaultBranch, then looks for main, and finally falls back to master.
func (g *gitDiffer) getDefaultBranch() string {
	// try to get the default branch from git config
	cmd := g.executor.Command("git", "config", "--get", "init.defaultBranch")
	defaultBranch, err := g.getCommandOutputTrimmed(cmd, "failed to get default branch from git config")
	if err == nil && defaultBranch != "" && g.checkBranchExists(defaultBranch) {
		return defaultBranch
	}

	// check if main branch exists
	if g.checkBranchExists("main") {
		return "main"
	}

	// fallback to master
	return "master"
}

// checkBranchExists checks if a branch exists in the repository
func (g *gitDiffer) checkBranchExists(branch string) bool {
	cmd := g.executor.Command("git", "rev-parse", "--verify", branch)
	return g.executor.CommandRun(cmd) == nil
}

// getCurrentBranch returns the current git branch name.
// It first tries the modern git approach with --show-current flag,
// then falls back to using rev-parse --abbrev-ref HEAD for older git versions.
// Returns an empty string if the current branch cannot be determined.
func (g *gitDiffer) getCurrentBranch() string {
	// try modern git version first
	cmd := g.executor.Command("git", "branch", "--show-current")
	output, err := g.getCommandOutputTrimmed(cmd, "failed to get current git branch using --show-current")
	if err == nil {
		return output
	}

	// fallback to older git versions that don't have --show-current
	cmd = g.executor.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err = g.getCommandOutputTrimmed(cmd, "failed to get current git branch using rev-parse fallback")
	if err != nil {
		return ""
	}
	return output
}

// sanitizeBranchName ensures the branch name is a valid git reference
// Returns empty string if the name is invalid or potentially unsafe
func (g *gitDiffer) sanitizeBranchName(branch string) string {
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

	// at this point, the branch name is safe for command-line use
	// now verify it's a valid git reference by checking if it exists
	if !g.isValidGitRef(branch) {
		return ""
	}

	return branch
}

// isValidGitRef checks if a git reference is valid
// Note: This function should ONLY be called with sanitized input from sanitizeBranchName
func (g *gitDiffer) isValidGitRef(ref string) bool {
	// use array-based command construction to avoid shell injection
	// this ensures arguments are properly escaped
	cmdLocal := g.executor.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+ref)
	if g.executor.CommandRun(cmdLocal) == nil {
		return true
	}

	// also check if it's a valid remote branch, still using array-based construction
	cmdRemote := g.executor.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/"+ref)
	return g.executor.CommandRun(cmdRemote) == nil
}
