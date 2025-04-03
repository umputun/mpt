package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/umputun/mpt/pkg/provider"
	"github.com/umputun/mpt/pkg/runner"
)

// Server represents an MCP server that uses MPT's runner to fulfill MCP requests
type Server struct {
	mcpServer *server.MCPServer
	runner    *runner.Runner
}

// NewServer creates a new MCP server using MPT's runner
func NewServer(r *runner.Runner, opts ServerOptions) *Server {
	// Create MCP server
	mcpServer := server.NewMCPServer(
		opts.Name,
		opts.Version,
		server.WithResourceCapabilities(true, true),
		server.WithToolCapabilities(true),
		server.WithSamplingCapabilities(true),
		server.WithLogging(),
	)

	srv := &Server{
		mcpServer: mcpServer,
		runner:    r,
	}

	// Register sampling handler - this is the core functionality that will 
	// use MPT's runner to handle MCP requests
	mcpServer.RegisterSamplingHandler(srv.handleSampling)

	return srv
}

// handleSampling is the core function that processes MCP model sampling requests
// by routing them through MPT's runner which coordinates multiple providers
func (s *Server) handleSampling(ctx context.Context, params mcp.SampleModelParams) (*mcp.SampleModelResult, error) {
	// Construct the prompt from the MCP request
	prompt := params.Prompt.Content

	// Process any provided resources
	for _, resource := range params.Resources {
		// Add resources as context to the prompt
		// For text resources, add them to the prompt with appropriate formatting
		if resource.MimeType == "text/plain" || resource.MimeType == "text/markdown" {
			resourceType := resource.Metadata["type"]
			if resourceType == "file" && resource.Metadata["path"] != nil {
				// Format as a file inclusion
				path, _ := resource.Metadata["path"].(string)
				prompt += fmt.Sprintf("\n\n// file: %s\n%s", path, resource.Data)
			} else {
				// Just add as plain text
				prompt += "\n\n" + resource.Data
			}
		}
	}

	// Run the prompt through MPT's runner
	result, err := s.runner.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to run prompt through MPT: %w", err)
	}

	// Return the result in MCP format
	return &mcp.SampleModelResult{
		Content: result,
	}, nil
}

// Start starts the MCP server using stdio transport (standard input/output)
func (s *Server) Start() error {
	return server.ServeStdio(s.mcpServer)
}

// ServerOptions contains configuration options for the MCP server
type ServerOptions struct {
	Name    string
	Version string
}