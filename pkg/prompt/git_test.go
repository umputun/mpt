package prompt

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/mpt/pkg/prompt/mocks"
)

func TestBuilder_WithGitDiff(t *testing.T) { //nolint:gocyclo // test complexity due to mocking
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful git diff", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("mock diff output"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Len(t, gitCleanupFiles, 1)
		assert.Contains(t, builder.baseText, "git diff (uncommitted changes)")
	})

	t.Run("no local changes, branch diff used", func(t *testing.T) {
		// reset gitCleanupFiles for this test
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				// store the actual git args in a way we can check them
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// check the actual git command that was executed
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 2 && args[1] == "diff" && len(args) == 2 {
						// first diff (uncommitted changes) - none
						return []byte(""), nil
					}
					if len(args) >= 3 && args[1] == "branch" && args[2] == "--show-current" {
						// current branch
						return []byte("feature-branch"), nil
					}
					if len(args) >= 4 && args[1] == "config" && args[2] == "--get" && args[3] == "init.defaultBranch" {
						// no default branch in config
						return []byte(""), errors.New("no config")
					}
					if len(args) >= 3 && args[1] == "diff" && strings.HasPrefix(args[2], "master...") {
						// branch diff - this is the second diff command
						return []byte("branch diff output"), nil
					}
				}
				return []byte(""), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 4 && args[1] == "rev-parse" && args[2] == "--verify" {
						// branch exists check - return error for main, success for master
						if args[3] == "main" {
							return errors.New("main branch not found")
						}
						return nil
					}
				}
				return nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Len(t, gitCleanupFiles, 1)
		assert.Contains(t, builder.baseText, "git diff between master and feature-branch branches")
	})

	t.Run("no local changes, on main branch", func(t *testing.T) {
		// reset gitCleanupFiles for this test
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 2 && args[1] == "diff" && len(args) == 2 {
						// uncommitted changes - none
						return []byte(""), nil
					}
					if len(args) >= 3 && args[1] == "branch" && args[2] == "--show-current" {
						// current branch is main
						return []byte("main"), nil
					}
					if len(args) >= 4 && args[1] == "config" && args[2] == "--get" && args[3] == "init.defaultBranch" {
						return []byte("main"), nil
					}
				}
				return []byte(""), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 3 && args[1] == "rev-parse" && args[2] == "--verify" {
						// branch exists check
						return nil
					}
				}
				return nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Empty(t, gitCleanupFiles)
		assert.Equal(t, "test prompt", builder.baseText) // no git diff added
	})

	t.Run("git not found", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "", errors.New("git executable not found")
			},
		}

		executor = mockExec
		builder := New("test prompt")

		_, err := builder.WithGitDiff()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git executable not found")
	})

	t.Run("empty diff output", func(t *testing.T) {
		// reset gitCleanupFiles for this test
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// all commands return empty output in this test
				return []byte{}, nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// all commands succeed
				return nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Empty(t, gitCleanupFiles)
	})

	t.Run("invalid branch names sanitization", func(t *testing.T) {
		// reset gitCleanupFiles for this test
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 2 && args[1] == "diff" && len(args) == 2 {
						// uncommitted changes - none
						return []byte(""), nil
					}
					if len(args) >= 3 && args[1] == "branch" && args[2] == "--show-current" {
						// current branch with invalid characters
						return []byte("invalid;branch|name"), nil
					}
					if len(args) >= 4 && args[1] == "config" && args[2] == "--get" && args[3] == "init.defaultBranch" {
						return []byte("main"), nil
					}
				}
				return []byte(""), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// branch validation will fail for invalid names
				return errors.New("invalid branch")
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Empty(t, gitCleanupFiles)
		assert.Equal(t, "test prompt", builder.baseText) // no git diff added due to invalid branch name
	})
}

func TestBuilder_WithGitBranchDiff(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful branch diff", func(t *testing.T) {
		// reset gitCleanupFiles for this test
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("mock branch diff output"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitBranchDiff("feature-branch")
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Len(t, gitCleanupFiles, 1)
		assert.Contains(t, builder.baseText, "git diff between")
	})

	t.Run("invalid branch name", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// default branch config
				if cmd.Path == "git" && len(cmd.Args) >= 4 && cmd.Args[1] == "config" && cmd.Args[2] == "--get" && cmd.Args[3] == "init.defaultBranch" {
					return []byte(""), errors.New("no config")
				}
				return []byte(""), errors.New("command failed")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		builder := New("test prompt")

		_, err := builder.WithGitBranchDiff("invalid;branch")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid branch name")
	})
}

func TestGetDefaultBranch(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("default branch from config", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 4 && args[1] == "config" && args[2] == "--get" && args[3] == "init.defaultBranch" {
						return []byte("develop\n"), nil
					}
				}
				return []byte(""), errors.New("unexpected command")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				if cmd.Path == "git" {
					args := cmd.Args
					if len(args) >= 4 && args[1] == "rev-parse" && args[2] == "--verify" && args[3] == "develop" {
						return nil // branch exists
					}
				}
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		branch := getDefaultBranch()
		assert.Equal(t, "develop", branch)
	})

	t.Run("main exists", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				cmd := exec.Command("echo", "test")
				cmd.Path = name
				cmd.Args = append([]string{name}, args...)
				return cmd
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// config command fails
				return []byte(""), errors.New("no config")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// check if we're verifying the "main" branch
				if cmd.Path == "git" && len(cmd.Args) >= 4 && cmd.Args[1] == "rev-parse" && cmd.Args[2] == "--verify" && cmd.Args[3] == "main" {
					return nil // main exists
				}
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		branch := getDefaultBranch()
		assert.Equal(t, "main", branch)
	})

	t.Run("fallback to master", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// config command fails
				return []byte(""), errors.New("no config")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				callCount++
				if callCount == 1 {
					return errors.New("branch not found") // main doesn't exist
				}
				return nil // master exists
			},
		}

		executor = mockExec
		branch := getDefaultBranch()
		assert.Equal(t, "master", branch)
		assert.Equal(t, 1, callCount)
	})
}

func TestCheckBranchExists(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("branch exists", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil // success
			},
		}

		executor = mockExec
		exists := checkBranchExists("main")
		assert.True(t, exists)
	})

	t.Run("branch doesn't exist", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		exists := checkBranchExists("non-existent")
		assert.False(t, exists)
	})
}

func TestSanitizeBranchName(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("valid branch name", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil // success - branch exists
			},
		}

		executor = mockExec

		result := sanitizeBranchName("feature-branch")
		assert.Equal(t, "feature-branch", result)
	})

	t.Run("branch with unsafe characters", func(t *testing.T) {
		result := sanitizeBranchName("branch;with;semicolons")
		assert.Empty(t, result)
	})

	t.Run("branch with invalid characters", func(t *testing.T) {
		result := sanitizeBranchName("branch★with★stars")
		assert.Empty(t, result)
	})
}

func TestGetCommandOutputTrimmed(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful command", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("  output with spaces  \n"), nil
			},
		}

		executor = mockExec
		cmd := exec.Command("test", "command")
		result, err := getCommandOutputTrimmed(cmd, "test context")
		require.NoError(t, err)
		assert.Equal(t, "output with spaces", result)
	})

	t.Run("command error", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return nil, errors.New("command failed")
			},
		}

		executor = mockExec
		cmd := exec.Command("test", "command")
		result, err := getCommandOutputTrimmed(cmd, "test context")
		require.Error(t, err)
		assert.Empty(t, result)
	})
}

func TestGetCurrentBranch(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("modern git version", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("feature-branch\n"), nil
			},
		}

		executor = mockExec
		branch := getCurrentBranch()
		assert.Equal(t, "feature-branch", branch)
	})

	t.Run("older git version fallback", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				callCount++
				if callCount == 1 {
					// first call fails (modern git version not supported)
					return []byte(""), errors.New("unrecognized option")
				}
				// second call succeeds (older git version)
				return []byte("legacy-branch\n"), nil
			},
		}

		executor = mockExec
		branch := getCurrentBranch()
		assert.Equal(t, "legacy-branch", branch)
		assert.Equal(t, 2, callCount)
	})

	t.Run("error handling", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte(""), errors.New("git error")
			},
		}

		executor = mockExec
		branch := getCurrentBranch()
		assert.Empty(t, branch)
	})
}

func TestIsValidGitRef(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("local branch exists", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				callCount++
				if callCount == 1 {
					return nil // first call succeeds (local branch)
				}
				return errors.New("not called") // should not reach here
			},
		}

		executor = mockExec

		result := isValidGitRef("master")
		assert.True(t, result)
		assert.Equal(t, 1, callCount, "Should have checked only local branch")
	})

	t.Run("remote branch exists", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				callCount++
				if callCount == 1 {
					return errors.New("not a local branch") // first call fails (no local branch)
				}
				return nil // second call succeeds (remote branch)
			},
		}

		executor = mockExec

		result := isValidGitRef("origin/master")
		assert.True(t, result)
		assert.Equal(t, 2, callCount, "Should have checked both local and remote branch")
	})

	t.Run("branch doesn't exist", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				callCount++
				return errors.New("branch not found") // both calls fail
			},
		}

		executor = mockExec

		result := isValidGitRef("non-existent-branch")
		assert.False(t, result)
		assert.Equal(t, 2, callCount, "Should have checked both local and remote branch")
	})
}

func TestProcessGitDiff(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful diff", func(t *testing.T) {
		// clear the gitCleanupFiles for tests
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("mock diff output"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil
			},
		}

		executor = mockExec
		tempFile, desc, err := processGitDiff(true, "")
		require.NoError(t, err)
		assert.NotEmpty(t, tempFile)
		assert.Equal(t, "git diff (uncommitted changes)", desc)
	})

	t.Run("empty diff", func(t *testing.T) {
		// clear the gitCleanupFiles for tests
		gitCleanupFiles = []string{}

		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte{}, nil
			},
		}

		executor = mockExec
		tempFile, desc, err := processGitDiff(true, "")
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
	})
}

func TestCleanupGitDiffFiles(t *testing.T) {
	// create a fake temp file to test cleanup
	dir := t.TempDir()
	tempFile := filepath.Join(dir, "mpt-git-diff-test.txt")
	err := os.WriteFile(tempFile, []byte("test content"), 0o600)
	require.NoError(t, err)

	// add to cleanup list
	gitCleanupFiles = append(gitCleanupFiles, tempFile)

	// test cleanup
	CleanupGitDiffFiles()

	// verify the file is removed
	_, err = os.Stat(tempFile)
	assert.True(t, os.IsNotExist(err), "File should have been removed")

	// verify cleanup list is empty
	assert.Empty(t, gitCleanupFiles, "Cleanup list should be empty")
}
