package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses size values like "1024", "1k", "1K", "1kb", "1KB", "1m", "1M", "1mb", "1MB", "1g", "1G", "1gb", "1GB"
// Returns int64 to avoid overflow issues during parsing. Callers should check bounds before converting to int.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty size value")
	}

	// check for suffix and extract multiplier
	multiplier := int64(1)
	numericPart := s

	// check for two-letter suffixes first (kb, mb, gb)
	if len(s) >= 2 {
		suffix := s[len(s)-2:]
		switch suffix {
		case "kb":
			multiplier = 1024
			numericPart = s[:len(s)-2]
		case "mb":
			multiplier = 1024 * 1024
			numericPart = s[:len(s)-2]
		case "gb":
			multiplier = 1024 * 1024 * 1024
			numericPart = s[:len(s)-2]
		}
	}

	// if no two-letter suffix found, check for single-letter suffix
	if multiplier == 1 && len(s) >= 1 {
		suffix := s[len(s)-1]
		switch suffix {
		case 'k':
			multiplier = 1024
			numericPart = s[:len(s)-1]
		case 'm':
			multiplier = 1024 * 1024
			numericPart = s[:len(s)-1]
		case 'g':
			multiplier = 1024 * 1024 * 1024
			numericPart = s[:len(s)-1]
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			// no suffix, just a number
		default:
			return 0, fmt.Errorf("invalid suffix")
		}
	}

	// parse the numeric part
	n, err := strconv.ParseInt(numericPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value")
	}

	if n < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}

	// calculate result and check for overflow
	result := n * multiplier
	if n != 0 && result/n != multiplier {
		return 0, fmt.Errorf("size value overflow")
	}

	return result, nil
}
