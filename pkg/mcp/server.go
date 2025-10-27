package mcp

import (
	"context"
	"fmt"

	"github.com/go-pkgz/lgr"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

//go:generate moq -out mocks/runner.go -pkg mocks -skip-ensure -fmt goimports . Runner

// Server represents an MCP server that uses MPT's runner to fulfill MCP requests
type Server struct {
	mcpServer *server.MCPServer
	runner    Runner
}

// Runner defines the interface for running prompts through providers
type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// NewServer creates a new MCP server using MPT's runner
func NewServer(r Runner, opts ServerOptions) *Server {
	// create MCP server
	mcpServer := server.NewMCPServer(
		opts.Name,
		opts.Version,
		server.WithResourceCapabilities(true, true),
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	srv := &Server{
		mcpServer: mcpServer,
		runner:    r,
	}

	// add a tool for generating text through MPT's providers
	generateTool := mcp.NewTool("mpt_generate",
		mcp.WithDescription("Generate text using multiple LLM providers"),
		mcp.WithString("prompt",
			mcp.Required(),
			mcp.Description("The prompt to send to the LLM providers"),
		),
	)

	// register the tool handler
	mcpServer.AddTool(generateTool, srv.handleGenerateTool)

	return srv
}

// handleGenerateTool processes text generation requests by routing them through MPT's runner
func (s *Server) handleGenerateTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lgr.Printf("[DEBUG] MCP tool 'mpt_generate' called")

	// extract the prompt from the request using library's type-safe method
	prompt, err := request.RequireString("prompt")
	if err != nil {
		lgr.Printf("[WARN] MCP tool 'mpt_generate' invalid prompt parameter: %v", err)
		return nil, fmt.Errorf("invalid prompt parameter: %w", err)
	}

	// run the prompt through MPT's runner
	lgr.Printf("[DEBUG] MCP tool 'mpt_generate' running prompt through MPT")
	result, err := s.runner.Run(ctx, prompt)
	if err != nil {
		lgr.Printf("[WARN] MCP tool 'mpt_generate' failed: %v", err)
		return nil, fmt.Errorf("failed to run prompt through MPT: %w", err)
	}

	lgr.Printf("[DEBUG] MCP tool 'mpt_generate' completed successfully")
	// return the result as text
	return mcp.NewToolResultText(result), nil
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
