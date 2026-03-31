package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// RegisterAll registers all SolidServer tools with the MCP server.
func RegisterAll(s *mcp.Server, client *services.APIClientWrapper) {
	RegisterIPAMTools(s, client)
	RegisterSubnetTools(s, client)
	RegisterDNSTools(s, client)
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
	}, "ignored"
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
	}, "error"
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
	execute CommonListRequester,
) (*mcp.CallToolResult, any, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	resp, err := execute(ctx, opts.Where, limit, opts.Offset)
	if err != nil {
		res, anyVal := errorResult("SolidServer API error: %v", err)
		return res, anyVal, nil
	}

	res, anyVal := jsonResult(resp)
	return res, anyVal, nil
}
