package prompt

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/mpt/pkg/prompt/mocks"
)

func TestBuilder_WithGitDiff(t *testing.T) {
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
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte{}, nil
			},
		}

		executor = mockExec
		builder := New("test prompt")

		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Empty(t, gitCleanupFiles)
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
				return exec.Command("echo", "test")
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

	t.Run("main exists", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil // success for any branch
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
