package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAI_Name(t *testing.T) {
	provider := NewOpenAI(Options{})
	assert.Equal(t, "OpenAI", provider.Name())
}

func TestOpenAI_Enabled(t *testing.T) {
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
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOpenAI(tt.options)
			assert.Equal(t, tt.expected, provider.Enabled())
		})
	}
}

func TestOpenAI_Generate_NotEnabled(t *testing.T) {
	provider := NewOpenAI(Options{Enabled: false})
	_, err := provider.Generate(context.Background(), "test prompt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}