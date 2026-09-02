package tools

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

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

type DhcpScopeCreateInput struct {
	Server  string `json:"server" jsonschema:"The name of the DHCP server to create the scope on."`
	Address string `json:"address" jsonschema:"The IPv4 network address of the scope (e.g. '10.0.0.0')."`
	Prefix  string `json:"prefix" jsonschema:"The IPv4 prefix length of the scope (e.g. '24')."`
	Name    string `json:"name,omitempty" jsonschema:"An optional name for the scope."`
}

type DhcpRangeCreateInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server."`
	Start  string `json:"start" jsonschema:"The first IP address of the range."`
	End    string `json:"end" jsonschema:"The last IP address of the range."`
	Scope  string `json:"scope,omitempty" jsonschema:"The name of the scope the range belongs to (optional)."`
	Name   string `json:"name,omitempty" jsonschema:"An optional name for the range."`
}

type DhcpScopeDeleteInput struct {
	Server  string `json:"server" jsonschema:"The name of the DHCP server the scope is on."`
	Address string `json:"address" jsonschema:"The IPv4 network address of the scope to delete (e.g. '10.0.0.0')."`
}

type DhcpRangeDeleteInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server the range is on."`
	Start  string `json:"start" jsonschema:"The first IP address of the range to delete."`
	End    string `json:"end" jsonschema:"The last IP address of the range to delete."`
}

type DhcpSharedNetworkListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpSharedNetworkCreateInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server to create the shared network on."`
	Name   string `json:"name" jsonschema:"The name of the shared network to create."`
}

type DhcpSharedNetworkDeleteInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server the shared network is on."`
	Name   string `json:"name" jsonschema:"The name of the shared network to delete."`
}

type DhcpGroupListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DhcpGroupCreateInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server to create the group on."`
	Name   string `json:"name" jsonschema:"The name of the DHCP group to create."`
}

type DhcpGroupDeleteInput struct {
	Server string `json:"server" jsonschema:"The name of the DHCP server the group is on."`
	Name   string `json:"name" jsonschema:"The name of the DHCP group to delete."`
}

type DhcpStaticListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

// DHCP Output Structs
type DhcpServerListOut = ListOutput[sdsclient.DataInnerDhcpServerData]
type DhcpScopeListOut = ListOutput[sdsclient.DataInnerDhcpScopeData]
type DhcpRangeListOut = ListOutput[sdsclient.DataInnerDhcpRangeData]
type DhcpLeaseListOut = ListOutput[sdsclient.DataInnerDhcpLeaseData]

type DhcpStaticAddOut struct {
	Data []sdsclient.DataInnerDhcpStaticAddSuccess `json:"data" jsonschema:"Created static DHCP reservation records."`
}

type DhcpStaticDeleteOut struct {
	Data []sdsclient.DataInnerDhcpStaticDeleteSuccess `json:"data" jsonschema:"Deleted static DHCP reservation response records."`
}

type DhcpScopeCreateOut struct {
	Data []sdsclient.DataInnerDhcpScopeAddSuccess `json:"data" jsonschema:"Created DHCP scope response records."`
}

type DhcpRangeCreateOut struct {
	Data []sdsclient.DataInnerDhcpRangeAddSuccess `json:"data" jsonschema:"Created DHCP range response records."`
}

type DhcpScopeDeleteOut struct {
	Data []sdsclient.DataInnerDhcpScopeDeleteSuccess `json:"data" jsonschema:"Deleted DHCP scope response records."`
}

type DhcpRangeDeleteOut struct {
	Data []sdsclient.DataInnerDhcpRangeDeleteSuccess `json:"data" jsonschema:"Deleted DHCP range response records."`
}

type DhcpSharedNetworkListOut = ListOutput[sdsclient.DataInnerDhcpSharednetworkData]
type DhcpGroupListOut = ListOutput[sdsclient.DataInnerDhcpGroupData]
type DhcpStaticListOut = ListOutput[sdsclient.DataInnerDhcpStaticData]

type DhcpSharedNetworkCreateOut struct {
	Data []sdsclient.DataInnerDhcpSharednetworkAddSuccess `json:"data" jsonschema:"Created DHCP shared network response records."`
}

type DhcpSharedNetworkDeleteOut struct {
	Data []sdsclient.DataInnerDhcpSharednetworkDeleteSuccess `json:"data" jsonschema:"Deleted DHCP shared network response records."`
}

type DhcpGroupCreateOut struct {
	Data []sdsclient.DataInnerDhcpGroupAddSuccess `json:"data" jsonschema:"Created DHCP group response records."`
}

type DhcpGroupDeleteOut struct {
	Data []sdsclient.DataInnerDhcpGroupDeleteSuccess `json:"data" jsonschema:"Deleted DHCP group response records."`
}

// RegisterDhcpTools registers DHCP management tools.
func RegisterDhcpTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_server_list",
		Title:       "List DHCP servers",
		Annotations: readOnlyTool("List DHCP servers"),
		Description: "Enumerates the DHCP servers the appliance manages. Start here when you do not " +
			"already know the server name, since solidserver_dhcp_static_add and " +
			"solidserver_dhcp_static_delete both require one. Use solidserver_dhcp_scope_list " +
			"instead to see the scopes configured on a server. Returns the server records as JSON.",
	}, dhcpServerListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_scope_list",
		Title:       "List DHCP scopes",
		Annotations: readOnlyTool("List DHCP scopes"),
		Description: "Enumerates DHCP scopes, the per-subnet configuration blocks that hold options " +
			"and ranges. Use this to see which subnets a server actually serves. Use " +
			"solidserver_dhcp_range_list instead for the address ranges handed out within a scope, " +
			"and solidserver_dhcp_lease_list for the leases currently held. Returns the scope " +
			"records as JSON.",
	}, dhcpScopeListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_range_list",
		Title:       "List DHCP ranges",
		Annotations: readOnlyTool("List DHCP ranges"),
		Description: "Enumerates the address ranges DHCP allocates from. Use this to see the dynamic " +
			"pool boundaries, for instance to pick a static reservation address that sits outside " +
			"the pool before calling solidserver_dhcp_static_add. Describes configured ranges, not " +
			"live assignments: use solidserver_dhcp_lease_list for what is currently leased. Returns " +
			"the range records as JSON.",
	}, dhcpRangeListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_lease_list",
		Title:       "List DHCP leases",
		Annotations: readOnlyTool("List DHCP leases"),
		Description: "Enumerates leases DHCP has currently handed out, tying an address to the " +
			"client MAC holding it. Use this to find which device is on an address right now, or to " +
			"recover a MAC for a static reservation. Leases expire and are reassigned, so this is a " +
			"point-in-time view rather than stable inventory; use solidserver_ip_list for recorded " +
			"IPAM allocations instead. Returns the lease records as JSON.",
	}, dhcpLeaseListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_add",
		Title:       "Add a static DHCP reservation",
		Annotations: additiveTool("Add a static DHCP reservation"),
		Description: "Pins an address to a specific client MAC on a DHCP server, so that client " +
			"always receives the same address. The server must already exist; list them with " +
			"solidserver_dhcp_server_list. Prefer an address outside the dynamic range, which " +
			"solidserver_dhcp_range_list will show you, since reserving one inside the pool can " +
			"collide with a lease already issued to another client. Changes appliance state and is " +
			"undone only by solidserver_dhcp_static_delete. Returns the created reservation as JSON.",
	}, dhcpStaticAddHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_delete",
		Title:       "Delete a static DHCP reservation",
		Annotations: destructiveTool("Delete a static DHCP reservation"),
		Description: "Permanently removes a static DHCP reservation, identified by the DHCP server " +
			"and the reserved IP address. This is destructive and cannot be undone from this server; the " +
			"reservation can only be recreated with solidserver_dhcp_static_add. The client keeps " +
			"its current lease until that lease expires and then falls back to a dynamic address, so " +
			"a device expected to be reachable at a fixed address will move. Confirm the address is " +
			"the reservation you mean with solidserver_dhcp_lease_list first; this delete matches on " +
			"the address alone and does not verify which MAC the reservation belonged to. Returns a " +
			"confirmation message.",
	}, dhcpStaticDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_scope_create",
		Title:       "Create a DHCP scope",
		Annotations: additiveTool("Create a DHCP scope"),
		Description: "Creates a DHCP scope, the per-subnet configuration block that holds options and " +
			"ranges, on a DHCP server. The server must already exist; list them with " +
			"solidserver_dhcp_server_list. A scope on its own hands out no addresses until you add a " +
			"range to it with solidserver_dhcp_range_create. Check for an existing scope on the same " +
			"subnet with solidserver_dhcp_scope_list first, since an overlap is rejected. Changes " +
			"appliance state. Returns the created scope as JSON.",
	}, dhcpScopeCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_range_create",
		Title:       "Create a DHCP range",
		Annotations: additiveTool("Create a DHCP range"),
		Description: "Creates a dynamic address range that DHCP allocates from, bounded by a start and " +
			"end address. The enclosing scope and server must already exist; find them with " +
			"solidserver_dhcp_scope_list and solidserver_dhcp_server_list. Keep static reservations " +
			"made with solidserver_dhcp_static_add outside this range, since an address inside the " +
			"pool can be leased to another client. Check existing ranges with " +
			"solidserver_dhcp_range_list first to avoid an overlap. Changes appliance state. Returns " +
			"the created range as JSON.",
	}, dhcpRangeCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_scope_delete",
		Title:       "Delete a DHCP scope",
		Annotations: destructiveTool("Delete a DHCP scope"),
		Description: "Permanently removes a DHCP scope from a server, identified by the server name and " +
			"the scope's IPv4 network address. This is destructive and cannot be undone from this " +
			"server; deleting a scope also removes the ranges and options configured inside it, so the " +
			"server stops serving that subnet. Audit what the scope contains with " +
			"solidserver_dhcp_range_list first. The scope's real size is resolved from the appliance so " +
			"the deletion is checked against protected-subnet rules for its whole range. Returns a " +
			"confirmation message.",
	}, dhcpScopeDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_range_delete",
		Title:       "Delete a DHCP range",
		Annotations: destructiveTool("Delete a DHCP range"),
		Description: "Permanently removes a dynamic address range from a DHCP server, identified by the " +
			"server name and the range's start and end addresses. This is destructive and cannot be " +
			"undone from this server; once removed, DHCP no longer allocates from that pool and clients " +
			"holding leases in it fall back to another range or stop being served when their lease " +
			"expires. Confirm the boundaries with solidserver_dhcp_range_list first. Returns a " +
			"confirmation message.",
	}, dhcpRangeDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_shared_network_list",
		Title:       "List DHCP shared networks",
		Annotations: readOnlyTool("List DHCP shared networks"),
		Description: "Enumerates DHCP shared networks, the containers that group several scopes so one " +
			"physical segment can serve more than one subnet. Use this to see how scopes are grouped " +
			"before creating one with solidserver_dhcp_scope_create, or to find the shared network name " +
			"that solidserver_dhcp_shared_network_delete needs. Use solidserver_dhcp_scope_list instead " +
			"for the individual subnets a shared network holds. Returns the shared network records as JSON.",
	}, dhcpSharedNetworkListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_shared_network_create",
		Title:       "Create a DHCP shared network",
		Annotations: additiveTool("Create a DHCP shared network"),
		Description: "Creates a DHCP shared network on a server, a named container used to group several " +
			"scopes that serve one physical segment. The server must already exist; list them with " +
			"solidserver_dhcp_server_list. A shared network holds no addresses of its own; its member " +
			"scopes are created separately with solidserver_dhcp_scope_create, and " +
			"solidserver_dhcp_shared_network_list shows the shared networks already defined. Check for an " +
			"existing one first, since a duplicate name is rejected. Changes appliance state. Returns the " +
			"created shared network as JSON.",
	}, dhcpSharedNetworkCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_shared_network_delete",
		Title:       "Delete a DHCP shared network",
		Annotations: destructiveTool("Delete a DHCP shared network"),
		Description: "Permanently removes a DHCP shared network from a server, identified by the server name " +
			"and the shared network name. This is destructive and cannot be undone from this server. " +
			"Depending on the appliance, deleting a shared network can also remove the scopes grouped " +
			"under it, so audit its members with solidserver_dhcp_scope_list first. When protected subnets " +
			"are configured, the member scopes are resolved and the delete is refused if any encloses or " +
			"overlaps a protected subnet. Returns a confirmation message.",
	}, dhcpSharedNetworkDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_group_list",
		Title:       "List DHCP groups",
		Annotations: readOnlyTool("List DHCP groups"),
		Description: "Enumerates DHCP groups, the containers that gather static reservations under shared " +
			"class parameters on a server. Use this to see how reservations are grouped, or to find the " +
			"group name that solidserver_dhcp_group_delete needs. Use solidserver_dhcp_static_list instead " +
			"for the individual reservations a group holds. Returns the group records as JSON.",
	}, dhcpGroupListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_group_create",
		Title:       "Create a DHCP group",
		Annotations: additiveTool("Create a DHCP group"),
		Description: "Creates a DHCP group on a server, a named container that gathers static reservations " +
			"under shared class parameters. The server must already exist; list them with " +
			"solidserver_dhcp_server_list. A group holds no reservations of its own; reservations are " +
			"created separately with solidserver_dhcp_static_add, and solidserver_dhcp_group_list shows " +
			"the groups already defined. Check for an existing one first, since a duplicate name is " +
			"rejected. Changes appliance state. Returns the created group as JSON.",
	}, dhcpGroupCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_group_delete",
		Title:       "Delete a DHCP group",
		Annotations: destructiveTool("Delete a DHCP group"),
		Description: "Permanently removes a DHCP group from a server, identified by the server name and the " +
			"group name. This is destructive and cannot be undone from this server. Depending on the " +
			"appliance, deleting a group can also remove the static reservations gathered under it, so " +
			"audit its members with solidserver_dhcp_static_list first. When protected subnets are " +
			"configured, the member reservations are resolved and the delete is refused if any address " +
			"falls inside a protected subnet. Returns a confirmation message.",
	}, dhcpGroupDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dhcp_static_list",
		Title:       "List static DHCP reservations",
		Annotations: readOnlyTool("List static DHCP reservations"),
		Description: "Enumerates static DHCP reservations, the fixed address-to-MAC bindings a server hands " +
			"out. Use this to audit which addresses are reserved before adding another with " +
			"solidserver_dhcp_static_add or removing one with solidserver_dhcp_static_delete, and to " +
			"recover the exact reserved address a delete needs. Use solidserver_dhcp_lease_list instead " +
			"for the dynamic leases currently in force. Returns the reservation records as JSON.",
	}, dhcpStaticListHandler(client, logger))
}

// ipv4MaskBits is the total bit width of an IPv4 mask.
const ipv4MaskBits = 32

// prefixToDottedMask converts an IPv4 prefix length to a dotted-decimal netmask
// (e.g. 24 to "255.255.255.0"), which is the form the DHCP scope endpoint's
// scope_net_mask field expects.
func prefixToDottedMask(prefix int) string {
	mask := net.CIDRMask(prefix, ipv4MaskBits)
	return net.IP(mask).String()
}

func dhcpServerListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpServerListInput) (*mcp.CallToolResult, DhcpServerListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpServerListInput) (*mcp.CallToolResult, DhcpServerListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_server_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpServerData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpServerList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpScopeListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpScopeListInput) (*mcp.CallToolResult, DhcpScopeListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpScopeListInput) (*mcp.CallToolResult, DhcpScopeListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_scope_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpScopeData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpScopeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpRangeListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpRangeListInput) (*mcp.CallToolResult, DhcpRangeListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpRangeListInput) (*mcp.CallToolResult, DhcpRangeListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_range_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpRangeData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpRangeList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpLeaseListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpLeaseListInput) (*mcp.CallToolResult, DhcpLeaseListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpLeaseListInput) (*mcp.CallToolResult, DhcpLeaseListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_lease_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpLeaseData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpLeaseList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpStaticAddHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpStaticAddInput) (*mcp.CallToolResult, DhcpStaticAddOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticAddInput) (*mcp.CallToolResult, DhcpStaticAddOut, error) {
		emptyOut := DhcpStaticAddOut{Data: make([]sdsclient.DataInnerDhcpStaticAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnet(in.IP); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIP(in.IP, "ip"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateDHCPMAC(in.MAC, "mac"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("adding static DHCP reservation", "name", in.Name, "ip", in.IP, "mac", in.MAC, "server", in.Server)
		input := sdsclient.DhcpStaticAddInput{
			ServerName:    &in.Server,
			StaticName:    &in.Name,
			StaticAddr:    &in.IP,
			StaticMacAddr: &in.MAC,
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpStaticAdd(authCtx).DhcpStaticAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_static_add", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpStaticAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpStaticAddSuccess, 0)
		}
		out := DhcpStaticAddOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpScopeCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpScopeCreateInput) (*mcp.CallToolResult, DhcpScopeCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpScopeCreateInput) (*mcp.CallToolResult, DhcpScopeCreateOut, error) {
		emptyOut := DhcpScopeCreateOut{Data: make([]sdsclient.DataInnerDhcpScopeAddSuccess, 0)}

		// Normalize address and prefix up front so the same value feeds the
		// guardrail, validation, and the appliance request. Without this, a
		// whitespace-padded address makes the protected-subnet CIDR unparseable so
		// the guardrail passes it (a bypass), while validation trims and accepts.
		address := strings.TrimSpace(in.Address)
		prefix := strings.TrimSpace(in.Prefix)

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnetCIDR(address, prefix); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateSubnetPrefix(address, prefix); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		addr, _ := netip.ParseAddr(address)
		if !addr.Is4() {
			return validationErrorResult(fmt.Errorf("address %q must be IPv4: DHCP scope creation supports IPv4 scopes", in.Address), emptyOut)
		}
		prefixInt, _ := strconv.Atoi(prefix)
		mask := prefixToDottedMask(prefixInt)

		logger.Info("creating DHCP scope", "server", in.Server, "address", address, "prefix", prefix)
		input := sdsclient.DhcpScopeAddInput{
			ServerName:   &in.Server,
			ScopeNetAddr: &address,
			ScopeNetMask: &mask,
		}
		if in.Name != "" {
			input.ScopeName = &in.Name
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpScopeAdd(authCtx).DhcpScopeAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_scope_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpScopeAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpScopeAddSuccess, 0)
		}
		out := DhcpScopeCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// validateDhcpRangeEndpoints checks the endpoint core that DHCP range create and
// delete share: a required server plus two valid same-family addresses forming a
// non-reversed [start, end] interval the appliance can describe. Both validators
// delegate here so the required-server, same-family, and non-reversed checks and
// their error strings cannot drift apart.
func validateDhcpRangeEndpoints(server, start, end string) error {
	if err := ValidateRequiredString(server, "server"); err != nil {
		return err
	}
	if err := ValidateIP(start, "start"); err != nil {
		return err
	}
	if err := ValidateIP(end, "end"); err != nil {
		return err
	}
	// Start and end are valid IPs here; require one range, not a reversed or
	// mixed-family pair the appliance cannot describe.
	startAddr, _ := netip.ParseAddr(strings.TrimSpace(start))
	endAddr, _ := netip.ParseAddr(strings.TrimSpace(end))
	if startAddr.Is4() != endAddr.Is4() {
		return fmt.Errorf("start %q and end %q must be the same address family", start, end)
	}
	if endAddr.Less(startAddr) {
		return fmt.Errorf("range end %q must not be before start %q", end, start)
	}
	return nil
}

func validateDhcpRangeCreateInput(in *DhcpRangeCreateInput) error {
	if err := validateDhcpRangeEndpoints(in.Server, in.Start, in.End); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Scope, "scope"); err != nil {
		return err
	}
	return ValidateOptionalString(in.Name, "name")
}

func dhcpRangeCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpRangeCreateInput) (*mcp.CallToolResult, DhcpRangeCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpRangeCreateInput) (*mcp.CallToolResult, DhcpRangeCreateOut, error) {
		emptyOut := DhcpRangeCreateOut{Data: make([]sdsclient.DataInnerDhcpRangeAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		// A range can start outside every protected subnet yet span into one, so
		// guard the whole [start, end] interval, not just the start address.
		if err := g.CheckProtectedRange(in.Start, in.End); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := validateDhcpRangeCreateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating DHCP range", "server", in.Server, "start", in.Start, "end", in.End)
		input := sdsclient.DhcpRangeAddInput{
			ServerName:     &in.Server,
			RangeStartAddr: &in.Start,
			RangeEndAddr:   &in.End,
		}
		if in.Scope != "" {
			input.ScopeName = &in.Scope
		}
		if in.Name != "" {
			input.RangeName = &in.Name
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpRangeAdd(authCtx).DhcpRangeAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_range_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpRangeAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpRangeAddSuccess, 0)
		}
		out := DhcpRangeCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpScopeDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpScopeDeleteInput) (*mcp.CallToolResult, DhcpScopeDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpScopeDeleteInput) (*mcp.CallToolResult, DhcpScopeDeleteOut, error) {
		emptyOut := DhcpScopeDeleteOut{Data: make([]sdsclient.DataInnerDhcpScopeDeleteSuccess, 0)}

		address := strings.TrimSpace(in.Address)

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		// Cheap first pass: catches deleting a scope whose network address sits
		// inside a protected subnet, without a lookup. The delete API identifies
		// the scope by server + net address only (no prefix), so the scope's real
		// size is unknown from the input; the enclosing case is handled by
		// resolving the true extent below.
		if err := g.CheckProtectedSubnet(address); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIPv4(address, "address"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// Resolve the scope's real extent from the appliance and refuse if it
		// encloses or overlaps a protected subnet. A caller-supplied prefix could
		// be narrowed to dodge this, so the size is taken from the appliance, not
		// the input.
		if res := applyDhcpScopeDeleteExtentProtection(ctx, client, logger, g, in.Server, address); res != nil {
			return res, emptyOut, nil
		}

		logger.Info("deleting DHCP scope", "server", in.Server, "address", address)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpScopeDelete(authCtx).
			ServerName(in.Server).
			ScopeNetAddr(address)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_scope_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpScopeDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpScopeDeleteSuccess, 0)
		}
		out := DhcpScopeDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// applyDhcpScopeDeleteExtentProtection refuses a scope delete when any scope at
// (server, netAddr) encloses or overlaps a protected subnet. It is a no-op
// (returns nil) when no protected subnets are configured or there is no client,
// so the caller can invoke it unconditionally.
func applyDhcpScopeDeleteExtentProtection(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, server, netAddr string) *mcp.CallToolResult {
	if client == nil || g == nil || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	extents, errResult := lookupDhcpScopeExtents(ctx, client, logger, server, netAddr)
	if errResult != nil {
		return errResult
	}
	for _, e := range extents {
		// A resolved scope with no usable extent cannot be checked; fail closed
		// rather than let an unverifiable delete through (matching subnet_update).
		if !e.parseable() {
			return errorResult("cannot verify protected-subnet rules: a scope at %q resolved no usable address extent", netAddr)
		}
		if p, ok := g.overlappingProtectedSubnet(e.start, e.end); ok {
			return errorResult("cannot delete scope %q-%q overlapping protected subnet %q", e.start, e.end, p)
		}
	}
	return nil
}

// lookupDhcpScopeExtents returns the inclusive [start, end] extents of the
// scopes matching (server, netAddr). A (server, netAddr) pair is normally one
// scope, but all matches are returned so every candidate the delete could hit
// is checked. A miss returns no extents and a nil error: nothing to protect and
// the delete itself reports the miss.
func lookupDhcpScopeExtents(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, server, netAddr string) ([]addrExtent, *mcp.CallToolResult) {
	where := fmt.Sprintf("scope_net_addr='%s' AND server_name='%s'", EscapeWhereValue(netAddr), EscapeWhereValue(server))
	return runDhcpScopeExtentQuery(ctx, client, logger, "solidserver_dhcp_scope_delete", where)
}

func dhcpRangeDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpRangeDeleteInput) (*mcp.CallToolResult, DhcpRangeDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpRangeDeleteInput) (*mcp.CallToolResult, DhcpRangeDeleteOut, error) {
		emptyOut := DhcpRangeDeleteOut{Data: make([]sdsclient.DataInnerDhcpRangeDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		// The range can start outside every protected subnet yet span into one, so
		// guard the whole [start, end] interval, not just the start address.
		if p, ok := g.overlappingProtectedSubnet(in.Start, in.End); ok {
			return errorResult("cannot delete a range %q-%q overlapping protected subnet %q", in.Start, in.End, p), emptyOut, nil
		}

		if err := validateDhcpRangeDeleteInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting DHCP range", "server", in.Server, "start", in.Start, "end", in.End)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpRangeDelete(authCtx).
			ServerName(in.Server).
			RangeStartAddr(in.Start).
			RangeEndAddr(in.End)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_range_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpRangeDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpRangeDeleteSuccess, 0)
		}
		out := DhcpRangeDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// validateDhcpRangeDeleteInput shares its endpoint core with
// validateDhcpRangeCreateInput via validateDhcpRangeEndpoints: both endpoints
// must be valid same-family addresses forming a non-reversed range, so the
// delete targets one coherent interval the appliance can describe.
func validateDhcpRangeDeleteInput(in *DhcpRangeDeleteInput) error {
	return validateDhcpRangeEndpoints(in.Server, in.Start, in.End)
}

func dhcpStaticDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpStaticDeleteInput) (*mcp.CallToolResult, DhcpStaticDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticDeleteInput) (*mcp.CallToolResult, DhcpStaticDeleteOut, error) {
		emptyOut := DhcpStaticDeleteOut{Data: make([]sdsclient.DataInnerDhcpStaticDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnet(in.IP); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIP(in.IP, "ip"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting static DHCP reservation", "ip", in.IP, "server", in.Server)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpStaticDelete(authCtx).
			ServerName(in.Server).
			StaticAddr(in.IP)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_static_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpStaticDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpStaticDeleteSuccess, 0)
		}
		out := DhcpStaticDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpSharedNetworkListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpSharedNetworkListInput) (*mcp.CallToolResult, DhcpSharedNetworkListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpSharedNetworkListInput) (*mcp.CallToolResult, DhcpSharedNetworkListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_shared_network_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpSharednetworkData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpSharednetworkList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpGroupListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpGroupListInput) (*mcp.CallToolResult, DhcpGroupListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpGroupListInput) (*mcp.CallToolResult, DhcpGroupListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_group_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpGroupData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpGroupList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpStaticListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DhcpStaticListInput) (*mcp.CallToolResult, DhcpStaticListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpStaticListInput) (*mcp.CallToolResult, DhcpStaticListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dhcp_static_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDhcpStaticData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DhcpAPI.DhcpStaticList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

func dhcpSharedNetworkCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpSharedNetworkCreateInput) (*mcp.CallToolResult, DhcpSharedNetworkCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpSharedNetworkCreateInput) (*mcp.CallToolResult, DhcpSharedNetworkCreateOut, error) {
		emptyOut := DhcpSharedNetworkCreateOut{Data: make([]sdsclient.DataInnerDhcpSharednetworkAddSuccess, 0)}

		// A shared network is a name-identified container with no address extent,
		// so only the read-only guard applies; there is nothing for the
		// protected-subnet guardrails to key on.
		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating DHCP shared network", "server", in.Server, "name", in.Name)
		input := sdsclient.DhcpSharednetworkAddInput{
			ServerName:        &in.Server,
			SharednetworkName: &in.Name,
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpSharednetworkAdd(authCtx).DhcpSharednetworkAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_shared_network_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpSharednetworkAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpSharednetworkAddSuccess, 0)
		}
		out := DhcpSharedNetworkCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpSharedNetworkDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpSharedNetworkDeleteInput) (*mcp.CallToolResult, DhcpSharedNetworkDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpSharedNetworkDeleteInput) (*mcp.CallToolResult, DhcpSharedNetworkDeleteOut, error) {
		emptyOut := DhcpSharedNetworkDeleteOut{Data: make([]sdsclient.DataInnerDhcpSharednetworkDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// A shared network carries no address of its own, but deleting it can
		// take its member scopes with it, so resolve those scopes and refuse if
		// any overlaps a protected subnet before the destructive call.
		if res := applyDhcpSharedNetworkDeleteExtentProtection(ctx, client, logger, g, in.Server, in.Name); res != nil {
			return res, emptyOut, nil
		}

		logger.Info("deleting DHCP shared network", "server", in.Server, "name", in.Name)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpSharednetworkDelete(authCtx).
			ServerName(in.Server).
			SharednetworkName(in.Name)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_shared_network_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpSharednetworkDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpSharednetworkDeleteSuccess, 0)
		}
		out := DhcpSharedNetworkDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpGroupCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpGroupCreateInput) (*mcp.CallToolResult, DhcpGroupCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpGroupCreateInput) (*mcp.CallToolResult, DhcpGroupCreateOut, error) {
		emptyOut := DhcpGroupCreateOut{Data: make([]sdsclient.DataInnerDhcpGroupAddSuccess, 0)}

		// A group is a name-identified reservation container with no address
		// extent, so only the read-only guard applies.
		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating DHCP group", "server", in.Server, "name", in.Name)
		input := sdsclient.DhcpGroupAddInput{
			ServerName: &in.Server,
			GroupName:  &in.Name,
		}

		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpGroupAdd(authCtx).DhcpGroupAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_group_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpGroupAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpGroupAddSuccess, 0)
		}
		out := DhcpGroupCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dhcpGroupDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DhcpGroupDeleteInput) (*mcp.CallToolResult, DhcpGroupDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DhcpGroupDeleteInput) (*mcp.CallToolResult, DhcpGroupDeleteOut, error) {
		emptyOut := DhcpGroupDeleteOut{Data: make([]sdsclient.DataInnerDhcpGroupDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// A group carries no address of its own, but deleting it can take its
		// member static reservations with it, so resolve those reservations and
		// refuse if any address sits inside a protected subnet.
		if res := applyDhcpGroupDeleteStaticProtection(ctx, client, logger, g, in.Server, in.Name); res != nil {
			return res, emptyOut, nil
		}

		logger.Info("deleting DHCP group", "server", in.Server, "name", in.Name)
		authCtx := client.AuthContext(ctx)
		req := client.DhcpAPI.DhcpGroupDelete(authCtx).
			ServerName(in.Server).
			GroupName(in.Name)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dhcp_group_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDhcpGroupDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDhcpGroupDeleteSuccess, 0)
		}
		out := DhcpGroupDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// scopeExtentsFromRows extracts each scope row's inclusive [start, end] extent,
// preferring the resolved start/end addresses and falling back to the network
// address. Shared by the scope-delete and shared-network-delete guards so both
// read a scope's extent the same way. An empty field means the extent could not
// be resolved; the caller fails closed on that.
func scopeExtentsFromRows(rows []sdsclient.DataInnerDhcpScopeData) []addrExtent {
	extents := make([]addrExtent, 0, len(rows))
	for i := range rows {
		start, end := scopeRowExtent(&rows[i])
		extents = append(extents, addrExtent{start: start, end: end})
	}
	return extents
}

// scopeRowExtent computes a scope's inclusive [start, end] as dotted-decimal
// strings. The scope_start_address_addr and scope_end_address_addr fields are
// HEXADECIMAL in this API (per the SDK model docs, e.g. "0a050000"), which
// netip cannot parse, so the extent is derived from the dotted scope_net_addr
// plus the scope's address count (scope_size, else the dotted scope_net_mask).
// It returns empty strings when the base address or the count cannot be
// determined, so the caller fails closed rather than checking an unverifiable
// extent. Reading the hex fields here is exactly what let a protected member
// scope slip past the guard, so they are deliberately not read.
func scopeRowExtent(row *sdsclient.DataInnerDhcpScopeData) (start, end string) {
	if row.ScopeNetAddr == nil {
		return "", ""
	}
	base, err := netip.ParseAddr(strings.TrimSpace(*row.ScopeNetAddr))
	if err != nil || !base.Is4() {
		return "", ""
	}
	size, ok := scopeAddressCount(row)
	if !ok {
		return "", ""
	}
	last, ok := ipv4AddrPlus(base, size-1)
	if !ok {
		return "", ""
	}
	return base.String(), last.String()
}

// scopeAddressCount returns how many addresses a scope spans, from the scope_size
// count when present and parseable, else derived from the dotted scope_net_mask.
func scopeAddressCount(row *sdsclient.DataInnerDhcpScopeData) (uint64, bool) {
	if row.ScopeSize != nil {
		if n, err := strconv.ParseUint(strings.TrimSpace(*row.ScopeSize), 10, 64); err == nil && n > 0 {
			return n, true
		}
	}
	if row.ScopeNetMask != nil {
		if ones, ok := dottedMaskPrefixLen(strings.TrimSpace(*row.ScopeNetMask)); ok {
			return uint64(1) << uint(ipv4MaskBits-ones), true
		}
	}
	return 0, false
}

// dottedMaskPrefixLen converts a dotted IPv4 netmask (e.g. "255.255.255.0") to
// its prefix length, reporting false for anything that is not a canonical IPv4
// mask.
func dottedMaskPrefixLen(mask string) (int, bool) {
	ip := net.ParseIP(mask)
	if ip == nil {
		return 0, false
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, false
	}
	ones, bits := net.IPMask(v4).Size()
	if bits != ipv4MaskBits {
		return 0, false
	}
	return ones, true
}

// ipv4AddrPlus returns base advanced by offset addresses, reporting false when
// base is not IPv4 or the result overflows the 32-bit space.
func ipv4AddrPlus(base netip.Addr, offset uint64) (netip.Addr, bool) {
	if !base.Is4() {
		return netip.Addr{}, false
	}
	// A valid IPv4 offset never exceeds the 32-bit space; rejecting a larger one
	// up front also stops base+offset from wrapping the uint64 addition below,
	// which could otherwise land back under MaxUint32 and slip past that guard.
	if offset > math.MaxUint32 {
		return netip.Addr{}, false
	}
	b := base.As4()
	sum := uint64(binary.BigEndian.Uint32(b[:])) + offset
	if sum > math.MaxUint32 {
		return netip.Addr{}, false
	}
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], uint32(sum))
	return netip.AddrFrom4(out), true
}

// runDhcpScopeExtentQuery runs a scope/list query and returns the matched scopes'
// dotted extents. It fails closed when the page fills to maxListLimit, since a
// member scope beyond the page could sit inside a protected subnet and go
// unchecked. toolName labels the log line on an API error.
func runDhcpScopeExtentQuery(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, toolName, where string) ([]addrExtent, *mcp.CallToolResult) {
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.DhcpAPI.DhcpScopeList(authCtx).Where(where).Limit(maxListLimit).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", toolName, "error", apiErr)
		return nil, apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	if len(resp.Data) >= maxListLimit {
		return nil, errorResult("cannot verify protected-subnet rules: matched %d or more scopes, more than can be enumerated in one page", len(resp.Data))
	}
	return scopeExtentsFromRows(resp.Data), nil
}

// applyDhcpSharedNetworkDeleteExtentProtection refuses a shared-network delete
// when any scope grouped under it encloses or overlaps a protected subnet.
// Whether the appliance cascade-deletes member scopes when the shared network is
// removed is NOT MEASURED here against a live appliance, so the member scopes are
// resolved and checked before the destructive call, failing closed the same way
// the scope-delete guard does. It is a no-op (returns nil) when no protected
// subnets are configured or there is no client, so the caller can invoke it
// unconditionally.
func applyDhcpSharedNetworkDeleteExtentProtection(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, server, name string) *mcp.CallToolResult {
	if client == nil || g == nil || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	extents, errResult := lookupDhcpSharedNetworkScopeExtents(ctx, client, logger, server, name)
	if errResult != nil {
		return errResult
	}
	for _, e := range extents {
		if !e.parseable() {
			return errorResult("cannot verify protected-subnet rules: a scope under shared network %q resolved no usable address extent", name)
		}
		if p, ok := g.overlappingProtectedSubnet(e.start, e.end); ok {
			return errorResult("cannot delete shared network %q: its member scope %q-%q overlaps protected subnet %q", name, e.start, e.end, p)
		}
	}
	return nil
}

// parseable reports whether both ends of the extent parse as IP addresses. An
// empty or unparseable end means the extent could not be resolved to a real
// address span, so the guard treats it as unverifiable and fails closed rather
// than letting it fall through overlappingProtectedSubnet's parse-and-skip.
func (e addrExtent) parseable() bool {
	if e.start == "" || e.end == "" {
		return false
	}
	_, startErr := netip.ParseAddr(strings.TrimSpace(e.start))
	_, endErr := netip.ParseAddr(strings.TrimSpace(e.end))
	return startErr == nil && endErr == nil
}

// lookupDhcpSharedNetworkScopeExtents returns the inclusive [start, end] extents
// of the scopes grouped under (server, name). A miss returns no extents and a nil
// error: nothing to protect, and the delete itself reports the miss.
func lookupDhcpSharedNetworkScopeExtents(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, server, name string) ([]addrExtent, *mcp.CallToolResult) {
	where := fmt.Sprintf("sharednetwork_name='%s' AND server_name='%s'", EscapeWhereValue(name), EscapeWhereValue(server))
	return runDhcpScopeExtentQuery(ctx, client, logger, "solidserver_dhcp_shared_network_delete", where)
}

// applyDhcpGroupDeleteStaticProtection refuses a group delete when any static
// reservation gathered under it sits inside a protected subnet. Whether the
// appliance cascade-deletes member reservations when the group is removed is NOT
// MEASURED here against a live appliance, so the members are resolved and checked
// before the destructive call. An address that will not parse fails closed rather
// than falling through CheckProtectedSubnet's parse-and-skip. No-op when no
// protected subnets are configured or there is no client.
func applyDhcpGroupDeleteStaticProtection(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, server, name string) *mcp.CallToolResult {
	if client == nil || g == nil || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	addrs, errResult := lookupDhcpGroupStaticAddrs(ctx, client, logger, server, name)
	if errResult != nil {
		return errResult
	}
	for _, a := range addrs {
		if _, err := netip.ParseAddr(strings.TrimSpace(a)); err != nil {
			return errorResult("cannot verify protected-subnet rules: a reservation under group %q resolved no usable address", name)
		}
		if err := g.CheckProtectedSubnet(a); err != nil {
			return errorResult("cannot delete group %q: %v", name, err)
		}
	}
	return nil
}

// lookupDhcpGroupStaticAddrs returns the reservation addresses gathered under
// (server, name). A miss returns no addresses and a nil error.
func lookupDhcpGroupStaticAddrs(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, server, name string) ([]string, *mcp.CallToolResult) {
	where := fmt.Sprintf("group_name='%s' AND server_name='%s'", EscapeWhereValue(name), EscapeWhereValue(server))
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.DhcpAPI.DhcpStaticList(authCtx).Where(where).Limit(maxListLimit).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_dhcp_group_delete", "error", apiErr)
		return nil, apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	// Fail closed on a full page: a reservation beyond it could sit inside a
	// protected subnet and would go unchecked.
	if len(resp.Data) >= maxListLimit {
		return nil, errorResult("cannot verify protected-subnet rules: group %q has %d or more reservations, more than can be enumerated in one page", name, len(resp.Data))
	}
	addrs := make([]string, 0, len(resp.Data))
	for i := range resp.Data {
		// static_addr is dotted-decimal; static_address_addr is hexadecimal in
		// this API, so only the dotted field is usable. A reservation with no
		// dotted address resolves to "" and the caller fails closed on it.
		addr := ""
		if resp.Data[i].StaticAddr != nil {
			addr = *resp.Data[i].StaticAddr
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}
