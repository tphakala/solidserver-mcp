package tools

import (
	"context"
	"fmt"

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
func RegisterDhcpTools(s *mcp.Server, client *services.APIClientWrapper) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_server_list",
		Description: "Lists DHCP servers.",
	}, dhcpServerListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_scope_list",
		Description: "Lists DHCP scopes.",
	}, dhcpScopeListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_range_list",
		Description: "Lists DHCP ranges.",
	}, dhcpRangeListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_lease_list",
		Description: "Lists DHCP leases.",
	}, dhcpLeaseListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_add",
		Description: "Adds a static DHCP reservation.",
	}, dhcpStaticAddHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_delete",
		Description: "Deletes a static DHCP reservation.",
	}, dhcpStaticDeleteHandler(client))
}

func dhcpServerListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpServerListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpServerListInput) (*mcp.CallToolResult, any, error) {
		return commonListHandler(ctx, ListOptions(in),
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpApi.DhcpServerList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

func dhcpScopeListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpScopeListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpScopeListInput) (*mcp.CallToolResult, any, error) {
		return commonListHandler(ctx, ListOptions(in),
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpApi.DhcpScopeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

func dhcpRangeListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpRangeListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpRangeListInput) (*mcp.CallToolResult, any, error) {
		return commonListHandler(ctx, ListOptions(in),
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpApi.DhcpRangeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

func dhcpLeaseListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpLeaseListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpLeaseListInput) (*mcp.CallToolResult, any, error) {
		return commonListHandler(ctx, ListOptions(in),
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpApi.DhcpLeaseList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

func dhcpStaticAddHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpStaticAddInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticAddInput) (*mcp.CallToolResult, any, error) {
		input := sdsclient.DhcpStaticAddInput{
			ServerName:    &in.Server,
			StaticName:    &in.Name,
			StaticAddr:    &in.IP,
			StaticMacAddr: &in.MAC,
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpApi.DhcpStaticAdd(authCtx).DhcpStaticAddInput(input)
		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func dhcpStaticDeleteHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DhcpStaticDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticDeleteInput) (*mcp.CallToolResult, any, error) {
		authCtx := client.AuthContext(ctx)
		req := client.DhcpApi.DhcpStaticDelete(authCtx).
			ServerName(in.Server).
			StaticAddr(in.IP)

		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}
