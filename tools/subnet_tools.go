package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// Subnet Input Structs
type SubnetListInput struct {
	Space  string `json:"space" jsonschema:"The name of the space."`
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering (e.g., \"subnet_name LIKE 'lan%'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type SubnetInfoInput struct {
	ID    int32  `json:"id,omitempty" jsonschema:"The numeric ID of the subnet. Provide this or cidr; id wins when both are given."`
	CIDR  string `json:"cidr,omitempty" jsonschema:"CIDR of the subnet to resolve when its id is unknown (e.g. '10.0.0.0/24')."`
	Space string `json:"space,omitempty" jsonschema:"Space name to disambiguate a cidr that exists in more than one space (optional)."`
}

type SpaceListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type SpaceCreateInput struct {
	Name        string `json:"name" jsonschema:"The name of the space to create."`
	Description string `json:"description,omitempty" jsonschema:"An optional human-readable description for the space."`
}

type SpaceDeleteInput struct {
	Name string `json:"name" jsonschema:"The name of the space to delete."`
}

type SubnetCreateInput struct {
	Space   string `json:"space" jsonschema:"The name of the space."`
	Address string `json:"address" jsonschema:"The start IP address of the subnet."`
	Prefix  string `json:"prefix" jsonschema:"The prefix length (e.g. '24')."`
	Name    string `json:"name" jsonschema:"The name of the subnet."`
}

type SubnetDeleteInput struct {
	Space   string `json:"space" jsonschema:"The name of the space."`
	Address string `json:"address" jsonschema:"The start IP address of the subnet to delete."`
}

// Subnet Output Structs
type SubnetListOut = ListOutput[sdsclient.DataInnerIpamNetworkData]

type SubnetInfoOut struct {
	Data []sdsclient.DataInnerIpamNetworkData `json:"data" jsonschema:"Subnet detail records including usage."`
}

type SubnetCreateOut struct {
	Data []sdsclient.DataInnerIpamNetworkAddSuccess `json:"data" jsonschema:"Created subnet records."`
}

type SubnetDeleteOut struct {
	Data []sdsclient.DataInnerIpamNetworkDeleteSuccess `json:"data" jsonschema:"Deleted subnet response records."`
}

type SpaceListOut = ListOutput[sdsclient.DataInnerIpamSpaceData]

type SpaceCreateOut struct {
	Data []sdsclient.DataInnerIpamSpaceAddSuccess `json:"data" jsonschema:"Created space records."`
}

type SpaceDeleteOut struct {
	Data []sdsclient.DataInnerIpamSpaceDeleteSuccess `json:"data" jsonschema:"Deleted space response records."`
}

// RegisterSubnetTools registers subnet and space management tools.
func RegisterSubnetTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_list",
		Title:       "List subnets",
		Annotations: readOnlyTool("List subnets"),
		Description: "Enumerates subnets, optionally scoped to a space or narrowed by a where " +
			"clause. Use this to find a subnet's name or ID before allocating an address with " +
			"solidserver_ip_create. Returns a summary row per subnet; use solidserver_subnet_info " +
			"instead when you already have one subnet's ID and need its full detail including " +
			"utilisation. Returns the matching subnets as JSON.",
	}, subnetListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_info",
		Title:       "Get subnet details",
		Annotations: readOnlyTool("Get subnet details"),
		Description: "Returns the full detail for one subnet, including size and usage. Identify the " +
			"subnet either by its numeric ID (typically from solidserver_subnet_list) or by its CIDR, " +
			"which is resolved to the matching terminal subnet; pass a space to disambiguate a CIDR " +
			"that exists in more than one space. Use solidserver_subnet_list instead to search or to " +
			"enumerate. Use solidserver_ip_list to see the addresses inside the subnet rather than the " +
			"subnet itself. Returns the subnet record as JSON.",
	}, subnetInfoHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_create",
		Title:       "Create a subnet",
		Annotations: additiveTool("Create a subnet"),
		Description: "Carves a new subnet out of an existing space. The space must already exist; " +
			"list them with solidserver_space_list. Check for an overlapping range with " +
			"solidserver_subnet_list first, since an overlap is rejected rather than merged. Changes " +
			"appliance state and is undone only by solidserver_subnet_delete. Returns the created " +
			"subnet as JSON.",
	}, subnetCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_delete",
		Title:       "Delete a subnet",
		Annotations: destructiveTool("Delete a subnet"),
		Description: "Permanently removes a subnet from a space. This is destructive, cannot be " +
			"undone from this server, and takes the addresses tracked inside the subnet with it, so " +
			"check what is allocated with solidserver_ip_list before calling it. Deleting a subnet " +
			"that is still in use loses the record of which hosts held which addresses. Returns a " +
			"confirmation message.",
	}, subnetDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_space_list",
		Title:       "List IPAM spaces",
		Annotations: readOnlyTool("List IPAM spaces"),
		Description: "Enumerates IPAM spaces. A space is the top-level container that subnets and " +
			"addresses live in, and separate spaces may reuse the same RFC 1918 ranges, so start " +
			"here when you do not already know which space to work in: an address or CIDR is " +
			"ambiguous without one. Use solidserver_subnet_list instead to see the subnets inside a " +
			"space. Returns the space records as JSON.",
	}, spaceListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_space_create",
		Title:       "Create an IPAM space",
		Annotations: additiveTool("Create an IPAM space"),
		Description: "Creates a new top-level IPAM space to hold subnets and addresses. Check whether " +
			"the name is already taken with solidserver_space_list first, since space names are how " +
			"every other tool selects where to work and a duplicate is rejected rather than merged. A " +
			"new space starts empty; carve subnets into it with solidserver_subnet_create afterwards. " +
			"Changes appliance state and is undone only by solidserver_space_delete. Returns the " +
			"created space as JSON.",
	}, spaceCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_space_delete",
		Title:       "Delete an IPAM space",
		Annotations: destructiveTool("Delete an IPAM space"),
		Description: "Permanently removes an IPAM space. This is destructive and cannot be undone from " +
			"this server; deleting a space takes every subnet and address recorded inside it with it, " +
			"so audit it with solidserver_subnet_list first. Deleting a space that is still in use " +
			"loses the record of which hosts held which addresses. Recreating it with " +
			"solidserver_space_create restores nothing. Returns a confirmation message.",
	}, spaceDeleteHandler(client, logger, g))
}

//nolint:dupl // similar list logic across modules
func subnetListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetListInput) (*mcp.CallToolResult, SubnetListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetListInput) (*mcp.CallToolResult, SubnetListOut, error) {
		emptyOut := SubnetListOut{Data: make([]sdsclient.DataInnerIpamNetworkData, 0), Limit: clampLimit(in.Limit), Offset: in.Offset}
		if err := ValidateOptionalString(in.Space, "space"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_subnet_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerIpamNetworkData, *http.Response, error) {
				fixed := ""
				if in.Space != "" {
					fixed = fmt.Sprintf("site_name='%s'", EscapeWhereValue(in.Space))
				}
				w := CombineWhereClause(fixed, where)
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamNetworkList(authCtx).Limit(limit).Offset(offset)
				if w != "" {
					req = req.Where(w)
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

// resolveSubnetIDByCIDR resolves a CIDR to the numeric ID of the matching
// terminal subnet. SolidServer stores each subnet's network address in the
// network_start_hostaddr column (IPv4 dotted, IPv6 canonical), which equals the
// masked CIDR address, so filtering on it plus network_is_terminal='1'
// (optionally scoped by space)
// selects the terminal subnet. Terminal subnets cannot overlap within one
// space, so start address plus terminal flag is unique per space; across spaces
// it can repeat, hence the optional space filter and the size-based
// disambiguation below. Returns the id, or a ready-to-return error result.
func resolveSubnetIDByCIDR(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, cidr, space string) (int32, *mcp.CallToolResult) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return 0, errorResult("invalid parameter: cidr %q is not a valid CIDR", cidr)
	}
	prefix = prefix.Masked()
	startAddr := prefix.Addr().String()

	fixed := fmt.Sprintf("network_start_hostaddr='%s' AND network_is_terminal='1'", EscapeWhereValue(startAddr))
	if space != "" {
		fixed = fmt.Sprintf("%s AND site_name='%s'", fixed, EscapeWhereValue(space))
	}

	logger.Info("resolving subnet by CIDR", "cidr", cidr, "space", space)
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.IpamAPI.IpamNetworkList(authCtx).Where(fixed).Limit(maxListLimit).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_subnet_info", "error", apiErr)
		return 0, apiErrorResult(apiErr, httpResp)
	}

	var rows []sdsclient.DataInnerIpamNetworkData
	if resp != nil && resp.Data != nil {
		rows = resp.Data
	}
	matched := disambiguateSubnetsBySize(rows, prefix)
	switch len(matched) {
	case 0:
		return 0, errorResult("no terminal subnet found for cidr %s; use solidserver_subnet_list to search", cidr)
	case 1:
		if matched[0].NetworkId == nil {
			return 0, errorResult("subnet for cidr %s has no id; use solidserver_subnet_list", cidr)
		}
		id, convErr := strconv.ParseInt(*matched[0].NetworkId, 10, 32)
		if convErr != nil || id <= 0 {
			return 0, errorResult("subnet for cidr %s has an unusable id; use solidserver_subnet_list", cidr)
		}
		return int32(id), nil
	default:
		return 0, errorResult("cidr %s matches %d subnets; pass a space to disambiguate or use the numeric id", cidr, len(matched))
	}
}

// disambiguateSubnetsBySize narrows candidate subnets that share a start address
// to those whose address count matches the requested prefix (usually one). When
// the count cannot be computed (IPv6 prefixes wider than a uint64 can hold) or
// no row carries a matching size, the rows are returned unfiltered so the caller
// reports the ambiguity rather than guessing.
func disambiguateSubnetsBySize(rows []sdsclient.DataInnerIpamNetworkData, prefix netip.Prefix) []sdsclient.DataInnerIpamNetworkData {
	if len(rows) <= 1 {
		return rows
	}
	hostBits := prefix.Addr().BitLen() - prefix.Bits()
	if hostBits < 0 || hostBits > 63 {
		return rows
	}
	want := strconv.FormatUint(uint64(1)<<uint(hostBits), 10)
	out := make([]sdsclient.DataInnerIpamNetworkData, 0, len(rows))
	for i := range rows {
		if rows[i].NetworkSize != nil && *rows[i].NetworkSize == want {
			out = append(out, rows[i])
		}
	}
	if len(out) == 0 {
		return rows
	}
	return out
}

func subnetInfoHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetInfoInput) (*mcp.CallToolResult, SubnetInfoOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetInfoInput) (*mcp.CallToolResult, SubnetInfoOut, error) {
		emptyOut := SubnetInfoOut{Data: make([]sdsclient.DataInnerIpamNetworkData, 0)}

		id := in.ID
		if id <= 0 {
			if strings.TrimSpace(in.CIDR) == "" {
				return validationErrorResult(fmt.Errorf("provide either id or cidr"), emptyOut)
			}
			resolved, errResult := resolveSubnetIDByCIDR(ctx, client, logger, in.CIDR, in.Space)
			if errResult != nil {
				return errResult, emptyOut, nil
			}
			id = resolved
		}

		logger.Info("fetching subnet details", "subnet_id", id)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkInfo(authCtx).NetworkId(id)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_subnet_info", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamNetworkData
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamNetworkData, 0)
		}
		out := SubnetInfoOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func checkSubnetCreateGuardrails(g *Guardrails, in *SubnetCreateInput) error {
	if err := g.CheckReadOnly(); err != nil {
		return err
	}
	if err := g.CheckProtectedSpace(in.Space); err != nil {
		return err
	}
	if in.Address != "" && in.Prefix != "" {
		cidr := in.Address + "/" + in.Prefix
		return g.CheckProtectedSubnet(cidr)
	}
	return g.CheckProtectedSubnet(in.Address)
}

func validateSubnetCreateInput(in *SubnetCreateInput) error {
	if err := ValidateRequiredString(in.Space, "space"); err != nil {
		return err
	}
	if err := ValidateRequiredString(in.Name, "name"); err != nil {
		return err
	}
	return ValidateSubnetPrefix(in.Address, in.Prefix)
}

func subnetCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SubnetCreateInput) (*mcp.CallToolResult, SubnetCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetCreateInput) (*mcp.CallToolResult, SubnetCreateOut, error) {
		emptyOut := SubnetCreateOut{Data: make([]sdsclient.DataInnerIpamNetworkAddSuccess, 0)}

		if err := checkSubnetCreateGuardrails(g, &in); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := validateSubnetCreateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating subnet", "name", in.Name, "address", in.Address, "prefix", in.Prefix, "space", in.Space)
		input := sdsclient.IpamNetworkAddInput{
			SpaceName:     &in.Space,
			NetworkAddr:   &in.Address,
			NetworkPrefix: &in.Prefix,
			NetworkName:   &in.Name,
		}

		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkAdd(authCtx).IpamNetworkAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_subnet_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamNetworkAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamNetworkAddSuccess, 0)
		}
		out := SubnetCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func subnetDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SubnetDeleteInput) (*mcp.CallToolResult, SubnetDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetDeleteInput) (*mcp.CallToolResult, SubnetDeleteOut, error) {
		emptyOut := SubnetDeleteOut{Data: make([]sdsclient.DataInnerIpamNetworkDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Space); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnet(in.Address); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIP(in.Address, "address"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting subnet", "address", in.Address, "space", in.Space)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkDelete(authCtx).
			SpaceName(in.Space).
			NetworkAddr(in.Address)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_subnet_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamNetworkDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamNetworkDeleteSuccess, 0)
		}
		out := SubnetDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func spaceCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SpaceCreateInput) (*mcp.CallToolResult, SpaceCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SpaceCreateInput) (*mcp.CallToolResult, SpaceCreateOut, error) {
		emptyOut := SpaceCreateOut{Data: make([]sdsclient.DataInnerIpamSpaceAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Name); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.Description, "description"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating space", "name", in.Name)
		input := sdsclient.IpamSpaceAddInput{SpaceName: &in.Name}
		if in.Description != "" {
			input.SpaceDescription = &in.Description
		}

		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamSpaceAdd(authCtx).IpamSpaceAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_space_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamSpaceAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamSpaceAddSuccess, 0)
		}
		out := SpaceCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func spaceDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SpaceDeleteInput) (*mcp.CallToolResult, SpaceDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SpaceDeleteInput) (*mcp.CallToolResult, SpaceDeleteOut, error) {
		emptyOut := SpaceDeleteOut{Data: make([]sdsclient.DataInnerIpamSpaceDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Name); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting space", "name", in.Name)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamSpaceDelete(authCtx).SpaceName(in.Name)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_space_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamSpaceDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamSpaceDeleteSuccess, 0)
		}
		out := SpaceDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

//nolint:dupl // similar list logic across modules
func spaceListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SpaceListInput) (*mcp.CallToolResult, SpaceListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SpaceListInput) (*mcp.CallToolResult, SpaceListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_space_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerIpamSpaceData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamSpaceList(authCtx).Limit(limit).Offset(offset)
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
