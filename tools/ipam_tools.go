package tools

import (
	"context"
	"fmt"
	"log/slog"

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

// RegisterIPAMTools registers IP management tools.
func RegisterIPAMTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
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
	}, ipCreateHandler(client, logger))

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
	}, ipDeleteHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_find_free",
		Title:       "Find free IP addresses",
		Annotations: readOnlyTool("Find free IP addresses"),
		Description: "Returns addresses currently free in a subnet without reserving any of them. " +
			"Use this to plan an allocation or to report available capacity; use " +
			"solidserver_ip_create when you actually want to claim one. The result is a snapshot " +
			"with no hold on it, so an address listed here can be taken by someone else before you " +
			"allocate it. Use solidserver_ip_list instead to see addresses already in use. Returns " +
			"the free addresses as JSON.",
	}, ipFindFreeHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_list",
		Title:       "List allocated IP addresses",
		Annotations: readOnlyTool("List allocated IP addresses"),
		Description: "Enumerates addresses already recorded in a space or subnet, optionally " +
			"narrowed by a where clause. Use this to find which host holds an address, to confirm an " +
			"allocation before calling solidserver_ip_delete, or to audit usage. Use " +
			"solidserver_ip_find_free instead to see what is still available: this tool reports what " +
			"is taken, not what is free. Returns the matching address records as JSON.",
	}, ipListHandler(client, logger))
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
	return nil
}

func findFirstFreeIP(ctx context.Context, client *services.APIClientWrapper, space, subnet string, logger *slog.Logger) (freeIP *string, res *mcp.CallToolResult, anyVal any) {
	where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'",
		EscapeWhereValue(subnet), EscapeWhereValue(space))
	logger.Debug("searching for free IP", "subnet", subnet, "space", space)
	authCtx := client.AuthContext(ctx)
	listReq := client.IpamAPI.IpamAddressList(authCtx).Where(where).Limit(1)
	listResp, _, apiErr := listReq.Execute()
	if apiErr != nil {
		r, a := errorResult("failed to find free IP in subnet %s: %v", subnet, apiErr)
		return nil, r, a
	}

	if len(listResp.Data) == 0 {
		r, a := errorResult("no free IP found in subnet: %s", subnet)
		return nil, r, a
	}

	firstFree := listResp.Data[0].AddressHostaddr
	if firstFree == nil {
		r, a := errorResult("found IP entry in subnet %s but it has no address", subnet)
		return nil, r, a
	}
	logger.Debug("found free IP", "ip", *firstFree)
	return firstFree, nil, nil
}

func ipCreateHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPCreateInput) (*mcp.CallToolResult, any, error) {
		if err := validateIPCreateInput(&in); err != nil {
			return validationErrorResult(err)
		}

		input := sdsclient.IpamAddressAddInput{
			SpaceName: &in.Space,
		}

		authCtx := client.AuthContext(ctx)

		if in.Hostaddr != "" {
			input.AddressHostaddr = &in.Hostaddr
		} else {
			freeIP, res, anyVal := findFirstFreeIP(ctx, client, in.Space, in.Subnet, logger)
			if res != nil {
				return res, anyVal, nil
			}
			input.AddressHostaddr = freeIP
		}

		if in.Name != "" {
			input.AddressName = &in.Name
		}
		if in.Mac != "" {
			input.AddressMacAddr = &in.Mac
		}

		logger.Info("creating IP address", "ip", *input.AddressHostaddr, "space", in.Space)
		req := client.IpamAPI.IpamAddressAdd(authCtx).IpamAddressAddInput(input)
		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func ipDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPDeleteInput) (*mcp.CallToolResult, any, error) {
		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err)
		}
		if err := ValidateIP(in.IPAddress, "ip_address"); err != nil {
			return validationErrorResult(err)
		}

		logger.Info("deleting IP address", "ip", in.IPAddress, "space", in.Space)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamAddressDelete(authCtx).
			AddressHostaddr(in.IPAddress).
			SpaceName(in.Space)

		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func ipFindFreeHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPFindFreeInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPFindFreeInput) (*mcp.CallToolResult, any, error) {
		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err)
		}
		if err := ValidateIP(in.Subnet, "subnet"); err != nil {
			return validationErrorResult(err)
		}

		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Limit: limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_ip_find_free",
			func(c context.Context, _ string, limit, offset int32) (any, error) {
				where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'",
					EscapeWhereValue(in.Subnet), EscapeWhereValue(in.Space))
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(where).
					Limit(limit).
					Offset(offset)
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

//nolint:dupl // similar list logic across modules
func ipListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, IPListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPListInput) (*mcp.CallToolResult, any, error) {
		if err := ValidateRequiredString(in.Space, "space"); err != nil {
			return validationErrorResult(err)
		}

		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_ip_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				w := fmt.Sprintf("space_name='%s'", EscapeWhereValue(in.Space))
				if where != "" {
					w = fmt.Sprintf("(%s) AND (%s)", w, where)
				}
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(w).
					Limit(limit).
					Offset(offset)
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}
