package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/prompt/mocks"
)

func TestPromptBuilder(t *testing.T) {
	t.Run("new builder", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}
		builder := New("base text", mockDiffer)
		prompt, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, "base text", prompt)
	})

	t.Run("with files", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFile, []byte("file content"), 0o644)
		require.NoError(t, err)

		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}
		builder := New("base text", mockDiffer).WithFiles([]string{testFile})
		prompt, err := builder.Build()
		require.NoError(t, err)
		assert.Contains(t, prompt, "base text")
		assert.Contains(t, prompt, "file content")
	})

	t.Run("with excludes", func(t *testing.T) {
		tempDir := t.TempDir()

		// create include file
		includeFile := filepath.Join(tempDir, "include.txt")
		err := os.WriteFile(includeFile, []byte("include content"), 0o644)
		require.NoError(t, err)

		// create exclude directory and file
		excludeDir := filepath.Join(tempDir, "exclude")
		err = os.MkdirAll(excludeDir, 0o755)
		require.NoError(t, err)

		excludeFile := filepath.Join(excludeDir, "exclude.txt")
		err = os.WriteFile(excludeFile, []byte("exclude content"), 0o644)
		require.NoError(t, err)

		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}
		builder := New("base text", mockDiffer).
			WithFiles([]string{
				filepath.Join(tempDir, "*.txt"),
				filepath.Join(tempDir, "**", "*.txt"),
			}).
			WithExcludes([]string{filepath.Join(tempDir, "exclude", "**")})

		prompt, err := builder.Build()
		require.NoError(t, err)
		assert.Contains(t, prompt, "base text")
		assert.Contains(t, prompt, "include content")
		assert.NotContains(t, prompt, "exclude content")
	})

	t.Run("combine with input", func(t *testing.T) {
		combined := CombineWithInput("", "input text")
		assert.Equal(t, "input text", combined)

		combined = CombineWithInput("base text", "input text")
		assert.Equal(t, "base text\ninput text", combined)
	})
}

func TestBuilder_WithGitDiff(t *testing.T) {
	t.Run("successful git diff", func(t *testing.T) {
		// create actual temp file for the test
		tempFile := filepath.Join(t.TempDir(), "test-diff.txt")
		err := os.WriteFile(tempFile, []byte("mock git diff content"), 0o600)
		require.NoError(t, err)

		cleanupCalled := false

		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				assert.True(t, isDiff)
				assert.Empty(t, branchName)
				return tempFile, "git diff (uncommitted changes)", nil
			},
			CleanupFunc: func() {
				cleanupCalled = true
			},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Contains(t, builder.baseText, "git diff (uncommitted changes)")
		assert.Contains(t, builder.files, tempFile)

		// verify cleanup is called on Build
		_, err = builder.Build()
		require.NoError(t, err)
		assert.True(t, cleanupCalled)

		// verify ProcessGitDiff was called once with correct params
		calls := mockDiffer.ProcessGitDiffCalls()
		assert.Len(t, calls, 1)
		assert.True(t, calls[0].IsDiff)
		assert.Empty(t, calls[0].BranchName)

		// verify Cleanup was called once
		assert.Len(t, mockDiffer.CleanupCalls(), 1)
	})

	t.Run("no local changes, branch diff used", func(t *testing.T) {
		// create actual temp file for the test
		tempFile := filepath.Join(t.TempDir(), "test-branch-diff.txt")
		err := os.WriteFile(tempFile, []byte("mock branch diff content"), 0o600)
		require.NoError(t, err)

		cleanupCalled := false
		callCount := 0

		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				callCount++
				if callCount == 1 {
					// first call - uncommitted changes check
					assert.True(t, isDiff)
					assert.Empty(t, branchName)
					return "", "", nil // no uncommitted changes
				}
				// should not be called again as TryBranchDiff should be used
				t.Fatalf("ProcessGitDiff called too many times")
				return "", "", nil
			},
			TryBranchDiffFunc: func() (string, string, error) {
				// branch diff
				return tempFile, "git diff between master and feature-branch branches", nil
			},
			CleanupFunc: func() {
				cleanupCalled = true
			},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Contains(t, builder.baseText, "git diff between master and feature-branch branches")
		assert.Contains(t, builder.files, tempFile)

		// verify cleanup is called on Build
		_, err = builder.Build()
		require.NoError(t, err)
		assert.True(t, cleanupCalled)

		// verify ProcessGitDiff was called once for uncommitted changes
		// and TryBranchDiff was called once
		processCalls := mockDiffer.ProcessGitDiffCalls()
		assert.Len(t, processCalls, 1)
		assert.True(t, processCalls[0].IsDiff)
		assert.Empty(t, processCalls[0].BranchName)

		branchCalls := mockDiffer.TryBranchDiffCalls()
		assert.Len(t, branchCalls, 1)

		// verify Cleanup was called once
		assert.Len(t, mockDiffer.CleanupCalls(), 1)
	})
}

func TestBuilder_WithGitBranchDiff(t *testing.T) {
	t.Run("successful branch diff", func(t *testing.T) {
		// create actual temp file for the test
		tempFile := filepath.Join(t.TempDir(), "test-branch-diff.txt")
		err := os.WriteFile(tempFile, []byte("mock branch diff content"), 0o600)
		require.NoError(t, err)

		cleanupCalled := false

		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				assert.False(t, isDiff)
				assert.Equal(t, "feature-branch", branchName)
				return tempFile, "git diff between master and feature-branch branches", nil
			},
			CleanupFunc: func() {
				cleanupCalled = true
			},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitBranchDiff("feature-branch")
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Contains(t, builder.baseText, "git diff between master and feature-branch branches")
		assert.Contains(t, builder.files, tempFile)

		// verify cleanup is called on Build
		_, err = builder.Build()
		require.NoError(t, err)
		assert.True(t, cleanupCalled)

		// verify ProcessGitDiff was called once with correct params
		calls := mockDiffer.ProcessGitDiffCalls()
		assert.Len(t, calls, 1)
		assert.False(t, calls[0].IsDiff)
		assert.Equal(t, "feature-branch", calls[0].BranchName)

		// verify Cleanup was called once
		assert.Len(t, mockDiffer.CleanupCalls(), 1)
	})

	t.Run("error from ProcessGitDiff", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				return "", "", assert.AnError
			},
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitBranchDiff("feature-branch")
		require.Error(t, err)
		assert.Equal(t, builder, result) // builder is still returned
	})

	t.Run("empty diff result", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				return "", "", nil // no diff
			},
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		originalText := builder.baseText
		result, err := builder.WithGitBranchDiff("feature-branch")
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Equal(t, originalText, builder.baseText) // baseText should not change
		assert.Empty(t, builder.files)                  // no files added
	})
}

func TestBuilder_WithMaxFileSize(t *testing.T) {
	mockDiffer := &mocks.GitDiffProcessorMock{
		CleanupFunc: func() {},
	}
	builder := New("test prompt", mockDiffer)

	result := builder.WithMaxFileSize(1024 * 1024) // 1MB
	assert.Equal(t, builder, result)
	assert.Equal(t, int64(1024*1024), builder.maxFileSize)
}

func TestBuilder_WithForce(t *testing.T) {
	mockDiffer := &mocks.GitDiffProcessorMock{
		CleanupFunc: func() {},
	}
	builder := New("test prompt", mockDiffer)

	result := builder.WithForce(true)
	assert.Equal(t, builder, result)
	assert.True(t, builder.force)

	result = builder.WithForce(false)
	assert.Equal(t, builder, result)
	assert.False(t, builder.force)
}

func TestBuilder_WithGitDiff_ErrorCases(t *testing.T) {
	t.Run("error from ProcessGitDiff", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				return "", "", assert.AnError
			},
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitDiff()
		require.Error(t, err)
		assert.Equal(t, builder, result) // builder is still returned
	})

	t.Run("error from TryBranchDiff", func(t *testing.T) {
		callCount := 0
		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				callCount++
				// first call - no uncommitted changes
				return "", "", nil
			},
			TryBranchDiffFunc: func() (string, string, error) {
				return "", "", assert.AnError
			},
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		result, err := builder.WithGitDiff()
		require.Error(t, err)
		assert.Equal(t, builder, result)
		assert.Equal(t, 1, callCount)
	})

	t.Run("no diff at all", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			ProcessGitDiffFunc: func(isDiff bool, branchName string) (string, string, error) {
				return "", "", nil // no uncommitted changes
			},
			TryBranchDiffFunc: func() (string, string, error) {
				return "", "", nil // no branch diff
			},
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		originalText := builder.baseText
		result, err := builder.WithGitDiff()
		require.NoError(t, err)
		assert.Equal(t, builder, result)
		assert.Equal(t, originalText, builder.baseText) // baseText should not change
		assert.Empty(t, builder.files)                  // no files added
	})
}

func TestBuilder_Build_ErrorCase(t *testing.T) {
	t.Run("no files matched pattern", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}

		builder := New("test prompt", mockDiffer)
		// add a pattern that won't match any files
		tempDir := t.TempDir()
		builder.WithFiles([]string{filepath.Join(tempDir, "*.nonexistent")})

		// this should now error because no files matched
		_, err := builder.Build()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no files matched the provided patterns")
	})
}

func TestBuilder_AddGitDiffFile(t *testing.T) {
	t.Run("with existing base text", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}

		builder := New("original prompt", mockDiffer)
		result := builder.addGitDiffFile("/tmp/diff.txt", "git diff description")

		assert.Equal(t, builder, result)
		assert.Contains(t, builder.baseText, "I'm providing git diff description for context.")
		assert.Contains(t, builder.baseText, "original prompt")
		assert.Contains(t, builder.files, "/tmp/diff.txt")
	})

	t.Run("with empty base text", func(t *testing.T) {
		mockDiffer := &mocks.GitDiffProcessorMock{
			CleanupFunc: func() {},
		}

		builder := New("", mockDiffer)
		result := builder.addGitDiffFile("/tmp/diff.txt", "git diff description")

		assert.Equal(t, builder, result)
		assert.Empty(t, builder.baseText) // should remain empty
		assert.Contains(t, builder.files, "/tmp/diff.txt")
	})
}
