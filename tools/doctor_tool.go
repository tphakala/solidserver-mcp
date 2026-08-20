package tools

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// DoctorInput defines optional arguments for solidserver_doctor tool.
type DoctorInput struct{}

// DoctorDiagnosticData contains the diagnostic check results.
type DoctorDiagnosticData struct {
	Healthy bool                       `json:"healthy" jsonschema:"Whether all diagnostic preflight checks passed."`
	Host    string                     `json:"host" jsonschema:"Target SOLIDserver hostname."`
	Checks  []services.DiagnosticCheck `json:"checks" jsonschema:"List of individual diagnostic check results."`
}

// DoctorOut is the structured output for the doctor tool matching standard Data wrapper convention.
type DoctorOut struct {
	Data []DoctorDiagnosticData `json:"data" jsonschema:"Diagnostic preflight check results."`
}

// RegisterDoctorTool registers the solidserver_doctor diagnostic tool with the MCP server.
func RegisterDoctorTool(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_doctor",
		Title:       "Run SOLIDserver connectivity and credentials doctor",
		Annotations: readOnlyTool("Run SOLIDserver connectivity and credentials doctor"),
		Description: "Performs preflight diagnostic checks against the remote SOLIDserver appliance, " +
			"testing DNS resolution, TCP network reachability, TLS handshake validation, and REST API " +
			"token authentication. Use this tool to troubleshoot connection failures, certificate " +
			"errors, or invalid credentials before running other operations like solidserver_space_list " +
			"or solidserver_ip_list. Returns structured diagnostics and actionable remediation hints " +
			"without exposing sensitive credentials.",
	}, doctorHandler(client, logger))
}

func doctorHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DoctorInput) (*mcp.CallToolResult, DoctorOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DoctorInput) (*mcp.CallToolResult, DoctorOut, error) {
		logger.Info("running solidserver_doctor preflight checks")
		var host string
		var sslVerify bool
		if client != nil {
			host = client.Host()
			sslVerify = client.SSLVerify()
		}

		res := services.RunDoctor(ctx, client, host, sslVerify)
		out := DoctorOut{
			Data: []DoctorDiagnosticData{
				{
					Healthy: res.Healthy,
					Host:    res.Host,
					Checks:  res.Checks,
				},
			},
		}

		if !res.Healthy {
			result := jsonResult(out)
			result.IsError = true
			return result, out, nil
		}
		return jsonResult(out), out, nil
	}
}
