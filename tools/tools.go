package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// The MCP defaults for DestructiveHint and OpenWorldHint are both true, which
// reads identically to "the author never considered it". Every tool therefore
// states its hints explicitly. All of these tools reach an external SolidServer
// appliance, so OpenWorldHint is true throughout.

const (
	defaultListLimit = 50
	maxListLimit     = 1000
)

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

// RegisterAll registers all SolidServer tools with default guardrails.
func RegisterAll(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	RegisterAllWithGuardrails(s, client, logger, nil)
}

// RegisterAllWithGuardrails registers all SolidServer tools with the MCP server and applies guardrails.
func RegisterAllWithGuardrails(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
	if logger == nil {
		logger = slog.Default()
	}
	RegisterIPAMTools(s, client, logger, g)
	RegisterSubnetTools(s, client, logger, g)
	RegisterDNSTools(s, client, logger, g)
	RegisterVlanTools(s, client, logger, g)
	RegisterDhcpTools(s, client, logger, g)
	RegisterDoctorTool(s, client, logger)
}

// jsonResult builds a JSON-formatted text content result from structured output data.
func jsonResult(data any) *mcp.CallToolResult {
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
	}
}

// errorResult builds an error result with IsError: true.
func errorResult(format string, args ...any) *mcp.CallToolResult {
	text := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
		IsError: true,
	}
}

// validationErrorResult builds a tool error result for client-side validation failures.
//
//nolint:unparam // Signature must match MCP tool handler return pattern (*mcp.CallToolResult, Out, error)
func validationErrorResult[T any](err error, empty T) (*mcp.CallToolResult, T, error) {
	return errorResult("invalid parameter: %v", err), empty, nil
}

type sdsErrorPayload struct {
	Errno  string `json:"errno"`
	Errmsg string `json:"errmsg"`
}

func httpStatusHint(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return " (check API token credentials and permissions)"
	case http.StatusNotFound:
		return " (verify target space, zone, or resource exists)"
	case http.StatusConflict:
		return " (resource already exists or conflicts with existing state)"
	case http.StatusTooManyRequests:
		return " (rate limit exceeded; back off and retry)"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return " (SOLIDserver appliance error)"
	default:
		return ""
	}
}

// formatAPIError parses errors from the SolidServer API client, extracting HTTP status,
// appliance errno, and errmsg where available, and includes actionable remediation hints.
func formatAPIError(err error, httpResp *http.Response) string {
	if err == nil {
		return ""
	}

	var status int
	if httpResp != nil {
		status = httpResp.StatusCode
	}

	var errno, errmsg string
	if openAPIErr, ok := errors.AsType[*sdsclient.GenericOpenAPIError](err); ok {
		body := openAPIErr.Body()
		if len(body) > 0 {
			var payload sdsErrorPayload
			if jsonErr := json.Unmarshal(body, &payload); jsonErr == nil {
				errno = payload.Errno
				errmsg = payload.Errmsg
			}
		}
	}

	hint := httpStatusHint(status)
	if errno != "" || errmsg != "" {
		errDetails := errmsg
		if errDetails == "" {
			errDetails = err.Error()
		}
		if errno != "" {
			if status != 0 {
				return fmt.Sprintf("SolidServer API error (status %d, errno %s): %s%s", status, errno, errDetails, hint)
			}
			return fmt.Sprintf("SolidServer API error (errno %s): %s%s", errno, errDetails, hint)
		}
		if status != 0 {
			return fmt.Sprintf("SolidServer API error (status %d): %s%s", status, errDetails, hint)
		}
		return fmt.Sprintf("SolidServer API error: %s%s", errDetails, hint)
	}

	if status != 0 {
		return fmt.Sprintf("SolidServer API error (status %d): %v%s", status, err, hint)
	}

	return fmt.Sprintf("SolidServer API error: %v", err)
}

// ListOptions defines common parameters for list tools.
type ListOptions struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

// ListOutput is the standardized typed output wrapper for all list tools.
type ListOutput[T any] struct {
	Data   []T   `json:"data" jsonschema:"Array of resource records matching the query."`
	Count  int   `json:"count" jsonschema:"Number of records returned in this page."`
	Limit  int32 `json:"limit" jsonschema:"Requested page size limit."`
	Offset int32 `json:"offset" jsonschema:"Requested pagination offset."`
}

// closeBody safely closes an HTTP response body if present.
func closeBody(httpResp *http.Response) {
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
}

// CommonListRequester is a function type that executes a list request against the SDK.
type CommonListRequester[T any] func(ctx context.Context, where string, limit, offset int32) ([]T, *http.Response, error)

// commonListHandler provides a generic way to handle list requests with typed outputs.
//
//nolint:unparam // Signature must match MCP tool handler return pattern (*mcp.CallToolResult, Out, error)
func commonListHandler[T any](
	ctx context.Context,
	opts ListOptions,
	logger *slog.Logger,
	toolName string,
	execute CommonListRequester[T],
) (*mcp.CallToolResult, ListOutput[T], error) {
	emptyOut := ListOutput[T]{
		Data:   make([]T, 0),
		Count:  0,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}

	if err := ValidateWhereClause(opts.Where); err != nil {
		logger.Warn("invalid where clause", "tool", toolName, "error", err)
		return errorResult("invalid where clause: %v", err), emptyOut, nil
	}

	if opts.Offset < 0 {
		return errorResult("offset must be non-negative, got %d", opts.Offset), emptyOut, nil
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	} else if limit > maxListLimit {
		limit = maxListLimit
	}

	logger.Debug("executing list tool", "tool", toolName, "where", opts.Where, "limit", limit, "offset", opts.Offset)
	items, httpResp, err := execute(ctx, opts.Where, limit, opts.Offset)
	closeBody(httpResp)
	if err != nil {
		logger.Error("API error", "tool", toolName, "error", err)
		return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
	}

	if items == nil {
		items = make([]T, 0)
	}

	logger.Debug("tool success", "tool", toolName, "count", len(items))
	out := ListOutput[T]{
		Data:   items,
		Count:  len(items),
		Limit:  limit,
		Offset: opts.Offset,
	}
	return jsonResult(out), out, nil
}

// CombineWhereClause combines a fixed appliance filter (e.g. space_name='...') with an optional user WHERE clause.
func CombineWhereClause(fixed, user string) string {
	if fixed == "" {
		return user
	}
	if user == "" {
		return fixed
	}
	return fmt.Sprintf("(%s) AND (%s)", fixed, user)
}
