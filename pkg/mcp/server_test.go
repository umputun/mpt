package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
	"github.com/umputun/mpt/pkg/runner/mocks"
)

func TestServer_handleSampling(t *testing.T) {
	// Create a mock provider for testing
	mockProvider := &mocks.ProviderMock{
		GenerateFunc: func(ctx context.Context, prompt string) (string, error) {
			return "Generated response for: " + prompt, nil
		},
		NameFunc: func() string {
			return "MockProvider"
		},
		EnabledFunc: func() bool {
			return true
		},
	}

	// Create runner with the mock provider
	r := runner.New(mockProvider)

	// Create MCP server
	server := NewServer(r, ServerOptions{
		Name:    "Test Server",
		Version: "1.0.0",
	})

	// Basic test for handleSampling
	result, err := server.handleSampling(context.Background(), mcp.SampleModelParams{
		Prompt: mcp.UserMessageParams{
			Content: "Test prompt",
		},
	})

	require.NoError(t, err)
	assert.Contains(t, result.Content, "Generated response for: Test prompt")

	// Test with resource
	result, err = server.handleSampling(context.Background(), mcp.SampleModelParams{
		Prompt: mcp.UserMessageParams{
			Content: "Test prompt with resource",
		},
		Resources: []mcp.Resource{
			{
				Data:     "This is a test file content",
				MimeType: "text/plain",
				Metadata: map[string]interface{}{
					"type": "file",
					"path": "test.txt",
				},
			},
		},
	})

	require.NoError(t, err)
	assert.Contains(t, result.Content, "Generated response for:")
	assert.Contains(t, mockProvider.GenerateCalls()[1].Prompt, "test.txt")
}