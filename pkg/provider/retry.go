package provider

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/repeater/v2"
)

// RetryableProvider wraps a provider with retry logic for transient failures.
// The wrapped provider must be safe for concurrent use if the RetryableProvider
// will be used concurrently. The retry logic itself is thread-safe.
type RetryableProvider struct {
	provider Provider
	repeater *repeater.Repeater
	name     string
}

// RetryOptions configures retry behavior
type RetryOptions struct {
	Attempts int
	Delay    time.Duration
	MaxDelay time.Duration
	Factor   float64
}

// NewRetryableProvider creates a provider wrapper with retry logic
func NewRetryableProvider(p Provider, opts RetryOptions) Provider {
	// if attempts is 1 or less, no retries needed
	if opts.Attempts <= 1 {
		return p
	}

	// create repeater with exponential backoff
	// the Factor controls the backoff type:
	// 1.0 = constant delay (fixed)
	// > 1.0 = exponential backoff
	var rep *repeater.Repeater

	if opts.Factor <= 1.0 {
		// fixed delay strategy
		rep = repeater.NewFixed(opts.Attempts, opts.Delay)
	} else {
		// exponential backoff with jitter
		rep = repeater.NewBackoff(opts.Attempts, opts.Delay,
			repeater.WithMaxDelay(opts.MaxDelay),
			repeater.WithBackoffType(repeater.BackoffExponential),
			repeater.WithJitter(0.1), // 10% jitter to avoid thundering herd
		)
	}

	// set error classifier to determine retryable errors
	rep.SetErrorClassifier(isRetryableError)

	return &RetryableProvider{
		provider: p,
		repeater: rep,
		name:     p.Name(),
	}
}

// Name returns the provider name
func (r *RetryableProvider) Name() string {
	return r.name
}

// Generate sends a prompt to the provider with retry logic
func (r *RetryableProvider) Generate(ctx context.Context, prompt string) (string, error) {
	var result string
	var attempt int32

	err := r.repeater.Do(ctx, func() error {
		currentAttempt := atomic.AddInt32(&attempt, 1)
		text, err := r.provider.Generate(ctx, prompt)
		if err != nil {
			// log based on error type (classifier will handle retry decision)
			if !isRetryableError(err) {
				lgr.Printf("[DEBUG] %s: non-retryable error on attempt %d: %v", r.name, currentAttempt, err)
			} else {
				lgr.Printf("[INFO] %s: retryable error on attempt %d: %v", r.name, currentAttempt, err)
			}
			return err
		}

		result = text
		return nil
	})

	if err != nil {
		return "", err
	}

	stats := r.repeater.Stats()
	if stats.Attempts > 1 {
		lgr.Printf("[INFO] %s: succeeded after %d attempts (total duration: %v)",
			r.name, stats.Attempts, stats.TotalDuration)
	}

	return result, nil
}

// Enabled returns whether this provider is enabled
func (r *RetryableProvider) Enabled() bool {
	return r.provider.Enabled()
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// definitely retryable errors
	retryablePatterns := []string{
		"429",                // rate limit
		"rate limit",         // rate limit exceeded
		"500",                // internal server error
		"502",                // bad gateway
		"503",                // service unavailable
		"504",                // gateway timeout
		"timeout",            // request timeout
		"deadline exceeded",  // context deadline
		"connection refused", // network error
		"connection reset",   // network error
		"broken pipe",        // network error
		"temporary failure",  // generic temporary
		"resource exhausted", // quota/limit
	}

	// check for retryable patterns
	errLower := strings.ToLower(errStr)
	for _, pattern := range retryablePatterns {
		if strings.Contains(errLower, pattern) {
			// special case: context deadline could be from cancellation
			if pattern == "deadline exceeded" && strings.Contains(errLower, "context canceled") {
				return false // don't retry on explicit cancellation
			}
			return true
		}
	}

	// non-retryable errors
	nonRetryablePatterns := []string{
		"401",              // unauthorized
		"authentication",   // auth failed
		"400",              // bad request
		"invalid",          // invalid request/model/etc
		"not found",        // model not found
		"context length",   // token limit
		"token limit",      // token limit
		"maximum context",  // token limit
		"context canceled", // explicit cancellation
		"model",            // model issues (unless it's a timeout)
	}

	// check for non-retryable patterns
	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errLower, pattern) {
			// exception: if it contains both "model" and "timeout", it's retryable
			if pattern == "model" && strings.Contains(errLower, "timeout") {
				return true
			}
			return false
		}
	}

	// default to not retrying unknown errors
	return false
}

// WrapProviderWithRetry wraps a provider with retry logic if configured
func WrapProviderWithRetry(p Provider, opts RetryOptions) Provider {
	if opts.Attempts <= 1 {
		return p // no retry needed
	}

	// validate and set defaults
	if opts.Delay <= 0 {
		opts.Delay = time.Second
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 30 * time.Second
	}
	if opts.Factor <= 0 {
		opts.Factor = 2
	}
	// ensure max delay is at least as large as initial delay
	if opts.MaxDelay < opts.Delay {
		opts.MaxDelay = opts.Delay
	}

	lgr.Printf("[DEBUG] wrapping provider %s with retry: attempts=%d, delay=%v, max_delay=%v, factor=%.1f",
		p.Name(), opts.Attempts, opts.Delay, opts.MaxDelay, opts.Factor)

	return NewRetryableProvider(p, opts)
}

// WrapProvidersWithRetry wraps multiple providers with retry logic
func WrapProvidersWithRetry(providers []Provider, opts RetryOptions) []Provider {
	if opts.Attempts <= 1 {
		return providers // no retry needed
	}

	wrapped := make([]Provider, len(providers))
	for i, p := range providers {
		wrapped[i] = WrapProviderWithRetry(p, opts)
	}
	return wrapped
}
