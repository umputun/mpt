// Package files provides functionality for working with file globs and content loading.
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-pkgz/lgr"
)

// DefaultMaxFileSize defines the default maximum size of individual files to process (64KB)
const DefaultMaxFileSize = 64 * 1024

// LoadContent loads content from files matching the given patterns and returns a formatted string
// with file names as comments and their contents. Supports recursive directory traversal.
// Exclude patterns can be provided to filter out unwanted files.
// The maxFileSize parameter controls the maximum size of individual files to process.
// Git ignore patterns from .gitignore files are automatically respected.
func LoadContent(patterns, excludePatterns []string, maxFileSize int64) (string, error) {
	if len(patterns) == 0 {
		return "", nil
	}

	// prepare all exclude patterns
	allExcludePatterns := prepareExcludePatterns(excludePatterns)

	// map to store all matched file paths
	matchedFiles := make(map[string]struct{})

	// expand all patterns and collect unique file paths
	for _, pattern := range patterns {
		// process different types of patterns
		switch {
		case strings.Contains(pattern, "**"):
			// bash-style patterns with **
			if err := processBashStylePattern(pattern, matchedFiles, maxFileSize); err != nil {
				return "", err
			}
		case strings.Contains(pattern, "/..."):
			// go-style recursive pattern: dir/...
			if err := processGoStylePattern(pattern, matchedFiles, maxFileSize); err != nil {
				return "", err
			}
		default:
			// standard glob pattern
			if err := processStandardGlobPattern(pattern, matchedFiles, maxFileSize); err != nil {
				return "", err
			}
		}
	}

	// apply exclusion patterns if any
	matchedFiles = applyExcludePatterns(matchedFiles, allExcludePatterns)

	// get sorted list of files
	sortedFiles := getSortedFiles(matchedFiles)
	if len(sortedFiles) == 0 {
		// check if we should report file size errors
		if err := checkFileSizeErrors(patterns, excludePatterns, maxFileSize); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no files matched the provided patterns. Try a different pattern such as \"./.../*.go\" or \"./**/*.go\" for recursive matching")
	}

	// format and combine file contents
	return formatFileContents(sortedFiles)
}

// checkFileSizeErrors checks if any direct file paths were skipped due to size limits
func checkFileSizeErrors(patterns, excludePatterns []string, maxFileSize int64) error {
	// only check for size errors when no exclude patterns are provided
	if len(patterns) == 0 || len(excludePatterns) > 0 {
		return nil
	}

	// check if any original file paths existed but were skipped due to size
	for _, pattern := range patterns {
		if !fileExists(pattern) {
			continue
		}

		if tooLarge, fileSize := isFileTooLarge(pattern, maxFileSize); tooLarge {
			return fmt.Errorf("file '%s' exceeds the size limit of %d bytes (file size: %d bytes). Use --max-file-size flag to increase the limit", pattern, maxFileSize, fileSize)
		}
	}

	return nil
}

// processBashStylePattern handles patterns with ** using the doublestar library
func processBashStylePattern(pattern string, matchedFiles map[string]struct{}, maxFileSize int64) error {
	fsys := os.DirFS(".")
	matches, err := doublestar.Glob(fsys, pattern)
	if err != nil {
		return fmt.Errorf("failed to glob doublestar pattern %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		lgr.Printf("[WARN] no files matched pattern: %s", pattern)
		return nil
	}

	matchCount := 0
	for _, match := range matches {
		// convert back to absolute path
		absPath := filepath.Join(".", match)

		// check if it's a file
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("failed to stat file %s: %w", absPath, err)
		}

		if !info.IsDir() {
			// skip files that exceed the size limit
			if info.Size() > maxFileSize {
				lgr.Printf("[WARN] file %s exceeds size limit (%d bytes), skipping", absPath, info.Size())
				continue
			}

			matchedFiles[absPath] = struct{}{}
			matchCount++
		}
	}

	if matchCount == 0 {
		lgr.Printf("[WARN] no files matched after doublestar pattern: %s", pattern)
	} else {
		lgr.Printf("[DEBUG] matched %d files for pattern: %s", matchCount, pattern)
	}

	return nil
}

// processGoStylePattern handles patterns with /... using filepath.Walk
func processGoStylePattern(pattern string, matchedFiles map[string]struct{}, maxFileSize int64) error {
	basePath, filter := parseRecursivePattern(pattern)

	// check if base directory exists
	info, err := os.Stat(basePath)
	if err != nil || !info.IsDir() {
		lgr.Printf("[WARN] invalid base directory for pattern %s: %v", pattern, err)
		return nil
	}

	// walk the directory tree filtering by the specified pattern
	matchCount := 0
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip files that can't be accessed
		}

		if info.IsDir() || info.Size() > maxFileSize {
			if info.Size() > maxFileSize {
				lgr.Printf("[WARN] file %s exceeds size limit (%d bytes), skipping", path, info.Size())
			}
			return nil
		}

		if filter == "" || (strings.HasPrefix(filter, "*.") && strings.HasSuffix(path, filter[1:])) {
			matchedFiles[path] = struct{}{}
			matchCount++
			return nil
		}

		if matched, _ := filepath.Match(filter, filepath.Base(path)); matched {
			matchedFiles[path] = struct{}{}
			matchCount++
		}
		return nil
	})

	if err != nil {
		lgr.Printf("[WARN] failed to walk directory for pattern %s: %v", pattern, err)
	}

	if matchCount == 0 {
		lgr.Printf("[WARN] no files matched pattern: %s", pattern)
	} else {
		lgr.Printf("[DEBUG] matched %d files for pattern: %s", matchCount, pattern)
	}

	return nil
}

// processStandardGlobPattern handles standard glob patterns using filepath.Glob
func processStandardGlobPattern(pattern string, matchedFiles map[string]struct{}, maxFileSize int64) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob pattern %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		lgr.Printf("[WARN] no files matched pattern: %s", pattern)
		return nil
	}

	matchCount := 0
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			return fmt.Errorf("failed to stat file %s: %w", match, err)
		}

		if info.IsDir() {
			// handle directories by walking them recursively
			dirMatchCount := 0
			err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || info.Size() > maxFileSize {
					if err == nil && info.Size() > maxFileSize {
						lgr.Printf("[WARN] file %s exceeds size limit (%d bytes), skipping", path, info.Size())
					}
					return nil
				}
				matchedFiles[path] = struct{}{}
				dirMatchCount++
				return nil
			})

			if err != nil {
				lgr.Printf("[WARN] failed to walk directory %s: %v", match, err)
			}
			matchCount += dirMatchCount
			continue
		}

		// skip files that exceed the size limit
		if info.Size() > maxFileSize {
			lgr.Printf("[WARN] file %s exceeds size limit (%d bytes), skipping", match, info.Size())
			continue
		}

		matchedFiles[match] = struct{}{}
		matchCount++
	}

	if matchCount == 0 {
		lgr.Printf("[WARN] no files matched after directory traversal: %s", pattern)
	} else {
		lgr.Printf("[DEBUG] matched %d files for pattern: %s", matchCount, pattern)
	}

	return nil
}

// getSortedFiles returns a sorted slice of filenames from the map
func getSortedFiles(matchedFiles map[string]struct{}) []string {
	sortedFiles := make([]string, 0, len(matchedFiles))
	for file := range matchedFiles {
		sortedFiles = append(sortedFiles, file)
	}
	sort.Strings(sortedFiles)
	return sortedFiles
}

// formatFileContents creates a formatted string with file contents and appropriate headers
func formatFileContents(files []string) (string, error) {
	var sb strings.Builder
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	for _, file := range files {
		content, err := os.ReadFile(file) // #nosec G304 - file paths are validated earlier
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", file, err)
		}

		// get relative path if possible, otherwise use absolute
		relPath, err := filepath.Rel(cwd, file)
		if err != nil {
			relPath = file
		}

		// determine the appropriate comment style based on file extension
		fileHeader := getFileHeader(relPath)

		sb.WriteString(fileHeader)
		sb.Write(content)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}

// prepareExcludePatterns combines and deduplicates all exclude patterns
func prepareExcludePatterns(excludePatterns []string) []string {
	// estimate capacity for the combined patterns
	totalCapacity := len(excludePatterns) + len(commonIgnorePatterns)
	gitIgnorePatterns := loadGitIgnorePatterns()
	totalCapacity += len(gitIgnorePatterns)

	// pre-allocate slice with sufficient capacity
	allPatterns := make([]string, 0, totalCapacity)

	// user-provided exclude patterns have highest priority
	allPatterns = append(allPatterns, excludePatterns...)

	// add common ignore patterns
	allPatterns = append(allPatterns, commonIgnorePatterns...)

	// add patterns from .gitignore
	allPatterns = append(allPatterns, gitIgnorePatterns...)

	// deduplicate patterns to avoid redundant processing
	return deduplicatePatterns(allPatterns)
}

// deduplicatePatterns removes duplicate patterns while preserving order
func deduplicatePatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return patterns
	}

	seen := make(map[string]struct{}, len(patterns))
	deduped := make([]string, 0, len(patterns))

	for _, pattern := range patterns {
		if _, ok := seen[pattern]; !ok {
			seen[pattern] = struct{}{}
			deduped = append(deduped, pattern)
		}
	}

	return deduped
}

// applyExcludePatterns removes files that match any of the exclude patterns from the matched files
func applyExcludePatterns(matchedFiles map[string]struct{}, excludePatterns []string) map[string]struct{} {
	if len(excludePatterns) == 0 {
		return matchedFiles
	}

	cwd, err := os.Getwd()
	if err != nil {
		lgr.Printf("[WARN] failed to get current working directory: %v", err)
		return matchedFiles // return original map if we can't get the working directory
	}

	// track how many files were excluded per pattern
	patternExcludeCount := make(map[string]int)
	for _, pattern := range excludePatterns {
		patternExcludeCount[pattern] = 0
	}

	// create a new map to store the filtered results
	filteredFiles := make(map[string]struct{})

	// process each file and check if it should be excluded
	for filePath := range matchedFiles {
		if shouldExcludeFile(filePath, cwd, excludePatterns, patternExcludeCount) {
			continue
		}
		// file didn't match any exclude pattern, keep it
		filteredFiles[filePath] = struct{}{}
	}

	logExclusionResults(matchedFiles, filteredFiles, patternExcludeCount)
	return filteredFiles
}

// shouldExcludeFile checks if a file should be excluded based on the exclude patterns
func shouldExcludeFile(filePath, cwd string, excludePatterns []string, patternExcludeCount map[string]int) bool {
	// get the relative path for pattern matching
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		// if we can't get a relative path, use the absolute path
		relPath = filePath
	}

	for _, pattern := range excludePatterns {
		if matchesPattern(pattern, filePath, relPath) {
			patternExcludeCount[pattern]++
			return true
		}
	}

	return false
}

// matchesPattern checks if a file matches a specific exclude pattern
func matchesPattern(pattern, filePath, relPath string) bool {
	// handle bash-style patterns with **
	if strings.Contains(pattern, "**") {
		matched, err := doublestar.Match(pattern, relPath)
		if err != nil {
			lgr.Printf("[WARN] error matching exclude pattern %s: %v", pattern, err)
			return false
		}
		return matched
	}

	// handle Go-style recursive patterns
	if strings.Contains(pattern, "/...") {
		return matchesGoStylePattern(pattern, filePath)
	}

	// handle standard glob patterns
	matched, err := filepath.Match(pattern, filepath.Base(filePath))
	if err != nil {
		lgr.Printf("[WARN] error matching exclude pattern %s: %v", pattern, err)
		return false
	}

	return matched
}

// matchesGoStylePattern checks if a file matches a Go-style recursive pattern
func matchesGoStylePattern(pattern, filePath string) bool {
	basePath, filter := parseRecursivePattern(pattern)

	// check if the file is under the base path
	if !strings.HasPrefix(filePath, basePath) {
		return false
	}

	// if there's no filter, exclude all files under basePath
	if filter == "" {
		return true
	}

	// extension filter
	if strings.HasPrefix(filter, "*.") {
		ext := filter[1:] // remove * prefix
		return strings.HasSuffix(filePath, ext)
	}

	// standard glob pattern for filename
	matched, _ := filepath.Match(filter, filepath.Base(filePath))
	return matched
}

// logExclusionResults logs statistics about excluded files
func logExclusionResults(matchedFiles, filteredFiles map[string]struct{}, patternExcludeCount map[string]int) {
	totalExcluded := len(matchedFiles) - len(filteredFiles)
	if totalExcluded > 0 {
		lgr.Printf("[DEBUG] excluded %d files in total", totalExcluded)
		for pattern, count := range patternExcludeCount {
			if count > 0 {
				lgr.Printf("[DEBUG] pattern %s excluded %d files", pattern, count)
			}
		}
	}
}

// parseRecursivePattern parses a Go-style recursive pattern like "pkg/..." or "cmd/.../*.go"
// returns basePath and filter (file extension or pattern to match)
func parseRecursivePattern(pattern string) (basePath, filter string) {
	// split at /...
	parts := strings.SplitN(pattern, "/...", 2)
	basePath = parts[0]
	filter = ""

	// check if there's a filter after /...
	if len(parts) > 1 && parts[1] != "" {
		// pattern like pkg/.../*.go
		if strings.HasPrefix(parts[1], "/") {
			filter = parts[1][1:] // remove leading slash
		} else {
			filter = parts[1]
		}
	}

	return basePath, filter
}

// commonIgnorePatterns defines patterns for directories and files that should always be ignored
// regardless of .gitignore files
var commonIgnorePatterns = []string{
	// Version control
	"**/.git/**", // Git
	"**/.svn/**", // Subversion
	"**/.hg/**",  // Mercurial
	"**/.bzr/**", // Bazaar

	// Build outputs and dependencies
	"**/vendor/**",       // Go vendor
	"**/node_modules/**", // Node.js
	"**/.venv/**",        // Python virtual environments
	"**/venv/**",         // Python virtual environments
	"**/__pycache__/**",  // Python cache
	"**/*.pyc",           // Python compiled files
	"**/target/**",       // Rust, Maven
	"**/dist/**",         // Many build systems
	"**/build/**",        // Many build systems
	"**/.gradle/**",      // Gradle

	// IDE and editor files
	"**/.idea/**",   // JetBrains IDEs
	"**/.vscode/**", // Visual Studio Code
	"**/.vs/**",     // Visual Studio

	// Logs and metadata files
	"**/logs/**",   // Log directories
	"**/*.log",     // Log files
	"**/.DS_Store", // macOS metadata
	"**/Thumbs.db", // Windows thumbnails
	// Note: We don't exclude /tmp as it's often used in tests
}

// gitignoreFile is the name of the Git ignore file
const gitignoreFile = ".gitignore"

// maxGitignoreSize is the maximum size of a .gitignore file to process (1MB)
const maxGitignoreSize = 1 * 1024 * 1024

// loadGitIgnorePatterns reads the .gitignore file in the current directory
// and converts its patterns to glob patterns compatible with our exclude system.
// Note: Only top-level .gitignore is processed. Nested .gitignore files are not supported.
// Negation patterns (patterns starting with !) are not supported.
func loadGitIgnorePatterns() []string {
	// check if .gitignore exists and is accessible
	fileInfo, err := os.Stat(gitignoreFile)
	if err != nil {
		if !os.IsNotExist(err) {
			lgr.Printf("[DEBUG] error accessing .gitignore: %v", err)
		}
		return nil
	}

	// check file size limit
	if fileInfo.Size() > maxGitignoreSize {
		lgr.Printf("[WARN] .gitignore file exceeds maximum size limit of %d bytes, ignoring", maxGitignoreSize)
		return nil
	}

	// try to read .gitignore from current directory
	data, err := os.ReadFile(gitignoreFile)
	if err != nil {
		lgr.Printf("[DEBUG] error reading .gitignore: %v", err)
		return nil
	}

	// pre-allocate slice with reasonable capacity
	lines := strings.Split(string(data), "\n")
	patterns := make([]string, 0, len(lines))

	// process each line from .gitignore
	for i, line := range lines {
		pattern := convertGitIgnorePattern(line, i+1)
		if pattern != "" {
			patterns = append(patterns, pattern)
		}
	}

	if len(patterns) > 0 {
		lgr.Printf("[DEBUG] loaded %d patterns from .gitignore", len(patterns))
	}

	return patterns
}

// convertGitIgnorePattern converts a single .gitignore pattern to a glob pattern
// returns empty string for patterns that should be skipped
func convertGitIgnorePattern(line string, lineNum int) string {
	// skip empty lines and comments
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}

	// handle negation (!) - not supported currently
	if strings.HasPrefix(line, "!") {
		lgr.Printf("[WARN] .gitignore negation pattern not supported at line %d: %s", lineNum, line)
		return ""
	}

	// handle / prefix - pattern relative to the root directory
	line = strings.TrimPrefix(line, "/")

	// add ** to make pattern recursive if needed (only for basic patterns without /)
	if !strings.Contains(line, "**") && !strings.Contains(line, "/") {
		line = "**/" + line
	}

	// handle directory-only patterns (ending with /)
	if strings.HasSuffix(line, "/") {
		line += "**"
	}

	// add ** to the beginning of the pattern if it doesn't already have / or **
	if !strings.HasPrefix(line, "**") && !strings.Contains(line, "/") {
		line = "**/" + line
	}

	return line
}

// getFileHeader returns an appropriate comment header for a file based on its extension
func getFileHeader(filePath string) string {
	ext := filepath.Ext(filePath)

	// define comment styles for different file types
	// special case for Makefile which has no extension
	if strings.HasSuffix(filePath, "Makefile") || strings.HasSuffix(filePath, "makefile") {
		return fmt.Sprintf("# file: %s\n", filePath)
	}

	switch ext {
	// hash-style comments (#)
	case ".py", ".rb", ".pl", ".pm", ".sh", ".bash", ".zsh", ".fish", ".tcl", ".r",
		".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg", ".properties", ".mk", ".makefile":
		return fmt.Sprintf("# file: %s\n", filePath)

	// Double-slash comments (//)
	case ".js", ".ts", ".jsx", ".tsx", ".java", ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp",
		".hxx", ".cs", ".php", ".go", ".swift", ".kt", ".rs", ".scala", ".dart", ".groovy", ".d":
		return fmt.Sprintf("// file: %s\n", filePath)

	// HTML/XML style comments
	case ".html", ".xml", ".svg", ".xaml", ".jsp", ".asp", ".aspx", ".jsf", ".vue":
		return fmt.Sprintf("<!-- file: %s -->\n", filePath)

	// CSS style comments
	case ".css", ".scss", ".sass", ".less":
		return fmt.Sprintf("/* file: %s */\n", filePath)

	// SQL comments
	case ".sql":
		return fmt.Sprintf("-- file: %s\n", filePath)

	// lisp/Clojure comments
	case ".lisp", ".cl", ".el", ".clj", ".cljs", ".cljc":
		return fmt.Sprintf(";; file: %s\n", filePath)

	// haskell/VHDL comments
	case ".hs", ".lhs", ".vhdl", ".vhd":
		return fmt.Sprintf("-- file: %s\n", filePath)

	// PowerShell comments
	case ".ps1", ".psm1", ".psd1":
		return fmt.Sprintf("# file: %s\n", filePath)

	// batch file comments
	case ".bat", ".cmd":
		return fmt.Sprintf(":: file: %s\n", filePath)

	// fortran comments
	case ".f", ".f90", ".f95", ".f03":
		return fmt.Sprintf("! file: %s\n", filePath)

	// Default to // for unknown types
	default:
		return fmt.Sprintf("// file: %s\n", filePath)
	}
}

// fileExists checks if a file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// isFileTooLarge checks if a file's size exceeds the maximum allowed size
// returns true if file is too large and the actual file size
func isFileTooLarge(path string, maxFileSize int64) (tooLarge bool, fileSize int64) {
	info, err := os.Stat(path)
	if err != nil {
		return false, 0
	}
	return info.Size() > maxFileSize, info.Size()
}
