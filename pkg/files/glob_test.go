package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadContent(t *testing.T) {
	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	// test loading files by direct paths
	t.Run("direct_paths", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "test1.go"),
			filepath.Join(testDataDir, "test2.txt"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "func TestFunc1")
		assert.Contains(t, result, "This is a text file for testing")
	})

	// test loading files using standard glob pattern
	t.Run("standard_glob", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "func TestFunc1")
		assert.NotContains(t, result, "This is a text file for testing")
	})

	// test recursive directory traversal
	t.Run("directory_recursive", func(t *testing.T) {
		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "nested"),
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "func TestFunc3")
		assert.Contains(t, result, "package deep")
	})

	// test go-style recursive pattern with extension filter
	t.Run("go_style_recursive_go_files", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		goStylePath := testDataDir + "/.../*.go"
		result, err := LoadContent([]string{goStylePath}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")
	})

	// test go-style recursive pattern with no extension filter
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

	// test bash-style recursive pattern with **
	t.Run("bash_style_recursive", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testDataDir for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		result, err := LoadContent([]string{
			"**/*.go", // use relative pattern
		}, nil)
		require.NoError(t, err)

		assert.Contains(t, result, "package testdata")
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")
		assert.NotContains(t, result, "This is a text file for testing")

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)
	})

	// test bash-style recursive pattern specific to nested directory
	t.Run("bash_style_recursive_nested", func(t *testing.T) {
		// save current directory
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// change to testDataDir for the relative pattern
		err = os.Chdir(testDataDir)
		require.NoError(t, err)

		result, err := LoadContent([]string{
			"nested/**/*.go", // use relative pattern
		}, nil)
		require.NoError(t, err)

		assert.NotContains(t, result, "package testdata") // should not include root level files
		assert.Contains(t, result, "package nested")
		assert.Contains(t, result, "package deep")

		// return to original directory
		err = os.Chdir(currentDir)
		require.NoError(t, err)
	})

	// test bash-style recursive pattern with multiple patterns
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
		}, nil)
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

	// test with invalid directory in go-style pattern
	t.Run("invalid_directory", func(t *testing.T) {
		// construct the path properly to avoid linter warnings about path separators
		invalidPath := filepath.Join(testDataDir, "non-existent-dir") + "/..."
		_, err := LoadContent([]string{invalidPath}, nil)
		assert.Error(t, err)
	})

	// test combination of different pattern styles
	t.Run("mixed_patterns", func(t *testing.T) {
		// construct go-style path properly
		nestedPath := testDataDir + "/nested/..."

		result, err := LoadContent([]string{
			filepath.Join(testDataDir, "*.go"), // standard glob
			nestedPath,                         // go-style recursive
		}, nil)
		require.NoError(t, err)

		// now change to testDataDir for the bash-style pattern
		currentDir, err := os.Getwd()
		require.NoError(t, err)

		// add content from bash-style pattern
		txtContent, err := LoadContent([]string{"**/*.txt"}, nil)
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

	// test with exclude patterns
	t.Run("exclude_patterns", func(t *testing.T) {
		// test excluding specific files
		result, err := LoadContent([]string{
			testDataDir + "/...", // all files
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
			testDataDir + "/...", // all files
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
			testDataDir + "/...", // all files
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
	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

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
		err = processBashStylePattern("**/*.go", matchedFiles)
		require.NoError(t, err)

		// we should have matched at least 3 go files: test1.go, nested/test3.go, nested/deep/test4.go
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test a non-existent pattern
		matchedFiles = make(map[string]struct{})
		err = processBashStylePattern("**/nonexistent*.abc", matchedFiles)
		assert.NoError(t, err) // matching zero files is not an error
		assert.Empty(t, matchedFiles)

		// test with an invalid pattern
		matchedFiles = make(map[string]struct{})
		err = processBashStylePattern("**/*[", matchedFiles) // invalid pattern
		assert.Error(t, err)
	})

	// test processGoStylePattern
	t.Run("processGoStylePattern", func(t *testing.T) {
		// create a map to store matched files
		matchedFiles := make(map[string]struct{})

		// test go-style recursive pattern with .go extension filter
		pattern := testDataDir + "/.../*.go"
		err := processGoStylePattern(pattern, matchedFiles)
		require.NoError(t, err)

		// we should have matched at least 3 go files
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test recursive pattern without filter
		matchedFiles = make(map[string]struct{})
		pattern = testDataDir + "/..."
		err = processGoStylePattern(pattern, matchedFiles)
		require.NoError(t, err)

		// we should have matched all files, at least 5
		assert.GreaterOrEqual(t, len(matchedFiles), 5)

		// test with non-existent directory
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "nonexistent") + "/..."
		err = processGoStylePattern(pattern, matchedFiles)
		assert.NoError(t, err) // non-existent dir is handled gracefully
		assert.Empty(t, matchedFiles)
	})

	// test processStandardGlobPattern
	t.Run("processStandardGlobPattern", func(t *testing.T) {
		// create a map to store matched files
		matchedFiles := make(map[string]struct{})

		// test standard glob with extension
		pattern := filepath.Join(testDataDir, "*.go")
		err := processStandardGlobPattern(pattern, matchedFiles)
		require.NoError(t, err)

		// we should have matched 1 go file in the top level
		assert.Equal(t, 1, len(matchedFiles))

		// test with wildcard to match directory
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "n*") // should match "nested" directory
		err = processStandardGlobPattern(pattern, matchedFiles)
		require.NoError(t, err)

		// recursive traversal should match all files in nested, including subdirectories
		assert.GreaterOrEqual(t, len(matchedFiles), 3)

		// test with non-existent pattern
		matchedFiles = make(map[string]struct{})
		pattern = filepath.Join(testDataDir, "nonexistent*.xyz")
		err = processStandardGlobPattern(pattern, matchedFiles)
		assert.NoError(t, err) // non-matching pattern is not an error
		assert.Empty(t, matchedFiles)
	})

	// test parseRecursivePattern
	t.Run("parseRecursivePattern", func(t *testing.T) {
		// test with no filter
		basePath, filter := parseRecursivePattern("pkg/...")
		assert.Equal(t, "pkg", basePath)
		assert.Equal(t, "", filter)

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