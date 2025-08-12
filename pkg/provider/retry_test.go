package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/provider/mocks"
)

func TestRetryableProvider_NoRetry(t *testing.T) {
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "success", nil
		},
	}

	// with attempts=1, should return original provider
	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 1,
	})

	// should be the same provider (no wrapping)
	assert.Equal(t, mock, wrapped)
}

func TestRetryableProvider_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return "success", nil
		},
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 3,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
		Factor:   2,
	})

	ctx := context.Background()
	result, err := wrapped.Generate(ctx, "test prompt")

	require.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 1, callCount) // should only call once
}

func TestRetryableProvider_RetryOnTransientError(t *testing.T) {
	tests := []struct {
		name          string
		errorMsg      string
		shouldRetry   bool
		expectedCalls int
	}{
		{
			name:          "rate limit error",
			errorMsg:      "429 rate limit exceeded",
			shouldRetry:   true,
			expectedCalls: 3,
		},
		{
			name:          "timeout error",
			errorMsg:      "request timeout",
			shouldRetry:   true,
			expectedCalls: 3,
		},
		{
			name:          "service unavailable",
			errorMsg:      "503 service unavailable",
			shouldRetry:   true,
			expectedCalls: 3,
		},
		{
			name:          "authentication error",
			errorMsg:      "401 authentication failed",
			shouldRetry:   false,
			expectedCalls: 1,
		},
		{
			name:          "bad request",
			errorMsg:      "400 bad request",
			shouldRetry:   false,
			expectedCalls: 1,
		},
		{
			name:          "token limit",
			errorMsg:      "maximum context length exceeded",
			shouldRetry:   false,
			expectedCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			mock := &mocks.ProviderMock{
				NameFunc:    func() string { return "test" },
				EnabledFunc: func() bool { return true },
				GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
					callCount++
					// fail first 2 attempts
					if callCount <= 2 {
						return "", errors.New(tt.errorMsg)
					}
					return "success", nil
				},
			}

			wrapped := NewRetryableProvider(mock, RetryOptions{
				Attempts: 3,
				Delay:    10 * time.Millisecond,
				MaxDelay: 100 * time.Millisecond,
				Factor:   2,
			})

			ctx := context.Background()
			result, err := wrapped.Generate(ctx, "test prompt")

			if tt.shouldRetry {
				// should eventually succeed
				require.NoError(t, err)
				assert.Equal(t, "success", result)
			} else {
				// should fail immediately
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			}

			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestRetryableProvider_ExhaustedRetries(t *testing.T) {
	callCount := 0
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return "", errors.New("500 internal server error")
		},
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 3,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
		Factor:   2,
	})

	ctx := context.Background()
	result, err := wrapped.Generate(ctx, "test prompt")

	require.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "500 internal server error")
	assert.Equal(t, 3, callCount) // should try all attempts
}

func TestRetryableProvider_ContextCancellation(t *testing.T) {
	callCount := 0
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return "", errors.New("500 internal server error")
		},
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 5,
		Delay:    100 * time.Millisecond,
		MaxDelay: 1 * time.Second,
		Factor:   2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	result, err := wrapped.Generate(ctx, "test prompt")

	require.Error(t, err)
	assert.Empty(t, result)
	// should have tried at least once but not all attempts
	assert.GreaterOrEqual(t, callCount, 1)
	assert.Less(t, callCount, 5)
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		// retryable errors
		{"rate limit", errors.New("429 too many requests"), true},
		{"rate limit text", errors.New("rate limit exceeded"), true},
		{"server error", errors.New("500 internal server error"), true},
		{"bad gateway", errors.New("502 bad gateway"), true},
		{"service unavailable", errors.New("503 service unavailable"), true},
		{"gateway timeout", errors.New("504 gateway timeout"), true},
		{"timeout", errors.New("request timeout"), true},
		{"deadline", errors.New("context deadline exceeded"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"resource exhausted", errors.New("resource exhausted"), true},

		// non-retryable errors
		{"auth error", errors.New("401 unauthorized"), false},
		{"authentication", errors.New("authentication failed"), false},
		{"bad request", errors.New("400 bad request"), false},
		{"invalid request", errors.New("invalid request format"), false},
		{"not found", errors.New("model not found"), false},
		{"context length", errors.New("maximum context length exceeded"), false},
		{"token limit", errors.New("token limit reached"), false},
		{"context canceled", errors.New("context canceled"), false},
		{"model error", errors.New("model configuration error"), false},

		// edge cases
		{"nil error", nil, false},
		{"empty error", errors.New(""), false},
		{"model timeout", errors.New("model request timeout"), true}, // model with timeout is retryable
		{"deadline canceled", errors.New("context deadline exceeded: context canceled"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result, "error: %v", tt.err)
		})
	}
}

func TestWrapProviderWithRetry(t *testing.T) {
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
	}

	// no retry when attempts <= 1
	wrapped := WrapProviderWithRetry(mock, RetryOptions{Attempts: 1})
	assert.Equal(t, mock, wrapped)

	// wrapping when attempts > 1
	wrapped = WrapProviderWithRetry(mock, RetryOptions{
		Attempts: 3,
		Delay:    2 * time.Second,
		MaxDelay: 30 * time.Second,
		Factor:   3,
	})
	_, ok := wrapped.(*RetryableProvider)
	assert.True(t, ok)

	// default values when not specified
	wrapped = WrapProviderWithRetry(mock, RetryOptions{
		Attempts: 3,
		Delay:    0, // should default to 1s
		MaxDelay: 0, // should default to 30s
		Factor:   0, // should default to 2
	})
	_, ok = wrapped.(*RetryableProvider)
	assert.True(t, ok)
}

func TestWrapProvidersWithRetry(t *testing.T) {
	providers := []Provider{
		&mocks.ProviderMock{
			NameFunc:    func() string { return "provider1" },
			EnabledFunc: func() bool { return true },
		},
		&mocks.ProviderMock{
			NameFunc:    func() string { return "provider2" },
			EnabledFunc: func() bool { return true },
		},
		&mocks.ProviderMock{
			NameFunc:    func() string { return "provider3" },
			EnabledFunc: func() bool { return false },
		},
	}

	// no wrapping when attempts <= 1
	wrapped := WrapProvidersWithRetry(providers, RetryOptions{Attempts: 1})
	assert.Equal(t, providers, wrapped)

	// all providers wrapped when attempts > 1
	wrapped = WrapProvidersWithRetry(providers, RetryOptions{
		Attempts: 3,
		Delay:    1 * time.Second,
		MaxDelay: 30 * time.Second,
		Factor:   2,
	})

	assert.Len(t, wrapped, 3)
	for i, p := range wrapped {
		_, ok := p.(*RetryableProvider)
		assert.True(t, ok, "provider %d should be wrapped", i)
	}
}

func TestRetryableProvider_Properties(t *testing.T) {
	enabled := true
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "TestProvider" },
		EnabledFunc: func() bool { return enabled },
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 3,
		Delay:    1 * time.Second,
		MaxDelay: 30 * time.Second,
		Factor:   2,
	})

	// should preserve provider properties
	assert.Equal(t, "TestProvider", wrapped.Name())
	assert.True(t, wrapped.Enabled())

	// test with disabled provider
	enabled = false
	assert.False(t, wrapped.Enabled())
}

func TestRetryableProvider_MultipleResponses(t *testing.T) {
	callCount := 0
	responses := []string{"response 1", "response 2", "response 3"}

	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			idx := callCount
			callCount++
			if idx >= len(responses) {
				idx = len(responses) - 1
			}
			return responses[idx], nil
		},
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 3,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
		Factor:   2,
	})

	ctx := context.Background()

	// first call
	result, err := wrapped.Generate(ctx, "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "response 1", result)

	// second call
	result, err = wrapped.Generate(ctx, "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "response 2", result)

	// third call
	result, err = wrapped.Generate(ctx, "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "response 3", result)

	// fourth call (should use last response)
	result, err = wrapped.Generate(ctx, "test prompt")
	require.NoError(t, err)
	assert.Equal(t, "response 3", result)
}

func TestRetryableProvider_FormattedError(t *testing.T) {
	callCount := 0
	mock := &mocks.ProviderMock{
		NameFunc:    func() string { return "test" },
		EnabledFunc: func() bool { return true },
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return "", fmt.Errorf("500 error on attempt %d", callCount)
		},
	}

	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 2,
		Delay:    10 * time.Millisecond,
		MaxDelay: 100 * time.Millisecond,
		Factor:   2,
	})

	ctx := context.Background()
	result, err := wrapped.Generate(ctx, "test prompt")

	require.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "500 error on attempt 2") // should return last error
	assert.Equal(t, 2, callCount)
}
