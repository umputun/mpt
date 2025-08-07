package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider for testing retry logic
type mockProvider struct {
	name          string
	enabled       bool
	callCount     int
	failUntil     int
	errorToReturn error
	responses     []string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Generate(ctx context.Context, prompt string) (string, error) {
	m.callCount++

	// if errorToReturn is set and we haven't reached the success point
	if m.errorToReturn != nil {
		// if failUntil is set, fail until that attempt
		if m.failUntil > 0 && m.callCount <= m.failUntil {
			return "", m.errorToReturn
		}
		// if failUntil is 0, always fail
		if m.failUntil == 0 {
			return "", m.errorToReturn
		}
	}

	// return response based on call count
	if len(m.responses) > 0 {
		idx := m.callCount - 1
		if idx >= len(m.responses) {
			idx = len(m.responses) - 1
		}
		return m.responses[idx], nil
	}

	return fmt.Sprintf("response %d", m.callCount), nil
}

func (m *mockProvider) Enabled() bool {
	return m.enabled
}

func TestRetryableProvider_NoRetry(t *testing.T) {
	mock := &mockProvider{
		name:      "test",
		enabled:   true,
		responses: []string{"success"},
	}

	// with attempts=1, should return original provider
	wrapped := NewRetryableProvider(mock, RetryOptions{
		Attempts: 1,
	})

	// should be the same provider (no wrapping)
	assert.Equal(t, mock, wrapped)
}

func TestRetryableProvider_SuccessOnFirstAttempt(t *testing.T) {
	mock := &mockProvider{
		name:      "test",
		enabled:   true,
		responses: []string{"success"},
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
	assert.Equal(t, 1, mock.callCount) // should only call once
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
			mock := &mockProvider{
				name:          "test",
				enabled:       true,
				failUntil:     2, // fail first 2 attempts
				errorToReturn: errors.New(tt.errorMsg),
				responses:     []string{"success"},
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

			assert.Equal(t, tt.expectedCalls, mock.callCount)
		})
	}
}

func TestRetryableProvider_ExhaustedRetries(t *testing.T) {
	mock := &mockProvider{
		name:          "test",
		enabled:       true,
		errorToReturn: errors.New("500 internal server error"),
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
	assert.Equal(t, 3, mock.callCount) // should try all attempts
}

func TestRetryableProvider_ContextCancellation(t *testing.T) {
	mock := &mockProvider{
		name:          "test",
		enabled:       true,
		errorToReturn: errors.New("500 internal server error"),
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
	assert.GreaterOrEqual(t, mock.callCount, 1)
	assert.Less(t, mock.callCount, 5)
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
	mock := &mockProvider{
		name:    "test",
		enabled: true,
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
		&mockProvider{name: "provider1", enabled: true},
		&mockProvider{name: "provider2", enabled: true},
		&mockProvider{name: "provider3", enabled: false},
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
	mock := &mockProvider{
		name:    "TestProvider",
		enabled: true,
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
	mock.enabled = false
	assert.False(t, wrapped.Enabled())
}
