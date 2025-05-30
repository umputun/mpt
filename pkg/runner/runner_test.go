package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/runner/mocks"
)

func TestRunner_Run(t *testing.T) {
	t.Run("all providers fail", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("provider 1 error")
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		provider2 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider2"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("provider 2 error")
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider1, provider2)
		result, err := runner.Run(context.Background(), "test prompt")

		// should now return an error if all providers fail
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all providers failed")
		// our implementation only returns one of the provider errors
		// due to goroutine scheduling, either provider error could be included
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "provider 1 error") ||
				strings.Contains(errorMsg, "provider 2 error"),
			"Error should contain one of the provider errors")
		assert.Empty(t, result)
	})
	t.Run("all providers successful", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Response 1", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		provider2 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider2"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Response 2", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider1, provider2)
		result, err := runner.Run(context.Background(), "test prompt")

		require.NoError(t, err)
		assert.Contains(t, result, "== generated by Provider1 ==")
		assert.Contains(t, result, "Response 1")
		assert.Contains(t, result, "== generated by Provider2 ==")
		assert.Contains(t, result, "Response 2")

		// verify the mock was called with expected parameters
		require.Len(t, provider1.GenerateCalls(), 1)
		assert.Equal(t, "test prompt", provider1.GenerateCalls()[0].Prompt)
		require.Len(t, provider2.GenerateCalls(), 1)
		assert.Equal(t, "test prompt", provider2.GenerateCalls()[0].Prompt)
	})

	t.Run("single provider skips header", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Response 1", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider1)
		result, err := runner.Run(context.Background(), "test prompt")

		require.NoError(t, err)
		assert.Equal(t, "Response 1", result)
		assert.NotContains(t, result, "== generated by Provider1 ==")

		// verify the mock was called with expected parameters
		require.Len(t, provider1.GenerateCalls(), 1)
		assert.Equal(t, "test prompt", provider1.GenerateCalls()[0].Prompt)
	})

	t.Run("single provider with error", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("test error")
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider1)
		result, err := runner.Run(context.Background(), "test prompt")

		// now expecting an actual error instead of error message as result
		require.Error(t, err)
		assert.Contains(t, err.Error(), "test error")
		assert.Empty(t, result)

		// verify the mock was called with expected parameters
		require.Len(t, provider1.GenerateCalls(), 1)
		assert.Equal(t, "test prompt", provider1.GenerateCalls()[0].Prompt)
	})

	t.Run("some providers fail", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "", errors.New("provider 1 error")
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		provider2 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider2"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Response 2", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider1, provider2)
		result, err := runner.Run(context.Background(), "test prompt")

		// still expect success because at least one provider succeeded
		require.NoError(t, err)
		// failed provider should not be in the output
		assert.NotContains(t, result, "== generated by Provider1 ==")
		assert.NotContains(t, result, "provider 1 error")
		// successful provider should be in the output
		assert.Contains(t, result, "== generated by Provider2 ==")
		assert.Contains(t, result, "Response 2")
	})

	t.Run("no enabled providers", func(t *testing.T) {
		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			EnabledFunc: func() bool {
				return false
			},
		}

		provider2 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider2"
			},
			EnabledFunc: func() bool {
				return false
			},
		}

		runner := New(provider1, provider2)
		_, err := runner.Run(context.Background(), "test prompt")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled providers")
	})

	t.Run("single provider with multiline response", func(t *testing.T) {
		provider := &mocks.ProviderMock{
			NameFunc: func() string {
				return "MultilineProvider"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				return "Line 1\nLine 2\nLine 3", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		runner := New(provider)
		result, err := runner.Run(context.Background(), "multiline test")

		require.NoError(t, err)
		assert.Equal(t, "Line 1\nLine 2\nLine 3", result)
		assert.NotContains(t, result, "== generated by MultilineProvider ==")

		// verify the mock was called with expected parameters
		require.Len(t, provider.GenerateCalls(), 1)
		assert.Equal(t, "multiline test", provider.GenerateCalls()[0].Prompt)
	})

	t.Run("providers complete out of order but results are ordered", func(t *testing.T) {
		// we'll simulate provider2 completing faster than provider1
		// but want to verify results still appear in original provider order (1, 2, 3)

		provider1 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider1"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				// simulate slow completion with a response
				return "Response 1", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		provider2 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider2"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				// simulate fastest completion with a response
				return "Response 2", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		provider3 := &mocks.ProviderMock{
			NameFunc: func() string {
				return "Provider3"
			},
			GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
				// simulate medium speed completion with a response
				return "Response 3", nil
			},
			EnabledFunc: func() bool {
				return true
			},
		}

		// create the runner with providers in order 1, 2, 3
		runner := New(provider1, provider2, provider3)

		// run with the test prompt
		result, err := runner.Run(context.Background(), "test prompt")
		require.NoError(t, err)

		// verify all providers were called
		require.Len(t, provider1.GenerateCalls(), 1)
		require.Len(t, provider2.GenerateCalls(), 1)
		require.Len(t, provider3.GenerateCalls(), 1)

		// get the raw results array to verify exact ordering
		results := runner.GetResults()
		require.Len(t, results, 3)

		// check that provider results are in the same order as providers were defined
		assert.Equal(t, "Provider1", results[0].Provider)
		assert.Equal(t, "Provider2", results[1].Provider)
		assert.Equal(t, "Provider3", results[2].Provider)

		// also verify the order in the formatted output
		// first check Provider1 appears before Provider2
		provider1Pos := strings.Index(result, "== generated by Provider1 ==")
		provider2Pos := strings.Index(result, "== generated by Provider2 ==")
		provider3Pos := strings.Index(result, "== generated by Provider3 ==")

		assert.GreaterOrEqual(t, provider1Pos, 0, "Provider1 header should be present in result")
		assert.GreaterOrEqual(t, provider2Pos, 0, "Provider2 header should be present in result")
		assert.GreaterOrEqual(t, provider3Pos, 0, "Provider3 header should be present in result")
		assert.Less(t, provider1Pos, provider2Pos, "Provider1 should appear before Provider2")
		assert.Less(t, provider2Pos, provider3Pos, "Provider2 should appear before Provider3")
	})
}
