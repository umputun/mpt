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

func TestGitDiffer_GetDefaultBranch(t *testing.T) {
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
		differ := newGitDiffer()
		branch := differ.getDefaultBranch()
		assert.Equal(t, "develop", branch)

		// verify Command was called twice (once for config, once for creating output command)
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 2)
		assert.Equal(t, "git", commandCalls[0].Name)
		assert.Equal(t, []string{"config", "--get", "init.defaultBranch"}, commandCalls[0].Args)

		// verify CommandOutput was called once for config
		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 1)

		// verify CommandRun was called once for branch verification
		runCalls := mockExec.CommandRunCalls()
		assert.Len(t, runCalls, 1)
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
		differ := newGitDiffer()
		branch := differ.getDefaultBranch()
		assert.Equal(t, "main", branch)

		// verify CommandRun was called once for main branch check
		runCalls := mockExec.CommandRunCalls()
		assert.Len(t, runCalls, 1)
		assert.Equal(t, "git", runCalls[0].Cmd.Path)
		assert.Equal(t, []string{"git", "rev-parse", "--verify", "main"}, runCalls[0].Cmd.Args)
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
		differ := newGitDiffer()
		branch := differ.getDefaultBranch()
		assert.Equal(t, "master", branch)
		assert.Equal(t, 1, callCount)
	})
}

func TestGitDiffer_CheckBranchExists(t *testing.T) {
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
		differ := newGitDiffer()
		exists := differ.checkBranchExists("main")
		assert.True(t, exists)

		// verify Command and CommandRun were called once
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 1)
		assert.Equal(t, "git", commandCalls[0].Name)
		assert.Equal(t, []string{"rev-parse", "--verify", "main"}, commandCalls[0].Args)

		runCalls := mockExec.CommandRunCalls()
		assert.Len(t, runCalls, 1)
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
		differ := newGitDiffer()
		exists := differ.checkBranchExists("non-existent")
		assert.False(t, exists)

		// verify calls
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 1)
		assert.Equal(t, "git", commandCalls[0].Name)
		assert.Equal(t, []string{"rev-parse", "--verify", "non-existent"}, commandCalls[0].Args)

		runCalls := mockExec.CommandRunCalls()
		assert.Len(t, runCalls, 1)
	})
}

func TestGitDiffer_SanitizeBranchName(t *testing.T) {
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
		differ := newGitDiffer()

		result := differ.sanitizeBranchName("feature-branch")
		assert.Equal(t, "feature-branch", result)
	})

	t.Run("branch with unsafe characters", func(t *testing.T) {
		differ := newGitDiffer()
		result := differ.sanitizeBranchName("branch;with;semicolons")
		assert.Empty(t, result)
	})

	t.Run("branch with invalid characters", func(t *testing.T) {
		differ := newGitDiffer()
		result := differ.sanitizeBranchName("branch★with★stars")
		assert.Empty(t, result)
	})
}

func TestGitDiffer_GetCommandOutputTrimmed(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful command", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return []byte("  output with spaces  \n"), nil
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		cmd := exec.Command("test", "command")
		result, err := differ.getCommandOutputTrimmed(cmd, "test context")
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
		differ := newGitDiffer()
		cmd := exec.Command("test", "command")
		result, err := differ.getCommandOutputTrimmed(cmd, "test context")
		require.Error(t, err)
		assert.Empty(t, result)
	})
}

func TestGitDiffer_GetCurrentBranch(t *testing.T) {
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
		differ := newGitDiffer()
		branch := differ.getCurrentBranch()
		assert.Equal(t, "feature-branch", branch)

		// verify only one command was created and output called
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 1)
		assert.Equal(t, "git", commandCalls[0].Name)
		assert.Equal(t, []string{"branch", "--show-current"}, commandCalls[0].Args)

		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 1)
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
		differ := newGitDiffer()
		branch := differ.getCurrentBranch()
		assert.Equal(t, "legacy-branch", branch)
		assert.Equal(t, 2, callCount)

		// verify two commands were created
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 2)
		assert.Equal(t, []string{"branch", "--show-current"}, commandCalls[0].Args)
		assert.Equal(t, []string{"rev-parse", "--abbrev-ref", "HEAD"}, commandCalls[1].Args)

		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 2)
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
		differ := newGitDiffer()
		branch := differ.getCurrentBranch()
		assert.Empty(t, branch)

		// verify both commands were attempted
		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 2)
		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 2)
	})
}

func TestGitDiffer_IsValidGitRef(t *testing.T) {
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
		differ := newGitDiffer()

		result := differ.isValidGitRef("master")
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
		differ := newGitDiffer()

		result := differ.isValidGitRef("origin/master")
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
		differ := newGitDiffer()

		result := differ.isValidGitRef("non-existent-branch")
		assert.False(t, result)
		assert.Equal(t, 2, callCount, "Should have checked both local and remote branch")
	})
}

func TestGitDiffer_ProcessGitDiff(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful diff", func(t *testing.T) {
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
		differ := newGitDiffer()
		tempFile, desc, err := differ.ProcessGitDiff(true, "")
		require.NoError(t, err)
		assert.NotEmpty(t, tempFile)
		assert.Equal(t, "git diff (uncommitted changes)", desc)
		assert.Contains(t, tempFile, differ.tempDir)
		assert.FileExists(t, tempFile)

		// verify calls
		lookPathCalls := mockExec.LookPathCalls()
		assert.Len(t, lookPathCalls, 1)
		assert.Equal(t, "git", lookPathCalls[0].File)

		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 1)
		assert.Equal(t, "git", commandCalls[0].Name)
		assert.Equal(t, []string{"diff"}, commandCalls[0].Args)

		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 1)
	})

	t.Run("empty diff", func(t *testing.T) {
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
		differ := newGitDiffer()
		tempFile, desc, err := differ.ProcessGitDiff(true, "")
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
		// no temp files created for empty diff

		// verify calls were made even for empty diff
		lookPathCalls := mockExec.LookPathCalls()
		assert.Len(t, lookPathCalls, 1)

		commandCalls := mockExec.CommandCalls()
		assert.Len(t, commandCalls, 1)

		outputCalls := mockExec.CommandOutputCalls()
		assert.Len(t, outputCalls, 1)
	})
}

func TestGitDiffer_Cleanup(t *testing.T) {
	t.Run("cleanup removes temp directory", func(t *testing.T) {
		// create a gitDiffer which will create its own temp dir
		differ := newGitDiffer()
		tempDir := differ.tempDir

		// verify the directory exists
		_, err := os.Stat(tempDir)
		require.NoError(t, err, "Temp directory should exist")

		// create some test files in the temp directory
		testFile1 := filepath.Join(tempDir, "test1.txt")
		testFile2 := filepath.Join(tempDir, "test2.txt")
		require.NoError(t, os.WriteFile(testFile1, []byte("test1"), 0o600))
		require.NoError(t, os.WriteFile(testFile2, []byte("test2"), 0o600))

		// test cleanup
		differ.Cleanup()

		// verify the entire directory is removed
		_, err = os.Stat(tempDir)
		assert.True(t, os.IsNotExist(err), "Temp directory should have been removed")
	})

	t.Run("cleanup skips system temp dir", func(t *testing.T) {
		// create a gitDiffer with system temp dir (fallback case)
		differ := &gitDiffer{
			executor: executor,
			tempDir:  os.TempDir(),
		}

		// create a test file in system temp
		testFile := filepath.Join(os.TempDir(), "mpt-test-should-remain.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o600))
		defer os.Remove(testFile) // cleanup after test

		// test cleanup
		differ.Cleanup()

		// verify the file still exists (cleanup should skip system temp)
		_, err := os.Stat(testFile)
		assert.NoError(t, err, "File in system temp should not be removed")
	})
}

func TestGitDiffer_TryBranchDiff(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("successful branch diff", func(t *testing.T) {
		callCount := 0
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
				callCount++
				switch callCount {
				case 1:
					// getCurrentBranch
					return []byte("feature-branch\n"), nil
				case 2:
					// getDefaultBranch - config check
					return []byte(""), errors.New("no config")
				default:
					// for ProcessGitDiff, return empty since it writes to a file
					return []byte("diff output"), nil
				}
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// checkBranchExists for "main"
				if len(cmd.Args) >= 4 && cmd.Args[3] == "main" {
					return nil
				}
				// isValidGitRef for sanitizeBranchName
				if len(cmd.Args) >= 5 && cmd.Args[1] == "show-ref" {
					return nil
				}
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		tempFile, desc, err := differ.TryBranchDiff()
		require.NoError(t, err)
		assert.NotEmpty(t, tempFile)
		assert.Contains(t, desc, "git diff between main and feature-branch branches")
		assert.FileExists(t, tempFile)
	})

	t.Run("same branch as default", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// both getCurrentBranch and getDefaultBranch return "main"
				return []byte("main\n"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return nil
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		tempFile, desc, err := differ.TryBranchDiff()
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
	})

	t.Run("no current branch", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// getCurrentBranch fails
				return []byte(""), errors.New("not in git repo")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return errors.New("not in git repo")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		tempFile, desc, err := differ.TryBranchDiff()
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
	})

	t.Run("invalid current branch name", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// getCurrentBranch returns invalid name
				return []byte("branch;with;semicolons\n"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				return errors.New("invalid branch")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		tempFile, desc, err := differ.TryBranchDiff()
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
	})

	t.Run("invalid default branch name", func(t *testing.T) {
		callCount := 0
		mockExec := &mocks.GitExecutorMock{
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				callCount++
				if callCount == 1 {
					// getCurrentBranch
					return []byte("feature-branch\n"), nil
				}
				// getDefaultBranch returns invalid name
				return []byte("branch$with$dollar\n"), nil
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// isValidGitRef for feature-branch succeeds
				if len(cmd.Args) >= 5 && strings.Contains(cmd.Args[4], "feature-branch") {
					return nil
				}
				return errors.New("invalid branch")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		tempFile, desc, err := differ.TryBranchDiff()
		require.NoError(t, err)
		assert.Empty(t, tempFile)
		assert.Empty(t, desc)
	})
}

func TestGitDiffer_ProcessGitDiff_ErrorPaths(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	t.Run("git not found", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "", errors.New("git not found")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		_, _, err := differ.ProcessGitDiff(true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git executable not found")
	})

	t.Run("git command fails", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				return nil, errors.New("git command failed")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		_, _, err := differ.ProcessGitDiff(true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git command failed")
	})

	t.Run("invalid branch name for branch diff", func(t *testing.T) {
		mockExec := &mocks.GitExecutorMock{
			LookPathFunc: func(file string) (string, error) {
				return "/usr/bin/git", nil
			},
			CommandFunc: func(name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "test")
			},
			CommandOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
				// getDefaultBranch
				return []byte(""), errors.New("no config")
			},
			CommandRunFunc: func(cmd *exec.Cmd) error {
				// checkBranchExists for main
				if len(cmd.Args) >= 4 && cmd.Args[3] == "main" {
					return nil
				}
				return errors.New("branch not found")
			},
		}

		executor = mockExec
		differ := newGitDiffer()
		// try with invalid branch name
		_, _, err := differ.ProcessGitDiff(false, "branch;with;injection")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid branch name")
	})

	t.Run("write file error", func(t *testing.T) {
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
		}

		executor = mockExec
		// create differ with invalid temp directory
		differ := &gitDiffer{
			executor: mockExec,
			tempDir:  "/invalid/path/that/does/not/exist",
		}
		_, _, err := differ.ProcessGitDiff(true, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write git diff to temporary file")
	})
}

func TestGitDiffer_SanitizeBranchName_EdgeCases(t *testing.T) {
	origExecutor := executor
	defer func() { executor = origExecutor }()

	testCases := []struct {
		name     string
		branch   string
		expected string
	}{
		{
			name:     "command injection attempt",
			branch:   "branch$(rm -rf /)",
			expected: "",
		},
		{
			name:     "pipe character",
			branch:   "branch|echo hack",
			expected: "",
		},
		{
			name:     "backticks",
			branch:   "branch`whoami`",
			expected: "",
		},
		{
			name:     "quotes",
			branch:   "branch\"with\"quotes",
			expected: "",
		},
		{
			name:     "valid branch with slash",
			branch:   "feature/new-feature",
			expected: "feature/new-feature",
		},
		{
			name:     "valid branch with dots",
			branch:   "release-1.2.3",
			expected: "release-1.2.3",
		},
		{
			name:     "branch exists but not valid git ref",
			branch:   "validchars",
			expected: "",
		},
	}

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
		CommandRunFunc: func(cmd *exec.Cmd) error {
			// only feature/new-feature and release-1.2.3 are valid git refs
			if len(cmd.Args) >= 5 && cmd.Args[1] == "show-ref" {
				// extract the ref from the last argument (refs/heads/branch or refs/remotes/branch)
				lastArg := cmd.Args[len(cmd.Args)-1]
				if lastArg == "refs/heads/feature/new-feature" || lastArg == "refs/remotes/feature/new-feature" ||
					lastArg == "refs/heads/release-1.2.3" || lastArg == "refs/remotes/release-1.2.3" {
					return nil
				}
			}
			return errors.New("invalid ref")
		},
	}

	executor = mockExec
	differ := newGitDiffer()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := differ.sanitizeBranchName(tc.branch)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGitDiffer_Cleanup_ErrorHandling(t *testing.T) {
	t.Run("cleanup with permission error", func(t *testing.T) {
		// create a temporary directory
		tempDir, err := os.MkdirTemp("", "mpt-test-cleanup-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir) // ensure cleanup even if test fails

		// create a file in the directory
		testFile := filepath.Join(tempDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o600))

		// remove write permission from directory (on Unix-like systems)
		// this will cause RemoveAll to fail
		if err := os.Chmod(tempDir, 0o500); err == nil {
			// create differ with our test directory
			differ := &gitDiffer{
				executor: executor,
				tempDir:  tempDir,
			}

			// cleanup should handle the error gracefully
			differ.Cleanup()

			// restore permissions for cleanup
			os.Chmod(tempDir, 0o700)

			// verify directory still exists (cleanup failed)
			_, err = os.Stat(tempDir)
			assert.NoError(t, err, "Directory should still exist after failed cleanup")
		}
	})
}
