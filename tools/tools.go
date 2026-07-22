package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// The MCP defaults for DestructiveHint and OpenWorldHint are both true, which
// reads identically to "the author never considered it". Every tool therefore
// states its hints explicitly. All of these tools reach an external SolidServer
// appliance, so OpenWorldHint is true throughout.

// readOnlyTool annotates a tool that only reads state.
func readOnlyTool(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:         title,
		ReadOnlyHint:  true,
		OpenWorldHint: new(true),
	}
}

// additiveTool annotates a tool that creates state without destroying any.
// Not idempotent: calling it again either allocates another object or fails on
// a duplicate.
func additiveTool(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		DestructiveHint: new(false),
		IdempotentHint:  false,
		OpenWorldHint:   new(true),
	}
}

// destructiveTool annotates a tool that removes state. Idempotent in the sense
// the spec means: deleting an already-deleted object has no further effect.
func destructiveTool(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		DestructiveHint: new(true),
		IdempotentHint:  true,
		OpenWorldHint:   new(true),
	}
}

// RegisterAll registers all SolidServer tools with the MCP server.
func RegisterAll(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	RegisterIPAMTools(s, client, logger)
	RegisterSubnetTools(s, client, logger)
	RegisterDNSTools(s, client, logger)
	RegisterVlanTools(s, client, logger)
	RegisterDhcpTools(s, client, logger)
}

// textResult builds a simple text content result.
//
//nolint:unparam // anyVal is always nil; kept for signature consistency with jsonResult and errorResult.
func textResult(format string, args ...any) (res *mcp.CallToolResult, anyVal any) {
	text := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}, nil
}

// jsonResult builds a JSON-formatted text content result.
func jsonResult(data any) (res *mcp.CallToolResult, anyVal any) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errorResult("failed to marshal JSON: %v", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(b),
			},
		},
	}, data
}

// errorResult builds an error result with IsError: true.
func errorResult(format string, args ...any) (res *mcp.CallToolResult, anyVal any) {
	text := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
		IsError: true,
	}, nil
}

// ListOptions defines common parameters for list tools.
type ListOptions struct {
	Where  string `json:"where,omitempty"`
	Limit  int32  `json:"limit,omitempty"`
	Offset int32  `json:"offset,omitempty"`
}

// CommonListRequester is a function type that executes a list request.
type CommonListRequester func(ctx context.Context, where string, limit, offset int32) (any, error)

// commonListHandler provides a generic way to handle list requests.
func commonListHandler(
	ctx context.Context,
	opts ListOptions,
	logger *slog.Logger,
	toolName string,
	execute CommonListRequester,
) (*mcp.CallToolResult, any, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	logger.Debug("executing list tool", "tool", toolName, "where", opts.Where, "limit", limit, "offset", opts.Offset)
	resp, err := execute(ctx, opts.Where, limit, opts.Offset)
	if err != nil {
		logger.Error("API error", "tool", toolName, "error", err)
		res, anyVal := errorResult("SolidServer API error: %v", err)
		return res, anyVal, nil
	}

	logger.Debug("tool success", "tool", toolName)
	res, anyVal := jsonResult(resp)
	return res, anyVal, nil
}
