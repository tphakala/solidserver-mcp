package tools

import (
	"context"
	"fmt"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// VLAN Input Structs
type VlanDomainListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type VlanListInput struct {
	Domain string `json:"domain,omitempty" jsonschema:"The name of the VLAN domain."`
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering (e.g., \"vlan_name LIKE 'guest%'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type VlanCreateInput struct {
	Domain string `json:"domain" jsonschema:"The name of the VLAN domain."`
	VlanID int32  `json:"vlan_id" jsonschema:"The numeric VLAN ID."`
	Name   string `json:"name" jsonschema:"The name of the VLAN."`
}

type VlanDeleteInput struct {
	Domain string `json:"domain" jsonschema:"The name of the VLAN domain."`
	Name   string `json:"name" jsonschema:"The name of the VLAN to delete."`
}

// RegisterVlanTools registers VLAN management tools.
func RegisterVlanTools(s *mcp.Server, client *services.APIClientWrapper) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_domain_list",
		Description: "Lists VLAN domains.",
	}, vlanDomainListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_list",
		Description: "Lists VLANs with optional filtering by domain or query.",
	}, vlanListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_create",
		Description: "Creates a new VLAN within a specified domain.",
	}, vlanCreateHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_delete",
		Description: "Deletes a specific VLAN from a domain.",
	}, vlanDeleteHandler(client))
}

func vlanDomainListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, VlanDomainListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDomainListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.VlanApi.VlanDomainList(authCtx).Limit(limit).Offset(offset)
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

func vlanListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, VlanListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, where string, limit, offset int32) (any, error) {
				w := ""
				if in.Domain != "" {
					w = fmt.Sprintf("domain_name='%s'", in.Domain)
				}
				if where != "" {
					if w != "" {
						w = fmt.Sprintf("(%s) AND (%s)", w, where)
					} else {
						w = where
					}
				}

				authCtx := client.AuthContext(c)
				req := client.VlanApi.VlanVlanList(authCtx).Limit(limit).Offset(offset)
				if w != "" {
					req = req.Where(w)
				}
				resp, _, apiErr := req.Execute()
				if apiErr.Error() != "" {
					return nil, fmt.Errorf("%s", apiErr.Error())
				}
				return resp, nil
			})
	}
}

func vlanCreateHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, VlanCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanCreateInput) (*mcp.CallToolResult, any, error) {
		input := sdsclient.VlanVlanAddInput{
			DomainName: &in.Domain,
			VlanName:   &in.Name,
			VlanVlanId: &in.VlanID,
		}

		authCtx := client.AuthContext(ctx)
		req := client.VlanApi.VlanVlanAdd(authCtx).VlanVlanAddInput(input)
		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func vlanDeleteHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, VlanDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDeleteInput) (*mcp.CallToolResult, any, error) {
		authCtx := client.AuthContext(ctx)
		req := client.VlanApi.VlanVlanDelete(authCtx).
			DomainName(in.Domain).
			VlanName(in.Name)

		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}
