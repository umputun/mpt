package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/mcp/mocks"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name    string
		options ServerOptions
	}{
		{
			name: "creates server with options",
			options: ServerOptions{
				Name:    "Test Server",
				Version: "1.0.0",
			},
		},
		{
			name:    "creates server with default values",
			options: ServerOptions{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &mocks.RunnerMock{
				RunFunc: func(ctx context.Context, prompt string) (string, error) {
					return "test response", nil
				},
			}

			server := NewServer(runner, tc.options)

			// verify server was created properly
			assert.NotNil(t, server)
			assert.NotNil(t, server.mcpServer)
			assert.Equal(t, runner, server.runner)

			// test that tool is registered - we can only access this indirectly by
			// checking that we get a valid response from the handler
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{
				"prompt": "test prompt",
			}

			result, err := server.handleGenerateTool(context.Background(), request)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Content)

			// check that the runner was called with the prompt
			calls := runner.RunCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "test prompt", calls[0].Prompt)

			// verify that the response came back with the expected MCP format
			textContent, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok, "Expected TextContent")
			assert.Equal(t, "test response", textContent.Text)
		})
	}
}

func TestServer_handleGenerateTool(t *testing.T) {
	// define various runner behaviors
	successRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Generated response for: " + prompt, nil
		},
	}

	longResponseRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "This is a very long response with multiple paragraphs.\n\nIt contains newlines and special characters: !@#$%^&*().\n\nThe response should be properly handled and returned in the MCP protocol format.", nil
		},
	}

	errorRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("runner error")
		},
	}

	canceledContextRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			// simulate the context being canceled
			return "", context.Canceled
		},
	}

	tests := []struct {
		name           string
		runner         *mocks.RunnerMock
		arguments      map[string]any
		expectedPrompt string
		expectError    bool
		errorText      string
		checkResult    func(t *testing.T, result *mcp.CallToolResult)
	}{
		{
			name:           "valid prompt with short response",
			runner:         successRunner,
			arguments:      map[string]any{"prompt": "Test prompt"},
			expectedPrompt: "Test prompt",
			expectError:    false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				require.NotEmpty(t, result.Content)
				textContent, ok := result.Content[0].(mcp.TextContent)
				require.True(t, ok, "Expected TextContent")
				assert.Contains(t, textContent.Text, "Generated response for: Test prompt")
			},
		},
		{
			name:           "valid prompt with long response",
			runner:         longResponseRunner,
			arguments:      map[string]any{"prompt": "Long request"},
			expectedPrompt: "Long request",
			expectError:    false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				require.NotEmpty(t, result.Content)
				textContent, ok := result.Content[0].(mcp.TextContent)
				require.True(t, ok, "Expected TextContent")
				assert.Contains(t, textContent.Text, "This is a very long response")
				assert.Contains(t, textContent.Text, "multiple paragraphs")
				assert.Contains(t, textContent.Text, "special characters")
			},
		},
		{
			name:        "missing prompt parameter",
			runner:      successRunner,
			arguments:   map[string]any{},
			expectError: true,
			errorText:   "required argument \"prompt\" not found",
		},
		{
			name:        "wrong prompt type",
			runner:      successRunner,
			arguments:   map[string]any{"prompt": 123}, // not a string
			expectError: true,
			errorText:   "argument \"prompt\" is not a string",
		},
		{
			name:           "runner error",
			runner:         errorRunner,
			arguments:      map[string]any{"prompt": "Test prompt"},
			expectedPrompt: "Test prompt",
			expectError:    true,
			errorText:      "failed to run prompt through MPT",
		},
		{
			name:           "canceled context",
			runner:         canceledContextRunner,
			arguments:      map[string]any{"prompt": "Cancel me"},
			expectedPrompt: "Cancel me",
			expectError:    true,
			errorText:      "failed to run prompt through MPT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{runner: tc.runner}

			request := mcp.CallToolRequest{}
			request.Params.Arguments = tc.arguments

			// create a cancelable context to test cancellation handling
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			result, err := srv.handleGenerateTool(ctx, request)

			// check runner was called with the expected prompt if applicable
			if tc.expectedPrompt != "" {
				calls := tc.runner.RunCalls()
				if tc.expectError && err != nil {
					// even with errors, the runner should have been called
					require.GreaterOrEqual(t, len(calls), 1, "Runner should have been called")
					if len(calls) > 0 {
						assert.Equal(t, tc.expectedPrompt, calls[len(calls)-1].Prompt,
							"Runner should have been called with the expected prompt")
					}
				} else if !tc.expectError {
					require.Len(t, calls, 1, "Runner should have been called exactly once")
					assert.Equal(t, tc.expectedPrompt, calls[0].Prompt,
						"Runner should have been called with the expected prompt")
				}
			}

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorText)
			} else {
				require.NoError(t, err)
				if tc.checkResult != nil {
					tc.checkResult(t, result)
				}
			}
		})
	}
}
