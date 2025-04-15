package provider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResult_Format(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected string
	}{
		{
			name: "success result",
			result: Result{
				Provider: "TestProvider",
				Text:     "This is a test response",
				Error:    nil,
			},
			expected: "== generated by TestProvider ==\nThis is a test response\n",
		},
		{
			name: "error result",
			result: Result{
				Provider: "TestProvider",
				Text:     "",
				Error:    assert.AnError,
			},
			expected: "== generated by TestProvider ==\nassert.AnError general error for testing\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := tt.result.Format()
			assert.Equal(t, tt.expected, formatted)
		})
	}
}

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name           string
		inputErr       error
		expectedResult string
		shouldSanitize bool
	}{
		{
			name:           "nil error",
			inputErr:       nil,
			expectedResult: "",
			shouldSanitize: false,
		},
		{
			name:           "normal error",
			inputErr:       errors.New("normal error message"),
			expectedResult: "normal error message",
			shouldSanitize: false,
		},
		{
			name:           "error with API key",
			inputErr:       errors.New("error with api_key=1234567890"),
			expectedResult: "API error: the original error was redacted because it may contain sensitive information",
			shouldSanitize: true,
		},
		{
			name:           "error with bearer token",
			inputErr:       errors.New("error with bearer authentication"),
			expectedResult: "API error: the original error was redacted because it may contain sensitive information",
			shouldSanitize: true,
		},
		{
			name:     "error with provider prefix",
			inputErr: errors.New("openai api error: key expired"),
			// this example triggers "key" pattern detection
			expectedResult: "openai API error: the original error was redacted because it may contain sensitive information",
			shouldSanitize: true,
		},
		{
			name:           "error with URL containing token",
			inputErr:       errors.New("request to https://api.example.com/v1?access_token=abc123 failed"),
			expectedResult: "API error: the original error was redacted because it may contain sensitive information",
			shouldSanitize: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeError(tt.inputErr)

			if tt.inputErr == nil {
				assert.NoError(t, result)
				return
			}

			if tt.shouldSanitize {
				assert.Contains(t, result.Error(), "redacted")
				if tt.expectedResult != "" {
					assert.Equal(t, tt.expectedResult, result.Error())
				}
			} else {
				assert.Equal(t, tt.inputErr.Error(), result.Error())
			}
		})
	}
}
