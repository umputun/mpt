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
	runner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "test response", nil
		},
	}
	
	server := NewServer(runner, ServerOptions{
		Name:    "Test Server",
		Version: "1.0.0",
	})
	
	assert.NotNil(t, server)
	assert.NotNil(t, server.mcpServer)
	assert.Equal(t, runner, server.runner)
}

func TestServer_handleGenerateTool(t *testing.T) {
	runner := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Generated response for: " + prompt, nil
		},
	}
	srv := &Server{runner: runner}

	// test with valid prompt
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"prompt": "Test prompt",
	}

	result, err := srv.handleGenerateTool(context.Background(), request)
	require.NoError(t, err)

	// check that there's at least one content item
	require.NotEmpty(t, result.Content)

	// since we're using NewToolResultText, the first content should be TextContent
	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "Expected TextContent")

	// check the content of the text
	assert.Contains(t, textContent.Text, "Generated response for: Test prompt")

	// test with missing prompt parameter
	request = mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	_, err = srv.handleGenerateTool(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'prompt' parameter")

	// test with wrong prompt type
	request = mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"prompt": 123, // not a string
	}

	_, err = srv.handleGenerateTool(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'prompt' parameter must be a string")
	
	// test with runner error
	runnerWithError := &mocks.RunnerMock{
		RunFunc: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("runner error")
		},
	}
	srvWithError := &Server{runner: runnerWithError}
	
	request = mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"prompt": "Test prompt",
	}
	
	_, err = srvWithError.handleGenerateTool(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run prompt through MPT")
}
