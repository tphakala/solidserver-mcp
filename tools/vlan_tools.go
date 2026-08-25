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

type VlanDomainCreateInput struct {
	Name        string `json:"name" jsonschema:"The name of the VLAN domain to create."`
	Description string `json:"description,omitempty" jsonschema:"An optional human-readable description for the domain."`
}

type VlanDomainDeleteInput struct {
	Name string `json:"name" jsonschema:"The name of the VLAN domain to delete."`
}

// VLAN Output Structs
type VlanDomainListOut = ListOutput[sdsclient.DataInnerVlanDomainData]
type VlanListOut = ListOutput[sdsclient.DataInnerVlanVlanData]

type VlanCreateOut struct {
	Data []sdsclient.DataInnerVlanVlanAddSuccess `json:"data" jsonschema:"Created VLAN response records."`
}

type VlanDeleteOut struct {
	Data []sdsclient.DataInnerVlanVlanDeleteSuccess `json:"data" jsonschema:"Deleted VLAN response records."`
}

type VlanDomainCreateOut struct {
	Data []sdsclient.DataInnerVlanDomainAddSuccess `json:"data" jsonschema:"Created VLAN domain response records."`
}

type VlanDomainDeleteOut struct {
	Data []sdsclient.DataInnerVlanDomainDeleteSuccess `json:"data" jsonschema:"Deleted VLAN domain response records."`
}

// RegisterVlanTools registers VLAN management tools.
func RegisterVlanTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
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
	}, vlanCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_delete",
		Title:       "Delete a VLAN",
		Annotations: destructiveTool("Delete a VLAN"),
		Description: "Permanently removes a VLAN from a domain, identified by domain and VLAN name. " +
			"This is destructive and cannot be undone from this server; the VLAN can only be " +
			"recreated with solidserver_vlan_create, which will not restore anything that referenced " +
			"it. Confirm the exact name with solidserver_vlan_list first, since deleting the wrong " +
			"VLAN can black-hole traffic on the segment. Returns a confirmation message.",
	}, vlanDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_domain_create",
		Title:       "Create a VLAN domain",
		Annotations: additiveTool("Create a VLAN domain"),
		Description: "Creates a new VLAN domain, the namespace that VLAN IDs are unique within. Check " +
			"whether the name is already taken with solidserver_vlan_domain_list first, since a " +
			"duplicate is rejected rather than merged and every VLAN tool selects its domain by name. " +
			"A new domain starts empty; add VLANs to it with solidserver_vlan_create afterwards. " +
			"Changes appliance state and is undone only by solidserver_vlan_domain_delete. Returns the " +
			"created domain as JSON.",
	}, vlanDomainCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_vlan_domain_delete",
		Title:       "Delete a VLAN domain",
		Annotations: destructiveTool("Delete a VLAN domain"),
		Description: "Permanently removes a VLAN domain. This is destructive and cannot be undone from " +
			"this server; deleting a domain takes every VLAN defined inside it with it, so audit it " +
			"with solidserver_vlan_list first. Recreating the domain with " +
			"solidserver_vlan_domain_create restores none of the VLANs that were in it. Returns a " +
			"confirmation message.",
	}, vlanDomainDeleteHandler(client, logger, g))
}

func vlanDomainCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, VlanDomainCreateInput) (*mcp.CallToolResult, VlanDomainCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDomainCreateInput) (*mcp.CallToolResult, VlanDomainCreateOut, error) {
		emptyOut := VlanDomainCreateOut{Data: make([]sdsclient.DataInnerVlanDomainAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.Description, "description"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating VLAN domain", "name", in.Name)
		input := sdsclient.VlanDomainAddInput{DomainName: &in.Name}
		if in.Description != "" {
			input.DomainDescription = &in.Description
		}

		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanDomainAdd(authCtx).VlanDomainAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_vlan_domain_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerVlanDomainAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerVlanDomainAddSuccess, 0)
		}
		out := VlanDomainCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func vlanDomainDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, VlanDomainDeleteInput) (*mcp.CallToolResult, VlanDomainDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDomainDeleteInput) (*mcp.CallToolResult, VlanDomainDeleteOut, error) {
		emptyOut := VlanDomainDeleteOut{Data: make([]sdsclient.DataInnerVlanDomainDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting VLAN domain", "name", in.Name)
		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanDomainDelete(authCtx).DomainName(in.Name)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_vlan_domain_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerVlanDomainDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerVlanDomainDeleteSuccess, 0)
		}
		out := VlanDomainDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func vlanDomainListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanDomainListInput) (*mcp.CallToolResult, VlanDomainListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDomainListInput) (*mcp.CallToolResult, VlanDomainListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_vlan_domain_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerVlanDomainData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.VlanAPI.VlanDomainList(authCtx).Limit(limit).Offset(offset)
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

func vlanListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, VlanListInput) (*mcp.CallToolResult, VlanListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanListInput) (*mcp.CallToolResult, VlanListOut, error) {
		emptyOut := VlanListOut{Data: make([]sdsclient.DataInnerVlanVlanData, 0), Limit: clampLimit(in.Limit), Offset: in.Offset}
		if err := ValidateOptionalString(in.Domain, "domain"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_vlan_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerVlanVlanData, *http.Response, error) {
				fixed := ""
				if in.Domain != "" {
					fixed = fmt.Sprintf("domain_name='%s'", EscapeWhereValue(in.Domain))
				}
				w := CombineWhereClause(fixed, where)
				authCtx := client.AuthContext(c)
				req := client.VlanAPI.VlanVlanList(authCtx).Limit(limit).Offset(offset)
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

func vlanCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, VlanCreateInput) (*mcp.CallToolResult, VlanCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanCreateInput) (*mcp.CallToolResult, VlanCreateOut, error) {
		emptyOut := VlanCreateOut{Data: make([]sdsclient.DataInnerVlanVlanAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Domain, "domain"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateVlanID(in.VlanID); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating VLAN", "name", in.Name, "vlan_id", in.VlanID, "domain", in.Domain)
		input := sdsclient.VlanVlanAddInput{
			DomainName: &in.Domain,
			VlanName:   &in.Name,
			VlanVlanId: &in.VlanID,
		}

		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanVlanAdd(authCtx).VlanVlanAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_vlan_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerVlanVlanAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerVlanVlanAddSuccess, 0)
		}
		out := VlanCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func vlanDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, VlanDeleteInput) (*mcp.CallToolResult, VlanDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in VlanDeleteInput) (*mcp.CallToolResult, VlanDeleteOut, error) {
		emptyOut := VlanDeleteOut{Data: make([]sdsclient.DataInnerVlanVlanDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateRequiredString(in.Domain, "domain"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting VLAN", "name", in.Name, "domain", in.Domain)
		authCtx := client.AuthContext(ctx)
		req := client.VlanAPI.VlanVlanDelete(authCtx).
			DomainName(in.Domain).
			VlanName(in.Name)

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_vlan_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerVlanVlanDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerVlanVlanDeleteSuccess, 0)
		}
		out := VlanDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}
