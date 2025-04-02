package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoogle_Name(t *testing.T) {
	provider := NewGoogle(Options{})
	assert.Equal(t, "Google", provider.Name())
}

func TestGoogle_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		options  Options
		expected bool
	}{
		{
			name: "disabled without API key",
			options: Options{
				APIKey:  "",
				Enabled: true,
			},
			expected: false,
		},
		{
			name: "disabled explicitly",
			options: Options{
				APIKey:  "test-key",
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled with API key",
			options: Options{
				APIKey:  "test-key",
				Enabled: true,
				Model:   "gemini-1.5-pro",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGoogle(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestGoogle_Generate_NotEnabled(t *testing.T) {
	provider := NewGoogle(Options{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}