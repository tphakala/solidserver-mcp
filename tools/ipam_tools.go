package tools

import (
	"context"
	"fmt"

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
func RegisterIPAMTools(s *mcp.Server, client *services.APIClientWrapper) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_create",
		Description: "Requests a new IP address allocation from a specified subnet. If hostaddr is omitted, it allocates the next free IP.",
	}, ipCreateHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_delete",
		Description: "Releases/deletes a specified IP address.",
	}, ipDeleteHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_find_free",
		Description: "Returns a list of available free IP addresses in a subnet without allocating them.",
	}, ipFindFreeHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_ip_list",
		Description: "Lists IP addresses in a space/subnet with optional filtering.",
	}, ipListHandler(client))
}

func ipCreateHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, IPCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPCreateInput) (*mcp.CallToolResult, any, error) {
		input := sdsclient.IpamAddressAddInput{
			SpaceName: &in.Space,
		}

		authCtx := client.AuthContext(ctx)

		if in.Hostaddr != "" {
			input.AddressHostaddr = &in.Hostaddr
		} else {
			// Find a free IP from the specified subnet
			where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'", in.Subnet, in.Space)
			listReq := client.IpamAPI.IpamAddressList(authCtx).Where(where).Limit(1)
			listResp, _, apiErr := listReq.Execute()
			if apiErr.Error() != "" {
				r, a := errorResult("failed to find free IP in subnet %s: %v", in.Subnet, apiErr.Error())
				return r, a, nil
			}

			if len(listResp.Data) == 0 {
				r, a := errorResult("no free IP found in subnet: %s", in.Subnet)
				return r, a, nil
			}

			// Use the first available IP
			firstFreeIP := listResp.Data[0].AddressHostaddr
			input.AddressHostaddr = firstFreeIP
		}

		if in.Name != "" {
			input.AddressName = &in.Name
		}
		if in.Mac != "" {
			input.AddressMacAddr = &in.Mac
		}

		req := client.IpamAPI.IpamAddressAdd(authCtx).IpamAddressAddInput(input)
		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func ipDeleteHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, IPDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPDeleteInput) (*mcp.CallToolResult, any, error) {
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamAddressDelete(authCtx).
			AddressHostaddr(in.IPAddress).
			SpaceName(in.Space)

		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func ipFindFreeHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, IPFindFreeInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPFindFreeInput) (*mcp.CallToolResult, any, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Limit: limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, _ string, limit, offset int32) (any, error) {
				where := fmt.Sprintf("parent_subnet_addr='%s' AND is_free='1' AND space_name='%s'", in.Subnet, in.Space)
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(where).
					Limit(limit).
					Offset(offset)
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

//nolint:dupl // similar list logic across modules
func ipListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, IPListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in IPListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, where string, limit, offset int32) (any, error) {
				w := fmt.Sprintf("space_name='%s'", in.Space)
				if where != "" {
					w = fmt.Sprintf("(%s) AND (%s)", w, where)
				}
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamAddressList(authCtx).
					Where(w).
					Limit(limit).
					Offset(offset)
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}
