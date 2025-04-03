package files

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecursivePattern(t *testing.T) {
	testCases := []struct {
		pattern  string
		basePath string
		filter   string
	}{
		{"pkg/...", "pkg", ""},
		{"cmd/.../*.go", "cmd", "*.go"},
		{"./...", ".", ""},
		{"./.../cmd/*.go", ".", "cmd/*.go"},
		{"src/.../*.{go,js}", "src", "*.{go,js}"},
		{"/absolute/path/...", "/absolute/path", ""},
		{"/absolute/path/.../*.go", "/absolute/path", "*.go"},
		{"relative/path/.../*.{js,ts}", "relative/path", "*.{js,ts}"},
	}

	for _, tc := range testCases {
		t.Run(tc.pattern, func(t *testing.T) {
			basePath, filter := parseRecursivePattern(tc.pattern)
			assert.Equal(t, tc.basePath, basePath)
			assert.Equal(t, tc.filter, filter)
		})
	}
}

func TestGetFileHeader(t *testing.T) {
	testCases := []struct {
		filePath    string
		expectedFmt string
	}{
		{"test.go", "// file: %s\n"},
		{"test.py", "# file: %s\n"},
		{"test.html", "<!-- file: %s -->\n"},
		{"test.css", "/* file: %s */\n"},
		{"test.sql", "-- file: %s\n"},
		{"test.clj", ";; file: %s\n"},
		{"test.hs", "-- file: %s\n"},
		{"test.ps1", "# file: %s\n"},
		{"test.bat", ":: file: %s\n"},
		{"test.f90", "! file: %s\n"},
		{"test.unknown", "// file: %s\n"}, // default
		// additional file types
		{"test.js", "// file: %s\n"},
		{"test.jsx", "// file: %s\n"},
		{"test.ts", "// file: %s\n"},
		{"test.tsx", "// file: %s\n"},
		{"test.java", "// file: %s\n"},
		{"test.c", "// file: %s\n"},
		{"test.cpp", "// file: %s\n"},
		{"test.php", "// file: %s\n"},
		{"test.swift", "// file: %s\n"},
		{"test.rs", "// file: %s\n"},
		{"test.yaml", "# file: %s\n"},
		{"test.yml", "# file: %s\n"},
		{"test.sh", "# file: %s\n"},
		{"test.xml", "<!-- file: %s -->\n"},
		{"test.svg", "<!-- file: %s -->\n"},
		{"test.scss", "/* file: %s */\n"},
		{"test.less", "/* file: %s */\n"},
		{"test.sass", "/* file: %s */\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			expected := fmt.Sprintf(tc.expectedFmt, tc.filePath)
			actual := getFileHeader(tc.filePath)
			assert.Equal(t, expected, actual)
		})
	}
}

// Test various scenarios with the LoadContent function
func TestLoadContent(t *testing.T) {
	// setup test directory structure
	wd, err := os.Getwd()
	require.NoError(t, err)
	testDataDir := filepath.Join(wd, "testdata")

	// test with specific files
	t.Run("specific_files", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "test1.go"),
			filepath.Join(testDataDir, "test2.txt"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "func TestFunc1")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "// file:") // Go files use // comments
	})

	// test with standard glob patterns
	t.Run("standard_glob", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "func TestFunc1")
		assert.Contains(t, result, "// file:")
		assert.NotContains(t, result, "This is a text file for testing")
	})

	// test with directory path (should include all files recursively)
	t.Run("directory_recursive", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "nested"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is another text file for testing")
	})

	// test with Go-style recursive pattern for .go files
	t.Run("go_style_recursive_go_files", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		goStylePath := testDataDir + "/.../*.go"
		result, err := LoadContent([]string{goStylePath}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "This is another text file for testing")
	})

	// test with Go-style recursive pattern for all files
	t.Run("go_style_recursive_all", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		goStylePath := testDataDir + "/..."
		result, err := LoadContent([]string{goStylePath}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "This is another text file for testing")
	})

	// test with bash-style recursive pattern for .go files
	t.Run("bash_style_recursive_go_files", func(t *testing.T) {
		// change directory to testDataDir to make bash-style patterns work
		oldWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(testDataDir)
		require.NoError(t, err)
		defer os.Chdir(oldWd) // restore original working directory

		result, err := LoadContent([]string{
			"**/*.go", // use relative pattern
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "This is another text file for testing")
	})

	// test with bash-style recursive pattern for nested directory
	t.Run("bash_style_nested_directory", func(t *testing.T) {
		// change directory to testDataDir to make bash-style patterns work
		oldWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(testDataDir)
		require.NoError(t, err)
		defer os.Chdir(oldWd) // restore original working directory

		result, err := LoadContent([]string{
			"nested/**/*.go", // use relative pattern
		}, nil)
		require.NoError(t, err)

		assert.NotContains(t, result, "package testdata") // should not include root level files
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
	})

	// test with bash-style pattern with multiple extensions
	t.Run("bash_style_multiple_extensions", func(t *testing.T) {
		// change directory to testDataDir to make bash-style patterns work
		oldWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(testDataDir)
		require.NoError(t, err)
		defer os.Chdir(oldWd) // restore original working directory

		// note: doublestar doesn't support {go,txt} style patterns directly,
		// so we need to use multiple patterns
		result, err := LoadContent([]string{
			"**/*.go",  // use relative pattern
			"**/*.txt", // use relative pattern
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "This is another text file for testing")
	})

	// test with empty pattern list
	t.Run("empty_pattern", func(t *testing.T) {
		result, err := LoadContent([]string{}, nil)
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	// test with non-existent pattern
	t.Run("non_existent_pattern", func(t *testing.T) {
		_, err := LoadContent([]string{"non-existent-pattern-*.xyz"}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no files matched the provided patterns")
	})

	// test with invalid directory
	t.Run("invalid_directory", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		invalidPath := filepath.Join(testDataDir, "non-existent-dir") + "/..."
		_, err := LoadContent([]string{invalidPath}, nil)
		assert.Error(t, err)
	})

	// test with multiple patterns of different types
	t.Run("mixed_patterns", func(t *testing.T) {
		// change to a subdir of testDataDir where one type of file exists for bash-style patterns
		oldWd, err := os.Getwd()
		require.NoError(t, err)

		// construct the path properly to avoid linter warnings about path separators
		nestedPath := filepath.Join(testDataDir, "nested") + "/..."

		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"), // standard glob
			nestedPath,                         // go-style recursive
		}, nil)
		require.NoError(t, err)

		// now change to testDataDir for the bash-style pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		// add content from bash-style pattern
		txtContent, err := LoadContent([]string{"**/*.txt"}, nil)
		require.NoError(t, err)

		// return to original directory
		err = os.Chdir(oldWd)
		require.NoError(t, err)

		// combine results
		result += txtContent

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "This is another text file for testing")
	})

	// test with exclude patterns
	t.Run("exclude_patterns", func(t *testing.T) {
		// test excluding specific files
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "..."), // all files
		}, []string{
			"**/*.txt", // exclude all text files
		})
		require.NoError(t, err)

		// should contain go files but not txt files
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "This is another text file for testing")

		// test excluding directories
		result, err = LoadContent([]string{
			filepath.Join(testDataDir, "..."), // all files
		}, []string{
			"**/nested/**", // exclude all files in nested directory and its subdirectories
		})
		require.NoError(t, err)

		// should contain root level files but not nested directory files
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "package nested")
		assert.NotContains(t, result, "package deep")
		assert.NotContains(t, result, "This is another text file for testing")

		// test multiple exclude patterns
		result, err = LoadContent([]string{
			filepath.Join(testDataDir, "..."), // all files
		}, []string{
			"**/*.txt",        // exclude text files
			"**/deep/**/*.go", // exclude go files in deep directory
		})
		require.NoError(t, err)

		// should contain some go files but not txt files or deep go files
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.NotContains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "This is another text file for testing")
	})
}

// Test each pattern handling function individually
func TestProcessPatterns(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	testDataDir := filepath.Join(wd, "testdata")

	// test processBashStylePattern
	t.Run("processBashStylePattern", func(t *testing.T) {
		// change directory to testDataDir to make bash-style patterns work
		oldWd, err := os.Getwd()
		require.NoError(t, err)
		err = os.Chdir(testDataDir)
		require.NoError(t, err)
		defer os.Chdir(oldWd) // restore original working directory

		matchedFiles := make(map[string]struct{})

		// test with valid pattern
		err = processBashStylePattern("**/*.go", matchedFiles)
		require.NoError(t, err)
		assert.Greater(t, len(matchedFiles), 0)

		// count .go files
		goCount := 0
		for file := range matchedFiles {
			if filepath.Ext(file) == ".go" {
				goCount++
			}
		}
		assert.Equal(t, 3, goCount) // should match all 3 .go files

		// test with no matches
		matchedFiles = make(map[string]struct{})
		err = processBashStylePattern("nonexistent/**/*.xyz", matchedFiles)
		require.NoError(t, err) // no error, just no matches
		assert.Equal(t, 0, len(matchedFiles))
	})

	// test processGoStylePattern
	t.Run("processGoStylePattern", func(t *testing.T) {
		matchedFiles := make(map[string]struct{})

		// test with valid pattern for all files
		// construct the path properly to avoid linter warnings about path separators
		allFilesPath := testDataDir + "/..."
		err := processGoStylePattern(allFilesPath, matchedFiles)
		require.NoError(t, err)
		assert.Greater(t, len(matchedFiles), 0)
		assert.Equal(t, 5, len(matchedFiles)) // should match all 5 files

		// test with extension filter
		matchedFiles = make(map[string]struct{})
		// construct the path properly to avoid linter warnings about path separators
		goFilesPath := testDataDir + "/.../*.go"
		err = processGoStylePattern(goFilesPath, matchedFiles)
		require.NoError(t, err)
		assert.Equal(t, 3, len(matchedFiles)) // should match 3 .go files

		// test with invalid base directory
		matchedFiles = make(map[string]struct{})
		err = processGoStylePattern("nonexistent/...", matchedFiles)
		require.NoError(t, err) // no error, just warning logged
		assert.Equal(t, 0, len(matchedFiles))
	})

	// test processStandardGlobPattern
	t.Run("processStandardGlobPattern", func(t *testing.T) {
		matchedFiles := make(map[string]struct{})

		// test with exact file match
		err := processStandardGlobPattern(filepath.Join(testDataDir, "test1.go"), matchedFiles)
		require.NoError(t, err)
		assert.Equal(t, 1, len(matchedFiles))

		// test with wildcard
		matchedFiles = make(map[string]struct{})
		err = processStandardGlobPattern(filepath.Join(testDataDir, "*.go"), matchedFiles)
		require.NoError(t, err)
		assert.Equal(t, 1, len(matchedFiles)) // should match 1 .go file in root

		// test with directory (should walk recursively)
		matchedFiles = make(map[string]struct{})
		err = processStandardGlobPattern(filepath.Join(testDataDir, "nested"), matchedFiles)
		require.NoError(t, err)
		assert.Equal(t, 3, len(matchedFiles)) // should match all 3 files in nested

		// test with no matches
		matchedFiles = make(map[string]struct{})
		err = processStandardGlobPattern("nonexistent/*.xyz", matchedFiles)
		require.NoError(t, err) // no error, just no matches
		assert.Equal(t, 0, len(matchedFiles))
	})

	// test getSortedFiles
	t.Run("getSortedFiles", func(t *testing.T) {
		matchedFiles := map[string]struct{}{
			"c.txt": {},
			"a.txt": {},
			"b.txt": {},
		}

		sortedFiles := getSortedFiles(matchedFiles)
		assert.Equal(t, 3, len(sortedFiles))
		assert.Equal(t, "a.txt", sortedFiles[0])
		assert.Equal(t, "b.txt", sortedFiles[1])
		assert.Equal(t, "c.txt", sortedFiles[2])
	})

	// test formatFileContents
	t.Run("formatFileContents", func(t *testing.T) {
		files := []string{
			filepath.Join(testDataDir, "test1.go"),
			filepath.Join(testDataDir, "test2.txt"),
		}

		result, err := formatFileContents(files)
		require.NoError(t, err)

		// verify file content and headers
		assert.Contains(t, result, "// file:") // Both Go and txt files use // comments in our impl
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "This is a text file for testing")
	})

	// test applyExcludePatterns
	t.Run("applyExcludePatterns", func(t *testing.T) {
		// create a map of matched files
		matchedFiles := map[string]struct{}{
			filepath.Join(testDataDir, "test1.go"):                   {},
			filepath.Join(testDataDir, "test2.txt"):                  {},
			filepath.Join(testDataDir, "nested", "test3.go"):         {},
			filepath.Join(testDataDir, "nested", "test5.txt"):        {},
			filepath.Join(testDataDir, "nested", "deep", "test4.go"): {},
		}

		// test excluding by extension
		excludePatterns := []string{"**/*.txt"}
		filtered := applyExcludePatterns(matchedFiles, excludePatterns)
		assert.Equal(t, 3, len(filtered), "Should have 3 files after excluding *.txt")
		_, hasGo1 := filtered[filepath.Join(testDataDir, "test1.go")]
		assert.True(t, hasGo1, "Should have test1.go")
		_, hasTxt := filtered[filepath.Join(testDataDir, "test2.txt")]
		assert.False(t, hasTxt, "Should not have test2.txt")

		// test excluding by directory
		excludePatterns = []string{"**/nested/**"}
		filtered = applyExcludePatterns(matchedFiles, excludePatterns)
		assert.Equal(t, 2, len(filtered), "Should have 2 files after excluding nested directory")
		_, hasNested := filtered[filepath.Join(testDataDir, "nested", "test3.go")]
		assert.False(t, hasNested, "Should not have nested/test3.go")

		// test multiple exclude patterns
		excludePatterns = []string{"**/*.txt", "**/deep/**"}
		filtered = applyExcludePatterns(matchedFiles, excludePatterns)
		assert.Equal(t, 2, len(filtered), "Should have 2 files after excluding *.txt and deep directory")
		_, hasDeep := filtered[filepath.Join(testDataDir, "nested", "deep", "test4.go")]
		assert.False(t, hasDeep, "Should not have nested/deep/test4.go")
		_, hasNestedGo := filtered[filepath.Join(testDataDir, "nested", "test3.go")]
		assert.True(t, hasNestedGo, "Should still have nested/test3.go")

		// test no exclude patterns
		filtered = applyExcludePatterns(matchedFiles, nil)
		assert.Equal(t, len(matchedFiles), len(filtered), "Should have all files when no exclude patterns")

		// test empty exclude patterns
		filtered = applyExcludePatterns(matchedFiles, []string{})
		assert.Equal(t, len(matchedFiles), len(filtered), "Should have all files when empty exclude patterns")
	})
}
