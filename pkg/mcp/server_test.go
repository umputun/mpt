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
			name: "creates server with default values",
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
			
			// Verify server was created properly
			assert.NotNil(t, server)
			assert.NotNil(t, server.mcpServer)
			assert.Equal(t, runner, server.runner)
		})
	}
}

func TestServer_handleGenerateTool(t *testing.T) {
	successRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Generated response for: " + prompt, nil
		},
	}
	
	errorRunner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("runner error")
		},
	}

	tests := []struct {
		name        string
		runner      *mocks.RunnerMock
		arguments   map[string]interface{}
		expectError bool
		errorText   string
		checkResult func(t *testing.T, result *mcp.CallToolResult)
	}{
		{
			name:        "valid prompt",
			runner:      successRunner,
			arguments:   map[string]interface{}{"prompt": "Test prompt"},
			expectError: false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				require.NotEmpty(t, result.Content)
				textContent, ok := result.Content[0].(mcp.TextContent)
				require.True(t, ok, "Expected TextContent")
				assert.Contains(t, textContent.Text, "Generated response for: Test prompt")
			},
		},
		{
			name:        "missing prompt parameter",
			runner:      successRunner,
			arguments:   map[string]interface{}{},
			expectError: true,
			errorText:   "missing required 'prompt' parameter",
		},
		{
			name:        "wrong prompt type",
			runner:      successRunner,
			arguments:   map[string]interface{}{"prompt": 123}, // not a string
			expectError: true,
			errorText:   "'prompt' parameter must be a string",
		},
		{
			name:        "runner error",
			runner:      errorRunner,
			arguments:   map[string]interface{}{"prompt": "Test prompt"},
			expectError: true,
			errorText:   "failed to run prompt through MPT",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{runner: tc.runner}
			
			request := mcp.CallToolRequest{}
			request.Params.Arguments = tc.arguments
			
			result, err := srv.handleGenerateTool(context.Background(), request)
			
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
