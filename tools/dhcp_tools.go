package tools

import (
	"context"
	"fmt"
	"log/slog"
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

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnet(in.Address + "/" + in.Prefix); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateSubnetPrefix(in.Address, in.Prefix); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		// ValidateSubnetPrefix (above) trims before parsing, so a whitespace-padded
		// address or prefix passes it; trim here too so the re-parse sees the same
		// value and never silently yields a zero prefix (mask 0.0.0.0) or a false
		// IPv6 rejection. Both errors are dead: validation already accepted these.
		addr, _ := netip.ParseAddr(strings.TrimSpace(in.Address))
		if !addr.Is4() {
			return validationErrorResult(fmt.Errorf("address %q must be IPv4: DHCP scope creation supports IPv4 scopes", in.Address), emptyOut)
		}
		prefixInt, _ := strconv.Atoi(strings.TrimSpace(in.Prefix))
		mask := prefixToDottedMask(prefixInt)

		logger.Info("creating DHCP scope", "server", in.Server, "address", in.Address, "prefix", in.Prefix)
		input := sdsclient.DhcpScopeAddInput{
			ServerName:   &in.Server,
			ScopeNetAddr: &in.Address,
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

func validateDhcpRangeCreateInput(in *DhcpRangeCreateInput) error {
	if err := ValidateRequiredString(in.Server, "server"); err != nil {
		return err
	}
	if err := ValidateIP(in.Start, "start"); err != nil {
		return err
	}
	if err := ValidateIP(in.End, "end"); err != nil {
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
