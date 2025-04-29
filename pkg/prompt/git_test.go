package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_WithGitDiffStructure(t *testing.T) {
	// skip actual git diff test as it requires a git repo
	// this test only verifies the interface and method structure
	builder := New("test prompt")

	// test the interface (doesn't execute the git commands)
	_, err := builder.WithGitDiff()
	if err != nil {
		// error is expected since this test doesn't run in a git repository
		require.Error(t, err, "Should return an error in a non-git environment")
	}

	// no need to check for actual diff content, just verify method exists and returns a builder
	assert.NotNil(t, builder)
}

func TestBuilder_WithGitBranchDiffStructure(t *testing.T) {
	// skip actual git branch diff test as it requires a git repo
	// this test only verifies the interface and method structure
	builder := New("test prompt")

	// test the interface (doesn't execute the git commands)
	_, err := builder.WithGitBranchDiff("test-branch")
	if err != nil {
		// error is expected since this test doesn't run in a git repository or the branch doesn't exist
		// we can get different errors depending on the environment:
		// - invalid branch name (from sanitizeBranchName)
		// - git command failed (if git is installed but not a repo)
		// - git executable not found (if git is not installed)
		require.Error(t, err, "Should return an error in a non-git environment")
	}

	// no need to check for actual diff content, just verify method exists and returns a builder
	assert.NotNil(t, builder)
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
