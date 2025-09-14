package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		// plain numbers
		{"zero", "0", 0, ""},
		{"positive number", "1024", 1024, ""},
		{"large number", "1073741824", 1073741824, ""},
		{"with whitespace", "  1024  ", 1024, ""},

		// k/kb suffixes
		{"k suffix lowercase", "1k", 1024, ""},
		{"k suffix uppercase", "1K", 1024, ""},
		{"kb suffix lowercase", "1kb", 1024, ""},
		{"kb suffix uppercase", "1KB", 1024, ""},
		{"kb suffix mixed case", "1Kb", 1024, ""},
		{"multiple k", "16k", 16384, ""},
		{"64k", "64k", 65536, ""},

		// m/mb suffixes
		{"m suffix lowercase", "1m", 1048576, ""},
		{"m suffix uppercase", "1M", 1048576, ""},
		{"mb suffix lowercase", "1mb", 1048576, ""},
		{"mb suffix uppercase", "1MB", 1048576, ""},
		{"mb suffix mixed case", "1Mb", 1048576, ""},
		{"multiple m", "32m", 33554432, ""},

		// g/gb suffixes
		{"g suffix lowercase", "1g", 1073741824, ""},
		{"g suffix uppercase", "1G", 1073741824, ""},
		{"gb suffix lowercase", "1gb", 1073741824, ""},
		{"gb suffix uppercase", "1GB", 1073741824, ""},
		{"gb suffix mixed case", "1Gb", 1073741824, ""},
		{"multiple g", "2g", 2147483648, ""},

		// edge cases with whitespace
		{"whitespace before suffix", "  16k", 16384, ""},
		{"whitespace after suffix", "16k  ", 16384, ""},
		{"whitespace both sides", "  16k  ", 16384, ""},

		// error cases
		{"empty string", "", 0, "empty size value"},
		{"only whitespace", "   ", 0, "empty size value"},
		{"negative number", "-1", 0, "size cannot be negative"},
		{"negative with suffix", "-1k", 0, "size cannot be negative"},
		{"invalid suffix", "1x", 0, "invalid suffix"},
		{"invalid suffix tb", "1tb", 0, "invalid suffix"},
		{"non-numeric", "abc", 0, "invalid suffix"},
		{"non-numeric with valid suffix", "abck", 0, "invalid numeric value"},
		{"decimal number", "1.5k", 0, "invalid numeric value"},
		{"suffix only", "k", 0, "invalid numeric value"},
		{"multiple suffixes", "1km", 0, "invalid numeric value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseSize_Overflow(t *testing.T) {
	// test overflow detection
	tests := []struct {
		name  string
		input string
	}{
		{"max int64 overflow with g", "9223372036854775807g"}, // max int64 * 1G would overflow
		{"large number with g", "10000000000g"},               // clearly overflows
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSize(tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "overflow")
		})
	}
}

func TestParseSize_BoundaryValues(t *testing.T) {
	// test that we can parse values that fit in int64
	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"8GB fits in int64", "8g", 8 * 1024 * 1024 * 1024},
		{"1000000 plain", "1000000", 1000000},
		{"max int32 as plain number", "2147483647", 2147483647},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSize_SafeIntConversion(t *testing.T) {
	// demonstrate safe conversion to int for use in MaxTokens
	t.Run("safe conversion within int range", func(t *testing.T) {
		size, err := ParseSize("16k")
		require.NoError(t, err)

		// safe conversion with bounds check
		if size > math.MaxInt32 {
			t.Fatal("value too large for int32")
		}
		intValue := int(size)
		assert.Equal(t, 16384, intValue)
	})

	t.Run("detect overflow for int32", func(t *testing.T) {
		size, err := ParseSize("3g") // 3GB > MaxInt32
		require.NoError(t, err)

		// this would overflow int32
		assert.Greater(t, size, int64(math.MaxInt32))
	})
}
