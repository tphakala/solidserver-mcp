package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// IPAM Input Structs
type IPCreateInput struct {
	Space    string `json:"space" jsonschema:"The name of the space."`
	Subnet   string `json:"subnet" jsonschema:"The address of the subnet."`
	Hostaddr string `json:"hostaddr,omitempty" jsonschema:"Specific IP address to allocate (optional)."`
	Name     string `json:"name,omitempty" jsonschema:"The name to associate with the IP address."`
	Mac      string `json:"mac,omitempty" jsonschema:"The MAC address to associate with the IP address."`
}

type IPDeleteInput struct {
	IPAddress string `json:"ip_address" jsonschema:"The IP address to delete."`
	Space     string `json:"space" jsonschema:"The name of the space."`
}

type IPFindFreeInput struct {
	Space  string `json:"space" jsonschema:"The name of the space."`
	Subnet string `json:"subnet" jsonschema:"The address of the subnet."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of free IPs to return (default 10)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type IPListInput struct {
	Space  string `json:"space" jsonschema:"The name of the space."`
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering (e.g., \"address_name LIKE 'web%'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type IPUpdateInput struct {
	AddressID int32  `json:"address_id" jsonschema:"Numeric address ID from solidserver_ip_list."`
	Name      string `json:"name,omitempty" jsonschema:"New hostname to record against the address. Omit to leave unchanged."`
	Mac       string `json:"mac,omitempty" jsonschema:"New MAC address to associate with the address. Omit to leave unchanged."`
	Space     string `json:"space,omitempty" jsonschema:"IPAM space name (used for the protected-space guardrail)."`
}

// IPAM Output Structs
type IPCreateOut struct {
	Data []sdsclient.DataInnerIpamAddressAddSuccess `json:"data" jsonschema:"Allocated IP address records."`
}

type IPDeleteOut struct {
	Data []sdsclient.DataInnerIpamAddressDeleteSuccess `json:"data" jsonschema:"Deleted IP address response records."`
}

type IPUpdateOut struct {
	Data []sdsclient.DataInnerIpamAddressEditSuccess `json:"data" jsonschema:"Updated IP address response records."`
}

type IPFindFreeOut = ListOutput[sdsclient.DataInnerIpamAddressData]
type IPListOut = ListOutput[sdsclient.DataInnerIpamAddressData]

// RegisterIPAMTools registers IP management tools.
func RegisterIPAMTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_create",
		Title:       "Allocate an IP address",
		Annotations: additiveTool("Allocate an IP address"),
		Description: "Reserves an address in a subnet and records it against a hostname. Omit " +
			"hostaddr to take the next free address, or pass one to claim a specific address. This " +
			"is the tool that actually takes an address out of the pool: prefer " +
			"solidserver_ip_find_free when you only want to see what is available, since that " +
			"reserves nothing. Two callers racing for the next free address can be handed different " +
			"ones, so do not assume a prior find_free result is still free. Changes appliance state " +
			"and is released only by solidserver_ip_delete. Returns the allocated address as JSON.",
	}, ipCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_delete",
		Title:       "Release an IP address",
		Annotations: destructiveTool("Release an IP address"),
		Description: "Releases an allocated address back to its subnet's free pool and drops the " +
			"hostname associated with it. This is destructive and cannot be undone from this server; " +
			"the address can be re-allocated to a different host immediately afterwards. Confirm the " +
			"address is the one you mean with solidserver_ip_list first, since releasing an address " +
			"still configured on a live host invites a duplicate-address conflict later. Returns a " +
			"confirmation message.",
	}, ipDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_update",
		Title:       "Update an IP address",
		Annotations: additiveTool("Update an IP address"),
		Description: "Edits the hostname or MAC recorded against an existing allocation, identified by " +
			"its numeric address_id from solidserver_ip_list, without releasing and re-allocating the " +
			"address. Resolve the exact address_id first, since editing the wrong record renames a " +
			"different host. This changes only the IPAM record, not any live host configuration, so a " +
			"machine keeps its address until it is reconfigured. Use solidserver_ip_create instead to " +
			"allocate a new address rather than change one. Returns the updated address as JSON.",
	}, ipUpdateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_find_free",
		Title:       "Find the next available IP address",
		Annotations: readOnlyTool("Find the next available IP address"),
		Description: "Finds the first available unallocated address in a subnet without modifying " +
			"appliance state. Use this to inspect what is free before allocating with " +
			"solidserver_ip_create, or when deciding whether a subnet has enough capacity left for " +
			"new hosts. Use solidserver_ip_list instead when you need to see the addresses already " +
			"taken. Returns the free address as JSON.",
	}, ipFindFreeHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_list",
		Title:       "List IP addresses",
		Annotations: readOnlyTool("List IP addresses"),
		Description: "Enumerates IP address allocations, optionally scoped to a space or narrowed " +
			"by a where clause. Use this to find which host holds a given address, to audit " +
			"allocations in a subnet, or to verify an address exists before calling " +
			"solidserver_ip_delete. Use solidserver_ip_find_free instead to look for unallocated " +
			"addresses. Returns the matching address records as JSON.",
	}, ipListHandler(client, logger))
}

func checkIPCreateGuardrails(g *Guardrails, space, subnet, hostaddr string) error {
	if err := g.CheckReadOnly(); err != nil {
		return err
	}
	if err := g.CheckProtectedSpace(space); err != nil {
		return err
	}
	if hostaddr != "" {
		if err := g.CheckProtectedSubnet(hostaddr); err != nil {
			return err
		}
	}
	return g.CheckProtectedSubnet(subnet)
}

func buildIPAddressAddInput(ctx context.Context, client *services.APIClientWrapper, in *IPCreateInput, logger *slog.Logger) (sdsclient.IpamAddressAddInput, error) {
	input := sdsclient.IpamAddressAddInput{
		SpaceName: &in.Space,
	}
	if in.Hostaddr != "" {
		input.AddressHostaddr = &in.Hostaddr
	} else {
		freeIP, err := findFirstFreeIP(ctx, client, in.Space, in.Subnet, logger)
		if err != nil {
			return input, err
		}
		input.AddressHostaddr = freeIP
	}
	if in.Name != "" {
		input.AddressName = &in.Name
	}
	if in.Mac != "" {
		input.AddressMacAddr = &in.Mac
	}
	return input, nil
}

func validateIPCreateInput(in *IPCreateInput) error {
	if err := ValidateRequiredString(in.Space, "space"); err != nil {
		return err
	}
	if err := ValidateIP(in.Subnet, "subnet"); err != nil {
		return err
	}
	if in.Hostaddr != "" {
		if err := ValidateIP(in.Hostaddr, "hostaddr"); err != nil {
			return err
		}
	}
	if in.Mac != "" {
		if err := ValidateMAC(in.Mac, "mac"); err != nil {
			return err
		}
	}
	if in.Name != "" {
		if err := ValidateOptionalString(in.Name, "name"); err != nil {
			return err
		}
	}
	return nil
}

func findFirstFreeIP(ctx context.Context, client *services.APIClientWrapper, space, subnet string, logger *slog.Logger) (*string, error) {
	authCtx := client.AuthContext(ctx)
	where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'",
		EscapeWhereValue(subnet), EscapeWhereValue(space))
	logger.Debug("searching for free IP", "subnet", subnet, "space", space)
	listReq := client.IpamAPI.IpamAddressList(authCtx).Where(where).Limit(1)
	listResp, httpResp, apiErr := listReq.Execute()
	closeBody(httpResp)
	if apiErr != nil {
		// This error is surfaced to the model via an unfenced errorResult, so the
		// appliance-derived portion is fenced here at its source.
		return nil, fmt.Errorf("finding free IP in subnet %s: %s", subnet, fenceUntrusted(formatAPIError(apiErr, httpResp)))
	}
	if listResp == nil || len(listResp.Data) == 0 {
		return nil, fmt.Errorf("no free IP found in subnet: %s", subnet)
	}

	firstFree := listResp.Data[0].AddressHostaddr
	if firstFree == nil {
		return nil, fmt.Errorf("found IP entry in subnet %s but it has no address", subnet)
	}
	logger.Debug("found free IP", "ip", *firstFree)
	return firstFree, nil
}

func ipCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, IPCreateInput) (*mcp.CallToolResult, IPCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPCreateInput) (*mcp.CallToolResult, IPCreateOut, error) {
		emptyOut := IPCreateOut{Data: make([]sdsclient.DataInnerIpamAddressAddSuccess, 0)}

		if err := checkIPCreateGuardrails(g, in.Space, in.Subnet, in.Hostaddr); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := validateIPCreateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		input, err := buildIPAddressAddInput(ctx, client, &in, logger)
		if err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if input.AddressHostaddr != nil {
			if err := g.CheckProtectedSubnet(*input.AddressHostaddr); err != nil {
				return errorResult("%v", err), emptyOut, nil
			}
		}

		logger.Info("creating IP address", "ip", *input.AddressHostaddr, "space", in.Space)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamAddressAdd(authCtx).IpamAddressAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_ip_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamAddressAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamAddressAddSuccess, 0)
		}
		out := IPCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func ipDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, IPDeleteInput) (*mcp.CallToolResult, IPDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPDeleteInput) (*mcp.CallToolResult, IPDeleteOut, error) {
		emptyOut := IPDeleteOut{Data: make([]sdsclient.DataInnerIpamAddressDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Space); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSubnet(in.IPAddress); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIP(in.IPAddress, "ip_address"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting IP address", "ip", in.IPAddress, "space", in.Space)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamAddressDelete(authCtx).
			AddressHostaddr(in.IPAddress).
			SpaceName(in.Space)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_ip_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamAddressDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamAddressDeleteSuccess, 0)
		}
		out := IPDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

// guardrailsNeedAddressLookup reports whether any configured protection needs
// the address's real subnet or space, which the ip_update input does not carry.
func guardrailsNeedAddressLookup(g *Guardrails) bool {
	return g != nil && (len(g.ProtectedSubnets) > 0 || len(g.ProtectedSpaces) > 0)
}

// lookupAddressLocation resolves an address ID to its hostaddr and space so the
// guardrails can be checked before an edit. It returns a ready-to-return error
// result when the lookup fails or the address does not exist, so ip_update fails
// closed rather than editing an address whose protection could not be verified.
func lookupAddressLocation(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, addressID int32) (hostaddr, space string, errResult *mcp.CallToolResult) {
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.IpamAPI.IpamAddressInfo(authCtx).AddressId(addressID).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_ip_update", "error", apiErr)
		return "", "", apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return "", "", errorResult("address_id %d not found", addressID)
	}
	row := resp.Data[0]
	if row.AddressHostaddr != nil {
		hostaddr = *row.AddressHostaddr
	}
	if row.SpaceName != nil {
		space = *row.SpaceName
	}
	return hostaddr, space, nil
}

func validateIPUpdateInput(in *IPUpdateInput) error {
	if err := ValidatePositiveInt32(in.AddressID, "address_id"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Name, "name"); err != nil {
		return err
	}
	if err := ValidateMAC(in.Mac, "mac"); err != nil {
		return err
	}
	if in.Name == "" && in.Mac == "" {
		return fmt.Errorf("no fields to update: set name, mac, or both")
	}
	return nil
}

// applyIPUpdateProtections enforces the subnet and space guardrails against the
// address's real location, which the ip_update input does not carry. It returns
// a ready-to-return error result on refusal or lookup failure, or nil to allow
// the edit. It fails CLOSED: a lookup that resolves no usable hostaddr (or no
// space, when space protections are configured) is refused rather than checked
// against an empty string, which the guardrails treat as "no match".
func applyIPUpdateProtections(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, addressID int32) *mcp.CallToolResult {
	hostaddr, space, errResult := lookupAddressLocation(ctx, client, logger, addressID)
	if errResult != nil {
		return errResult
	}
	if len(g.ProtectedSubnets) > 0 && hostaddr == "" {
		return errorResult("cannot verify protected-subnet rules: address_id %d resolved no hostaddr", addressID)
	}
	if err := g.CheckProtectedSubnet(hostaddr); err != nil {
		return errorResult("%v", err)
	}
	if len(g.ProtectedSpaces) > 0 && space == "" {
		return errorResult("cannot verify protected-space rules: address_id %d resolved no space", addressID)
	}
	if err := g.CheckProtectedSpace(space); err != nil {
		return errorResult("%v", err)
	}
	return nil
}

// buildIPAddressEditInput builds the edit payload. Space is deliberately NOT
// forwarded to the appliance: the tool updates name/MAC metadata only, and
// sending space_name on an edit could reassign the address to a different space.
// The space input is used solely for the protected-space guardrail.
func buildIPAddressEditInput(in *IPUpdateInput) sdsclient.IpamAddressEditInput {
	input := sdsclient.IpamAddressEditInput{AddressId: &in.AddressID}
	if in.Name != "" {
		input.AddressName = &in.Name
	}
	if in.Mac != "" {
		input.AddressMacAddr = &in.Mac
	}
	return input
}

func ipUpdateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, IPUpdateInput) (*mcp.CallToolResult, IPUpdateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPUpdateInput) (*mcp.CallToolResult, IPUpdateOut, error) {
		emptyOut := IPUpdateOut{Data: make([]sdsclient.DataInnerIpamAddressEditSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedSpace(in.Space); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := validateIPUpdateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// The tool takes only the numeric address_id, so a protected-subnet or
		// protected-space rule cannot be checked against the input alone. When
		// either protection is configured, resolve the address and re-check so an
		// edit cannot slip past a guardrail a create or delete would hit.
		if guardrailsNeedAddressLookup(g) {
			if res := applyIPUpdateProtections(ctx, client, logger, g, in.AddressID); res != nil {
				return res, emptyOut, nil
			}
		}

		logger.Info("updating IP address", "address_id", in.AddressID, "space", in.Space)
		input := buildIPAddressEditInput(&in)

		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamAddressEdit(authCtx).IpamAddressEditInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_ip_update", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerIpamAddressEditSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerIpamAddressEditSuccess, 0)
		}
		out := IPUpdateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func ipFindFreeHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPFindFreeInput) (*mcp.CallToolResult, IPFindFreeOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPFindFreeInput) (*mcp.CallToolResult, IPFindFreeOut, error) {
		emptyOut := IPFindFreeOut{Data: make([]sdsclient.DataInnerIpamAddressData, 0)}
		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateIP(in.Subnet, "subnet"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		opts := ListOptions{Limit: limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_ip_find_free",
			func(c context.Context, _ string, limit, offset int32) ([]sdsclient.DataInnerIpamAddressData, *http.Response, error) {
				where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'",
					EscapeWhereValue(in.Subnet), EscapeWhereValue(in.Space))
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(where).
					Limit(limit).
					Offset(offset)
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

//nolint:dupl // similar list logic across modules
func ipListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPListInput) (*mcp.CallToolResult, IPListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPListInput) (*mcp.CallToolResult, IPListOut, error) {
		emptyOut := IPListOut{Data: make([]sdsclient.DataInnerIpamAddressData, 0), Limit: clampLimit(in.Limit), Offset: in.Offset}
		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_ip_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerIpamAddressData, *http.Response, error) {
				w := CombineWhereClause(fmt.Sprintf("space_name='%s'", EscapeWhereValue(in.Space)), where)
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(w).
					Limit(limit).
					Offset(offset)
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
