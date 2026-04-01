package tools

import (
	"context"
	"log/slog"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// DHCP Input Structs
type DhcpServerListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpScopeListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpRangeListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpLeaseListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpStaticAddInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server."`
	Name   string `json:"name" jsonschema:"The name of the static reservation."`
	IP     string `json:"ip" jsonschema:"The IP address to reserve."`
	MAC    string `json:"mac" jsonschema:"The MAC address for the reservation (e.g. 01:00:11:22:33:44:55). The first octet is the type (01 for Ethernet)."`
}

type DhcpStaticDeleteInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server."`
	IP     string `json:"ip" jsonschema:"The IP address of the reservation to delete."`
}

// RegisterDhcpTools registers DHCP management tools.
func RegisterDhcpTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_server_list",
		Description: "Lists DHCP servers.",
	}, dhcpServerListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_scope_list",
		Description: "Lists DHCP scopes.",
	}, dhcpScopeListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_range_list",
		Description: "Lists DHCP ranges.",
	}, dhcpRangeListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_lease_list",
		Description: "Lists DHCP leases.",
	}, dhcpLeaseListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_add",
		Description: "Adds a static DHCP reservation.",
	}, dhcpStaticAddHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_delete",
		Description: "Deletes a static DHCP reservation.",
	}, dhcpStaticDeleteHandler(client, logger))
}

func dhcpServerListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpServerListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpServerListInput) (*mcp.CallToolResult, any, error) {
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_server_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpServerList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

func dhcpScopeListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpScopeListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpScopeListInput) (*mcp.CallToolResult, any, error) {
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_scope_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpScopeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

func dhcpRangeListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpRangeListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpRangeListInput) (*mcp.CallToolResult, any, error) {
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_range_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpRangeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

func dhcpLeaseListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpLeaseListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpLeaseListInput) (*mcp.CallToolResult, any, error) {
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_lease_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpLeaseList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

func dhcpStaticAddHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpStaticAddInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticAddInput) (*mcp.CallToolResult, any, error) {
		logger.Info("adding static DHCP reservation", "name", in.Name, "ip", in.IP, "mac", in.MAC, "server", in.Server)
		input := sdsclient.DhcpStaticAddInput{
			ServerName: &in.Server,
			StaticName: &in.Name,
			StaticAddr: &in.IP,
			StaticMacAddr: &in.MAC,
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpStaticAdd(authCtx).DhcpStaticAddInput(input)
		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func dhcpStaticDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpStaticDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticDeleteInput) (*mcp.CallToolResult, any, error) {
		logger.Info("deleting static DHCP reservation", "ip", in.IP, "server", in.Server)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpStaticDelete(authCtx).
			ServerName(in.Server).
			StaticAddr(in.IP)

		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}
