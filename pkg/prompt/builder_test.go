package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder(t *testing.T) {
	t.Run("new builder", func(t *testing.T) {
		builder := New("base text")
		prompt, err := builder.Build()
		require.NoError(t, err)
		assert.Equal(t, "base text", prompt)
	})

	t.Run("with files", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(testFile, []byte("file content"), 0o644)
		require.NoError(t, err)

		builder := New("base text").WithFiles([]string{testFile})
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

		builder := New("base text").
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
