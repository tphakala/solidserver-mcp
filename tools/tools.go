package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

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
