package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRunner mocks the runner.Runner interface for testing
type MockRunner struct{}

func (mr *MockRunner) Run(ctx context.Context, prompt string) (string, error) {
	if prompt == "error" {
		return "", errors.New("test error")
	}
	return "Generated response for: " + prompt, nil
}

func TestServer_handleGenerateTool(t *testing.T) {
	// create a server with our mock runner
	srv := &Server{
		runner: &MockRunner{},
	}

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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'prompt' parameter")

	// test with wrong prompt type
	request = mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"prompt": 123, // not a string
	}

	_, err = srv.handleGenerateTool(context.Background(), request)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "'prompt' parameter must be a string")
}
