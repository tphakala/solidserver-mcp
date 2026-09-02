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

type SubnetUpdateInput struct {
	SubnetID  int32  `json:"subnet_id" jsonschema:"Numeric subnet (network) ID from solidserver_subnet_list or solidserver_subnet_info."`
	Name      string `json:"name,omitempty" jsonschema:"New name for the subnet. Omit to leave unchanged."`
	Prefix    string `json:"prefix,omitempty" jsonschema:"New prefix length to resize the subnet to (e.g. '25'). Omit to leave the size unchanged. Growing a subnet can be rejected if the larger range would overlap a protected subnet or an existing neighbour."`
	ClassName string `json:"class_name,omitempty" jsonschema:"New class to apply, as 'directory/name.class' (reclassify). Omit to leave unchanged."`
	Space     string `json:"space,omitempty" jsonschema:"IPAM space name, used only for the protected-space guardrail."`
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

type SubnetUpdateOut struct {
	Data []sdsclient.DataInnerIpamNetworkEditSuccess `json:"data" jsonschema:"Updated subnet response records."`
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
		Name:        "solidserver_subnet_update",
		Title:       "Update a subnet",
		Annotations: additiveTool("Update a subnet"),
		Description: "Edits an existing subnet in place, identified by its numeric ID from " +
			"solidserver_subnet_list or solidserver_subnet_info. Renames it, resizes it to a new " +
			"prefix, or reclassifies it, without the delete-and-recreate that would change its identity " +
			"and lose the addresses tracked inside it. Set only the fields you want to change; omitted " +
			"fields are left as they are. A resize that grows the subnet can be rejected if the larger " +
			"range would overlap a protected subnet or an existing neighbour. Changes appliance state. " +
			"Returns the updated subnet as JSON.",
	}, subnetUpdateHandler(client, logger, g))

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
		// A lone row matches only on start address; the size filter above runs
		// only when more than one row competes. Reject a single candidate whose
		// size contradicts the requested prefix so a /24 lookup cannot silently
		// resolve to a /25 that happens to share the start address.
		if want, ok := expectedNetworkSize(prefix); ok && matched[0].NetworkSize != nil && *matched[0].NetworkSize != want {
			return 0, errorResult("no terminal subnet found for cidr %s: a subnet starts at %s but its size does not match /%d; use solidserver_subnet_list to search", cidr, startAddr, prefix.Bits())
		}
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

// expectedNetworkSize returns the SolidServer network_size string for a prefix
// (the count of addresses it contains) and whether that count is computable. It
// is not computable for prefixes with more than 63 host bits, where the count
// overflows a uint64; callers treat "not computable" as "cannot disambiguate by
// size" rather than guessing.
func expectedNetworkSize(prefix netip.Prefix) (string, bool) {
	hostBits := prefix.Addr().BitLen() - prefix.Bits()
	if hostBits < 0 || hostBits > 63 {
		return "", false
	}
	return strconv.FormatUint(uint64(1)<<uint(hostBits), 10), true
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
	want, ok := expectedNetworkSize(prefix)
	if !ok {
		return rows
	}
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
		return g.CheckProtectedSubnetCIDR(in.Address, in.Prefix)
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

		// The bare-address check above catches deleting a subnet that equals or
		// sits inside a protected one, but not a larger terminal subnet that
		// encloses a smaller protected subnet (its start address lies outside the
		// protected range). Resolve the target's real extent and re-check.
		if res := applySubnetDeleteExtentProtection(ctx, client, logger, g, in.Space, in.Address); res != nil {
			return res, emptyOut, nil
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

func validateSubnetUpdateInput(in *SubnetUpdateInput) error {
	if err := ValidatePositiveInt32(in.SubnetID, "subnet_id"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Name, "name"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.ClassName, "class_name"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Space, "space"); err != nil {
		return err
	}
	if p := strings.TrimSpace(in.Prefix); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 128 {
			return fmt.Errorf("prefix %q must be an integer between 0 and 128", in.Prefix)
		}
	}
	if in.Name == "" && strings.TrimSpace(in.Prefix) == "" && in.ClassName == "" {
		return fmt.Errorf("no fields to update: set name, prefix, or class_name")
	}
	return nil
}

func buildIpamNetworkEditInput(in *SubnetUpdateInput) sdsclient.IpamNetworkEditInput {
	input := sdsclient.IpamNetworkEditInput{NetworkId: &in.SubnetID}
	if in.Name != "" {
		input.NetworkName = &in.Name
	}
	if p := strings.TrimSpace(in.Prefix); p != "" {
		input.NetworkPrefix = &p
	}
	if in.ClassName != "" {
		input.NetworkClassName = &in.ClassName
	}
	return input
}

// lookupSubnetExtentByID resolves a subnet (network) ID to its first and last
// address and its space. The update tool receives only the numeric id, so this
// is how a protected-subnet or protected-space rule is evaluated against the
// real object before the edit runs.
func lookupSubnetExtentByID(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, networkID int32) (start, end, space string, errResult *mcp.CallToolResult) {
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.IpamAPI.IpamNetworkInfo(authCtx).NetworkId(networkID).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_subnet_update", "error", apiErr)
		return "", "", "", apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return "", "", "", errorResult("subnet_id %d not found", networkID)
	}
	row := resp.Data[0]
	if row.NetworkStartHostaddr != nil {
		start = *row.NetworkStartHostaddr
	}
	if row.NetworkEndHostaddr != nil {
		end = *row.NetworkEndHostaddr
	}
	if row.SpaceName != nil {
		space = *row.SpaceName
	}
	return start, end, space, nil
}

// applySubnetUpdateProtections resolves the target subnet and refuses the edit
// if it touches a protected object. It checks the pre-edit extent (so a subnet
// that is or encloses a protected subnet cannot be modified) and, when the edit
// resizes, the post-edit CIDR (so a grow cannot newly swallow an adjacent
// protected subnet). It fails closed when a configured protection cannot be
// verified because the lookup returned no extent or space.
func applySubnetUpdateProtections(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, in *SubnetUpdateInput) *mcp.CallToolResult {
	start, end, space, errResult := lookupSubnetExtentByID(ctx, client, logger, in.SubnetID)
	if errResult != nil {
		return errResult
	}
	if len(g.ProtectedSubnets) > 0 && (start == "" || end == "") {
		return errorResult("cannot verify protected-subnet rules: subnet_id %d resolved no address extent", in.SubnetID)
	}
	if p, ok := g.overlappingProtectedSubnet(start, end); ok {
		return errorResult("cannot modify subnet %q-%q overlapping protected subnet %q", start, end, p)
	}
	if prefix := strings.TrimSpace(in.Prefix); prefix != "" && start != "" {
		// Build the resized CIDR through canonicalCIDR so a non-canonical ("08")
		// or family-invalid ("/128" on IPv4) prefix cannot make CheckProtectedSubnet
		// parse-fail and fall open. Validation already accepted the prefix as an
		// integer, so an unresolvable one here fails closed.
		cidr, ok := canonicalCIDR(start, prefix)
		if !ok {
			return errorResult("cannot verify resize: prefix %q is not valid for subnet %s", prefix, start)
		}
		if err := g.CheckProtectedSubnet(cidr); err != nil {
			return errorResult("%v", err)
		}
	}
	if len(g.ProtectedSpaces) > 0 && space == "" {
		return errorResult("cannot verify protected-space rules: subnet_id %d resolved no space", in.SubnetID)
	}
	if err := g.CheckProtectedSpace(space); err != nil {
		return errorResult("%v", err)
	}
	return nil
}

func subnetUpdateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SubnetUpdateInput) (*mcp.CallToolResult, SubnetUpdateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetUpdateInput) (*mcp.CallToolResult, SubnetUpdateOut, error) {
		emptyOut := SubnetUpdateOut{Data: make([]sdsclient.DataInnerIpamNetworkEditSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Space); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := validateSubnetUpdateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// The tool receives only the numeric subnet_id, so protected-subnet and
		// protected-space rules cannot be evaluated from the input alone. When a
		// relevant protection is configured, resolve the subnet's real extent and
		// space and re-check, so an edit cannot slip past a guardrail a create or
		// delete would hit.
		if guardrailsNeedAddressLookup(g) {
			if res := applySubnetUpdateProtections(ctx, client, logger, g, &in); res != nil {
				return res, emptyOut, nil
			}
		}

		logger.Info("updating subnet", "subnet_id", in.SubnetID)
		input := buildIpamNetworkEditInput(&in)

		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkEdit(authCtx).IpamNetworkEditInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_subnet_update", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamNetworkEditSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamNetworkEditSuccess, 0)
		}
		out := SubnetUpdateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// lookupSubnetExtents resolves every network starting at the given address (and
// space) to its first and last address, so a delete that names only the start
// address can be checked for enclosing a protected subnet. The delete API
// (IpamNetworkDelete by space+address) is not restricted to terminal subnets,
// so this lookup is not either: a non-terminal block network that shares the
// start address and encloses a protected subnet must be considered too. All
// matches are returned; the caller refuses if any overlaps a protected subnet
// (fail-closed over-refusal is safe here). A miss returns no extents and a nil
// error: nothing to protect and the delete itself reports the miss.
func lookupSubnetExtents(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, space, address string) ([]addrExtent, *mcp.CallToolResult) {
	startAddr, err := netip.ParseAddr(strings.TrimSpace(address))
	if err != nil {
		// Unparseable address is left to validation; nothing to resolve.
		return nil, nil
	}
	fixed := fmt.Sprintf("network_start_hostaddr='%s'", EscapeWhereValue(startAddr.String()))
	if space != "" {
		fixed = fmt.Sprintf("%s AND site_name='%s'", fixed, EscapeWhereValue(space))
	}
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.IpamAPI.IpamNetworkList(authCtx).Where(fixed).Limit(maxListLimit).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_subnet_delete", "error", apiErr)
		return nil, apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, nil
	}
	extents := make([]addrExtent, 0, len(resp.Data))
	for i := range resp.Data {
		row := resp.Data[i]
		var start, end string
		if row.NetworkStartHostaddr != nil {
			start = *row.NetworkStartHostaddr
		}
		if row.NetworkEndHostaddr != nil {
			end = *row.NetworkEndHostaddr
		}
		extents = append(extents, addrExtent{start: start, end: end})
	}
	return extents, nil
}

// applySubnetDeleteExtentProtection refuses a subnet delete when any network at
// the target address encloses or overlaps a protected subnet, closing the gap
// left by the bare-address check in the caller. It is a no-op (returns nil) when
// no protected subnets are configured or there is no client to resolve with, so
// the caller can invoke it unconditionally.
func applySubnetDeleteExtentProtection(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, space, address string) *mcp.CallToolResult {
	if client == nil || g == nil || len(g.ProtectedSubnets) == 0 {
		return nil
	}
	extents, errResult := lookupSubnetExtents(ctx, client, logger, space, address)
	if errResult != nil {
		return errResult
	}
	for _, e := range extents {
		// A resolved network with no usable extent cannot be checked; fail closed
		// rather than let an unverifiable delete through (matching subnet_update).
		if e.start == "" || e.end == "" {
			return errorResult("cannot verify protected-subnet rules: a subnet at %q resolved no address extent", address)
		}
		if p, ok := g.overlappingProtectedSubnet(e.start, e.end); ok {
			return errorResult("cannot delete subnet %q-%q overlapping protected subnet %q", e.start, e.end, p)
		}
	}
	return nil
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
