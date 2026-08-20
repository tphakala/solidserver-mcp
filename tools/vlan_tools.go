package tools

import (
	"context"
	"fmt"
	"log/slog"

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
func RegisterVlanTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_domain_list",
		Title:       "List VLAN domains",
		Annotations: readOnlyTool("List VLAN domains"),
		Description: "Enumerates the VLAN domains defined on the appliance. A VLAN domain is the " +
			"namespace that VLAN IDs are unique within, so start here when you do not already know " +
			"which domain to work in: both solidserver_vlan_create and solidserver_vlan_delete " +
			"require a domain name, and a VLAN ID is ambiguous without one. Use " +
			"solidserver_vlan_list instead to see the VLANs inside a domain. Returns the full " +
			"domain records as JSON.",
	}, vlanDomainListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_list",
		Title:       "List VLANs",
		Annotations: readOnlyTool("List VLANs"),
		Description: "Enumerates individual VLANs, optionally restricted to one domain or narrowed " +
			"by a where clause. Use this to check whether a VLAN ID is already taken before calling " +
			"solidserver_vlan_create, or to find the exact VLAN name that solidserver_vlan_delete " +
			"needs. Use solidserver_vlan_domain_list instead when you need the domains themselves " +
			"rather than their contents. Omitting the domain searches across all domains, which can " +
			"return the same VLAN ID more than once. Returns the matching VLAN records as JSON.",
	}, vlanListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_create",
		Title:       "Create a VLAN",
		Annotations: additiveTool("Create a VLAN"),
		Description: "Creates a VLAN with the given numeric ID and name inside an existing domain. " +
			"The domain must already exist; this tool does not create one. Check the ID is free with " +
			"solidserver_vlan_list first, because re-running this with an ID that is already in use " +
			"fails rather than updating the existing VLAN. Changes appliance state and is undone " +
			"only by solidserver_vlan_delete. Returns the created VLAN record as JSON.",
	}, vlanCreateHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_delete",
		Title:       "Delete a VLAN",
		Annotations: destructiveTool("Delete a VLAN"),
		Description: "Permanently removes a VLAN from a domain, identified by domain and VLAN name. " +
			"This is destructive and cannot be undone from this server; the VLAN can only be " +
			"recreated with solidserver_vlan_create, which will not restore anything that referenced " +
			"it. Confirm the exact name with solidserver_vlan_list first, since deleting the wrong " +
			"VLAN can black-hole traffic on the segment. Returns a confirmation message.",
	}, vlanDeleteHandler(client, logger))
}

func vlanDomainListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanDomainListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDomainListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_vlan_domain_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.VlanAPI.VlanDomainList(authCtx).Limit(limit).Offset(offset)
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

func vlanListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_vlan_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				w := ""
				if in.Domain != "" {
					w = fmt.Sprintf("domain_name='%s'", EscapeWhereValue(in.Domain))
				}
				if where != "" {
					if w != "" {
						w = fmt.Sprintf("(%s) AND (%s)", w, where)
					} else {
						w = where
					}
				}

				authCtx := client.AuthContext(c)
				req := client.VlanAPI.VlanVlanList(authCtx).Limit(limit).Offset(offset)
				if w != "" {
					req = req.Where(w)
				}
				resp, _, apiErr := req.Execute()
				if apiErr != nil {
					return nil, apiErr
				}
				return resp, nil
			})
	}
}

func vlanCreateHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanCreateInput) (*mcp.CallToolResult, any, error) {
		if err := ValidateRequiredString(in.Domain, "domain"); err != nil {
			return validationErrorResult(err)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err)
		}
		if err := ValidateVlanID(in.VlanID); err != nil {
			return validationErrorResult(err)
		}

		logger.Info("creating VLAN", "name", in.Name, "vlan_id", in.VlanID, "domain", in.Domain)
		input := sdsclient.VlanVlanAddInput{
			DomainName: &in.Domain,
			VlanName:   &in.Name,
			VlanVlanId: &in.VlanID,
		}

		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanVlanAdd(authCtx).VlanVlanAddInput(input)
		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func vlanDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDeleteInput) (*mcp.CallToolResult, any, error) {
		if err := ValidateRequiredString(in.Domain, "domain"); err != nil {
			return validationErrorResult(err)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err)
		}

		logger.Info("deleting VLAN", "name", in.Name, "domain", in.Domain)
		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanVlanDelete(authCtx).
			DomainName(in.Domain).
			VlanName(in.Name)

		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}
