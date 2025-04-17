package files

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitIgnoreRespect(t *testing.T) {
	t.Parallel()
	// create a temporary test directory
	tmpDir := t.TempDir()

	// create test files
	testFiles := map[string]string{
		"test.go":            "package test",
		"test.tmp":           "temporary file content",
		"logs/app.log":       "log file content",
		"logs/important.log": "important log content",
		"build/output.go":    "package main",
		"src/component.go":   "package component",
		"src/logs/debug.log": "debug logs",
		"vendor/lib/util.go": "package util",
		".gitignore":         "*.tmp\n*.log\n/build/\nvendor/\n!important.log\nsrc/logs/",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		// ensure directory exists
		err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
		require.NoError(t, err)
		// write file
		err = os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	// save current directory
	origDir, err := os.Getwd()
	require.NoError(t, err)

	// change to test directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// restore original directory when done
	defer func() {
		err := os.Chdir(origDir)
		require.NoError(t, err)
	}()

	// test that .gitignore patterns are respected
	result, err := LoadContent([]string{"**/*"}, nil, 64*1024)
	require.NoError(t, err)

	// should include non-ignored files
	assert.Contains(t, result, "package test", "Should include non-ignored files")
	assert.Contains(t, result, "package component", "Should include files in non-ignored directories")

	// should not include ignored files
	assert.NotContains(t, result, "temporary file content", "Should respect *.tmp pattern")
	assert.NotContains(t, result, "log file content", "Should respect *.log pattern")
	assert.NotContains(t, result, "debug logs", "Should respect src/logs/ pattern")
	assert.NotContains(t, result, "package main", "Should respect /build/ pattern")
	assert.NotContains(t, result, "package util", "Should respect vendor/ pattern")
	
	// negation patterns are not supported, so should not include this file
	assert.NotContains(t, result, "important log content", "Should ignore negation patterns")

	// test with explicit exclude patterns overriding .gitignore
	result, err = LoadContent([]string{"**/*"}, []string{"**/*.go"}, 64*1024)
	require.NoError(t, err)

	// should not include any go files (due to explicit exclude)
	assert.NotContains(t, result, "package test", "Should respect explicit exclude patterns")
	assert.NotContains(t, result, "package component", "Should respect explicit exclude patterns")

	// still respects .gitignore
	assert.NotContains(t, result, "temporary file content", "Should still respect .gitignore patterns")
}

func TestPatternMatching(t *testing.T) {
	t.Run("matchesPattern", func(t *testing.T) {
		tests := []struct {
			name     string
			pattern  string
			filePath string
			relPath  string
			want     bool
		}{
			{"bash_pattern_match", "**/*.go", "src/main.go", "src/main.go", true},
			{"bash_pattern_no_match", "**/*.go", "src/main.js", "src/main.js", false},
			{"go_pattern_match", "src/...", "src/main.go", "src/main.go", true},
			{"go_pattern_no_match", "src/...", "pkg/main.go", "pkg/main.go", false},
			{"standard_pattern_match", "*.go", "main.go", "main.go", true},
			{"standard_pattern_no_match", "*.go", "main.js", "main.js", false},
			{"invalid_pattern", "[invalid", "file.txt", "file.txt", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := matchesPattern(tt.pattern, tt.filePath, tt.relPath)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("matchesGoStylePattern", func(t *testing.T) {
		tests := []struct {
			name     string
			pattern  string
			filePath string
			want     bool
		}{
			{"base_match", "src/...", "src/main.go", true},
			{"sub_match", "src/...", "src/sub/main.go", true},
			{"no_match", "src/...", "pkg/main.go", false},
			{"with_filter_match", "src/.../*.go", "src/main.go", true},
			{"with_filter_no_match", "src/.../*.go", "src/main.js", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := matchesGoStylePattern(tt.pattern, tt.filePath)
				assert.Equal(t, tt.want, got)
			})
		}
	})
}

func TestCommonIgnorePatterns(t *testing.T) {
	// create a temporary test directory
	tmpDir := t.TempDir()

	// create test files for common ignore patterns
	testFiles := map[string]string{
		"regular.go":                              "package main",
		".git/HEAD":                               "ref: refs/heads/master",
		".git/config":                             "[core]",
		"node_modules/package/index.js":           "console.log('test')",
		".idea/workspace.xml":                     "<project>",
		"__pycache__/module.pyc":                  "# python bytecode",
		"venv/lib/python3.9/site-packages/pkg.py": "# python package",
		"dist/bundle.js":                          "// bundled js",
		"build/output.bin":                        "binary content",
		"logs/app.log":                            "log entries",
		".vscode/settings.json":                   "{ \"settings\": {} }",
		"target/app.class":                        "java bytecode",
		".DS_Store":                               "macOS metadata",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		// ensure directory exists
		err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
		require.NoError(t, err)
		// write file
		err = os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(t, err)
	}

	// save current directory
	origDir, err := os.Getwd()
	require.NoError(t, err)

	// change to test directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// restore original directory when done
	defer func() {
		err := os.Chdir(origDir)
		require.NoError(t, err)
	}()

	// test with common ignore patterns (without .gitignore)
	result, err := LoadContent([]string{"**/*"}, nil, 64*1024)
	require.NoError(t, err)

	// should include regular files
	assert.Contains(t, result, "package main", "Should include regular files")

	// should automatically exclude common ignored directories/files
	assert.NotContains(t, result, "ref: refs/heads/master", "Should ignore .git directory")
	assert.NotContains(t, result, "console.log('test')", "Should ignore node_modules directory")
	assert.NotContains(t, result, "<project>", "Should ignore .idea directory")
	assert.NotContains(t, result, "# python bytecode", "Should ignore __pycache__ directory")
	assert.NotContains(t, result, "# python package", "Should ignore venv directory")
	assert.NotContains(t, result, "// bundled js", "Should ignore dist directory")
	assert.NotContains(t, result, "binary content", "Should ignore build directory")
	assert.NotContains(t, result, "log entries", "Should ignore logs directory")
	assert.NotContains(t, result, "{ \"settings\": {} }", "Should ignore .vscode directory")
	assert.NotContains(t, result, "java bytecode", "Should ignore target directory")
	assert.NotContains(t, result, "macOS metadata", "Should ignore .DS_Store files")

	// test other explicit exclude cases
	osArch := "darwin_amd64"
	binPath := filepath.Join(tmpDir, "bin", osArch, "app")
	err = os.MkdirAll(filepath.Dir(binPath), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(binPath, []byte("binary file"), 0o644)
	require.NoError(t, err)

	// explicitly add a temp file to verify /tmp exclusion was removed from common patterns
	tmpFilePath := filepath.Join(tmpDir, "tmp", "temp.txt")
	err = os.MkdirAll(filepath.Dir(tmpFilePath), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(tmpFilePath, []byte("temp file content"), 0o644)
	require.NoError(t, err)

	result, err = LoadContent([]string{"**/*"}, []string{"**/bin/**"}, 64*1024)
	require.NoError(t, err)
	assert.NotContains(t, result, "binary file", "Should exclude files matching explicit exclude pattern")
	assert.Contains(t, result, "temp file content", "Should include files in tmp directory since it's not excluded by default")
}

func TestLoadContent(t *testing.T) {
	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	// default max file size for all tests
	defaultMaxFileSize := int64(64 * 1024)

	t.Run("direct_paths", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "test1.go"),
			filepath.Join(testDataDir, "test2.txt"),
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "func TestFunc1")
		assert.Contains(t, result, "This is a text file for testing")
	})

	t.Run("standard_glob", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"),
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "func TestFunc1")
		assert.NotContains(t, result, "This is a text file for testing")
	})

	t.Run("directory_recursive", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "nested"),
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "func TestFunc3")
		assert.Contains(t, result, "package deep")
	})

	t.Run("go_style_recursive_go_files", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		goStylePath := testDataDir + "/.../*.go"
		result, err := LoadContent([]string{goStylePath}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
	})

	t.Run("go_style_recursive_all", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		goStylePath := testDataDir + "/..."
		result, err := LoadContent([]string{goStylePath}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "This is another text file for testing")
	})

	t.Run("bash_style_recursive", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testDataDir for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		result, err := LoadContent([]string{
			"**/*.go", // use relative pattern
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)
	})

	t.Run("bash_style_recursive_nested", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testDataDir for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		result, err := LoadContent([]string{
			"nested/**/*.go", // use relative pattern
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.NotContains(t, result, "package testdata") // should not include root level files
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)
	})

	t.Run("bash_style_recursive_multiple", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testDataDir for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		result, err := LoadContent([]string{
			"**/*.go",  // use relative pattern
			"**/*.txt", // use relative pattern
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, result, "This is a text file for testing")
		assert.Contains(t, result, "This is another text file for testing")

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)
	})

	t.Run("empty_pattern", func(t *testing.T) {
		result, err := LoadContent([]string{}, nil, defaultMaxFileSize)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("non_existent_pattern", func(t *testing.T) {
		_, err := LoadContent([]string{"non-existent-pattern-*.xyz"}, nil, defaultMaxFileSize)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no files matched the provided patterns")
	})

	t.Run("invalid_directory", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		invalidPath := filepath.Join(testDataDir, "non-existent-dir") + "/..."
		_, err := LoadContent([]string{invalidPath}, nil, defaultMaxFileSize)
		require.Error(t, err)
	})

	t.Run("mixed_patterns", func(t *testing.T) {
		// construct go-style path properly
		nestedPath := testDataDir + "/nested/..."

		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"), // standard glob
			nestedPath,                         // go-style recursive
		}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		// now change to testDataDir for the bash-style pattern
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// add content from bash-style pattern
		txtContent, err := LoadContent([]string{"**/*.txt"}, nil, defaultMaxFileSize)
		require.NoError(t, err)

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)

		// verify combined content
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.Contains(t, txtContent, "This is a text file for testing")
		assert.Contains(t, txtContent, "This is another text file for testing")
	})

	t.Run("exclude_patterns", func(t *testing.T) {
		// test excluding specific files
		result, err := LoadContent([]string{
			testDataDir + "/...", // all files
		}, []string{
			"**/*.txt", // exclude all text files
		}, defaultMaxFileSize)
		require.NoError(t, err)

		// should contain go files but not txt files
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "This is another text file for testing")

		// test excluding directories
		result, err = LoadContent([]string{
			testDataDir + "/...", // all files
		}, []string{
			"**/nested/**", // exclude all files in nested directory and its subdirectories
		}, defaultMaxFileSize)
		require.NoError(t, err)

		// should contain root level files but not nested directory files
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "This is a text file for testing")
		assert.NotContains(t, result, "package nested")
		assert.NotContains(t, result, "package deep")
		assert.NotContains(t, result, "This is another text file for testing")

		// test multiple exclude patterns
		result, err = LoadContent([]string{
			testDataDir + "/...", // all files
		}, []string{
			"**/*.txt",        // exclude text files
			"**/deep/**/*.go", // exclude go files in deep directory
		}, defaultMaxFileSize)
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
	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	// default max file size for all tests
	defaultMaxFileSize := int64(64 * 1024)

	// test processBashStylePattern
	t.Run("processBashStylePattern", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testdata for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		// ensure we go back when done
		defer func() {
			err := os.Chdir(currentDir)
			if err != nil {
				t.Errorf("failed to change back to original directory: %v", err)
			}
		}()

		matchedFiles := make(map[string]struct{})
		err = processBashStylePattern("**/*.go", matchedFiles, defaultMaxFileSize)
		require.NoError(t, err)

		// we should have matched at least 3 go files: test1.go, nested/test3.go, nested/deep/test4.go
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test a non-existent pattern
		matchedFiles = make(map[string]struct{})
		err = processBashStylePattern("**/nonexistent*.abc", matchedFiles, defaultMaxFileSize)
		require.NoError(t, err) // matching zero files is not an error
		assert.Empty(t, matchedFiles)

		// test with an invalid pattern
		matchedFiles = make(map[string]struct{})
		err = processBashStylePattern("**/*[", matchedFiles, defaultMaxFileSize) // invalid pattern
		require.Error(t, err)
	})

	// test processGoStylePattern
	t.Run("processGoStylePattern", func(t *testing.T) {
		// create a map to store matched files
		matchedFiles := make(map[string]struct{})

		// test go-style recursive pattern with .go extension filter
		pattern := testDataDir + "/.../*.go"
		err := processGoStylePattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err)

		// we should have matched at least 3 go files
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test recursive pattern without filter
		matchedFiles = make(map[string]struct{})
		pattern = testDataDir + "/..."
		err = processGoStylePattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err)

		// we should have matched all files, at least 5
		assert.GreaterOrEqual(t, len(matchedFiles), 5)

		// test with non-existent directory
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "nonexistent") + "/..."
		err = processGoStylePattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err) // non-existent dir is handled gracefully
		assert.Empty(t, matchedFiles)
	})

	// test processStandardGlobPattern
	t.Run("processStandardGlobPattern", func(t *testing.T) {
		// create a map to store matched files
		matchedFiles := make(map[string]struct{})

		// test standard glob with extension
		pattern := filepath.Join(testDataDir, "*.go")
		err := processStandardGlobPattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err)

		// we should have matched 1 go file in the top level
		assert.Len(t, matchedFiles, 1)

		// test with wildcard to match directory
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "n*") // should match "nested" directory
		err = processStandardGlobPattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err)

		// recursive traversal should match all files in nested, including subdirectories
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test with non-existent pattern
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "nonexistent*.xyz")
		err = processStandardGlobPattern(pattern, matchedFiles, defaultMaxFileSize)
		require.NoError(t, err) // non-matching pattern is not an error
		assert.Empty(t, matchedFiles)
	})

	// test parseRecursivePattern
	t.Run("parseRecursivePattern", func(t *testing.T) {
		// test with no filter
		basePath, filter := parseRecursivePattern("pkg/...")
		assert.Equal(t, "pkg", basePath)
		assert.Empty(t, filter)

		// test with extension filter
		basePath, filter = parseRecursivePattern("pkg/.../*.go")
		assert.Equal(t, "pkg", basePath)
		assert.Equal(t, "*.go", filter)

		// test with filename pattern
		basePath, filter = parseRecursivePattern("pkg/.../test*.go")
		assert.Equal(t, "pkg", basePath)
		assert.Equal(t, "test*.go", filter)
	})

	// test formatFileContents
	t.Run("formatFileContents", func(t *testing.T) {
		// get actual files from testdata
		files := []string{
			filepath.Join(testDataDir, "test1.go"),
			filepath.Join(testDataDir, "test2.txt"),
		}

		result, err := formatFileContents(files)
		require.NoError(t, err)

		// check that we have proper headers for each file
		assert.Contains(t, result, "// file: ")
		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "This is a text file for testing")
	})

	// test getFileHeader function with different file extensions
	t.Run("getFileHeader", func(t *testing.T) {
		testCases := []struct {
			filePath       string
			expectedHeader string
		}{
			// hash-style comments (#)
			{"file.py", "# file: file.py\n"},
			{"file.yaml", "# file: file.yaml\n"},
			{"Makefile", "# file: Makefile\n"},

			// Double-slash comments (//)
			{"file.go", "// file: file.go\n"},
			{"file.js", "// file: file.js\n"},
			{"file.cpp", "// file: file.cpp\n"},
			{"file.ts", "// file: file.ts\n"},

			// HTML/XML style comments
			{"file.html", "<!-- file: file.html -->\n"},
			{"file.xml", "<!-- file: file.xml -->\n"},
			{"file.vue", "<!-- file: file.vue -->\n"},

			// CSS style comments
			{"file.css", "/* file: file.css */\n"},
			{"file.scss", "/* file: file.scss */\n"},

			// SQL comments
			{"file.sql", "-- file: file.sql\n"},

			// lisp/Clojure comments
			{"file.clj", ";; file: file.clj\n"},

			// haskell/VHDL comments
			{"file.hs", "-- file: file.hs\n"},

			// PowerShell comments
			{"file.ps1", "# file: file.ps1\n"},

			// batch file comments
			{"file.bat", ":: file: file.bat\n"},

			// fortran comments
			{"file.f90", "! file: file.f90\n"},

			// default case for unknown extension
			{"file.unknown", "// file: file.unknown\n"},
			{"file", "// file: file\n"},
		}

		for _, tc := range testCases {
			t.Run(tc.filePath, func(t *testing.T) {
				header := getFileHeader(tc.filePath)
				assert.Equal(t, tc.expectedHeader, header)
			})
		}
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
		assert.Len(t, filtered, 3, "Should have 3 files after excluding *.txt")
		_, hasGo1 := filtered[filepath.Join(testDataDir, "test1.go")]
		assert.True(t, hasGo1, "Should have test1.go")
		_, hasTxt := filtered[filepath.Join(testDataDir, "test2.txt")]
		assert.False(t, hasTxt, "Should not have test2.txt")

		// test excluding by directory
		excludePatterns = []string{"**/nested/**"}
		filtered = applyExcludePatterns(matchedFiles, excludePatterns)
		assert.Len(t, filtered, 2, "Should have 2 files after excluding nested directory")
		_, hasNested := filtered[filepath.Join(testDataDir, "nested", "test3.go")]
		assert.False(t, hasNested, "Should not have nested/test3.go")

		// test multiple exclude patterns
		excludePatterns = []string{"**/*.txt", "**/deep/**"}
		filtered = applyExcludePatterns(matchedFiles, excludePatterns)
		assert.Len(t, filtered, 2, "Should have 2 files after excluding *.txt and deep directory")
		_, hasDeep := filtered[filepath.Join(testDataDir, "nested", "deep", "test4.go")]
		assert.False(t, hasDeep, "Should not have nested/deep/test4.go")
		_, hasNestedGo := filtered[filepath.Join(testDataDir, "nested", "test3.go")]
		assert.True(t, hasNestedGo, "Should still have nested/test3.go")

		// test no exclude patterns
		filtered = applyExcludePatterns(matchedFiles, nil)
		assert.Equal(t, matchedFiles, filtered, "Should have all files when no exclude patterns")

		// test empty exclude patterns
		filtered = applyExcludePatterns(matchedFiles, []string{})
		assert.Equal(t, matchedFiles, filtered, "Should have all files when empty exclude patterns")
	})

	// test maxFileSize parameter
	t.Run("maxFileSize_limit", func(t *testing.T) {
		// get test files
		file1 := filepath.Join(testDataDir, "test1.go")
		file2 := filepath.Join(testDataDir, "test2.txt")

		// get file sizes
		info1, err := os.Stat(file1)
		require.NoError(t, err)
		size1 := info1.Size()

		info2, err := os.Stat(file2)
		require.NoError(t, err)
		size2 := info2.Size()

		// set max file size to 1 byte to force exclusion of both files
		tinySize := int64(1)
		result, err := LoadContent([]string{file1, file2}, nil, tinySize)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no files matched")
		assert.Empty(t, result)

		// set max file size to exclude just the larger file
		mediumSize := int64(math.Min(float64(size1), float64(size2))) + 1
		result, err = LoadContent([]string{file1, file2}, nil, mediumSize)
		require.NoError(t, err)

		// only the smaller file should be included
		if size1 < size2 {
			assert.Contains(t, result, "package testdata", "Should contain Go file content")
			assert.NotContains(t, result, "This is a text file for testing", "Should not contain text file content")
		} else {
			assert.NotContains(t, result, "package testdata", "Should not contain Go file content")
			assert.Contains(t, result, "This is a text file for testing", "Should contain text file content")
		}

		// set max file size to include both files
		largeSize := int64(math.Max(float64(size1), float64(size2))) + 1
		result, err = LoadContent([]string{file1, file2}, nil, largeSize)
		require.NoError(t, err)

		// both files should be included
		assert.Contains(t, result, "package testdata", "Should contain Go file content")
		assert.Contains(t, result, "This is a text file for testing", "Should contain text file content")
	})

	// test helper functions for pattern matching
	t.Run("pattern_matching_helpers", func(t *testing.T) {
		// setup test data
		cwd, err := os.Getwd()
		require.NoError(t, err)

		// test matchesPattern function
		t.Run("matchesPattern", func(t *testing.T) {
			// bash-style pattern
			assert.True(t, matchesPattern("**/*.txt",
				filepath.Join(testDataDir, "test2.txt"),
				filepath.Join("pkg", "files", "testdata", "test2.txt")))

			assert.False(t, matchesPattern("**/*.go",
				filepath.Join(testDataDir, "test2.txt"),
				filepath.Join("pkg", "files", "testdata", "test2.txt")))

			// standard glob pattern
			assert.True(t, matchesPattern("*.txt",
				filepath.Join(testDataDir, "test2.txt"),
				filepath.Join("pkg", "files", "testdata", "test2.txt")))

			// go-style pattern requires a real path, test separately
		})

		// test matchesGoStylePattern function
		t.Run("matchesGoStylePattern", func(t *testing.T) {
			// create test pattern - do not use filepath.Join for patterns with /...
			goPattern := testDataDir + "/.../*.go"

			// should match go files directly in testDataDir
			assert.True(t, matchesGoStylePattern(goPattern,
				filepath.Join(testDataDir, "test1.go")))

			// should match go files in subdirectories
			assert.True(t, matchesGoStylePattern(goPattern,
				filepath.Join(testDataDir, "nested", "test3.go")))

			// should not match txt files
			assert.False(t, matchesGoStylePattern(goPattern,
				filepath.Join(testDataDir, "test2.txt")))

			// should not match files outside testDataDir
			assert.False(t, matchesGoStylePattern(goPattern,
				filepath.Join(cwd, "glob.go")))

			// test pattern with no filter (all files in dir)
			allFilesPattern := testDataDir + "/..."
			assert.True(t, matchesGoStylePattern(allFilesPattern,
				filepath.Join(testDataDir, "test2.txt")))
		})

		// test shouldExcludeFile function
		t.Run("shouldExcludeFile", func(t *testing.T) {
			patternExcludeCount := make(map[string]int)

			// should exclude txt files with bash-style pattern
			excludePatterns := []string{"**/*.txt"}
			assert.True(t, shouldExcludeFile(
				filepath.Join(testDataDir, "test2.txt"),
				cwd, excludePatterns, patternExcludeCount))

			// should not exclude go files with txt pattern
			assert.False(t, shouldExcludeFile(
				filepath.Join(testDataDir, "test1.go"),
				cwd, excludePatterns, patternExcludeCount))

			// check that counts were incremented
			assert.Equal(t, 1, patternExcludeCount["**/*.txt"])

			// reset counters
			patternExcludeCount = make(map[string]int)

			// test with multiple patterns
			excludePatterns = []string{"**/*.txt", "**/deep/**"}
			assert.True(t, shouldExcludeFile(
				filepath.Join(testDataDir, "nested", "deep", "test4.go"),
				cwd, excludePatterns, patternExcludeCount))

			assert.Equal(t, 1, patternExcludeCount["**/deep/**"])
		})
	})
}
