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

// Subnet Input Structs
type SubnetListInput struct {
	Space  string `json:"space" jsonschema:"The name of the space."`
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering (e.g., \"subnet_name LIKE 'lan%'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type SubnetInfoInput struct {
	ID int32 `json:"id" jsonschema:"The numeric ID of the subnet."`
}

type SpaceListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
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
		Description: "Returns the full detail for one subnet given its numeric ID, including size " +
			"and usage. Requires an ID you already hold, typically from solidserver_subnet_list; it " +
			"cannot look a subnet up by CIDR or name. Use solidserver_subnet_list instead to search " +
			"or to enumerate. Use solidserver_ip_list to see the addresses inside the subnet rather " +
			"than the subnet itself. Returns the subnet record as JSON.",
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
}

//nolint:dupl // similar list logic across modules
func subnetListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetListInput) (*mcp.CallToolResult, SubnetListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetListInput) (*mcp.CallToolResult, SubnetListOut, error) {
		emptyOut := SubnetListOut{Data: make([]sdsclient.DataInnerIpamNetworkData, 0)}
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

func subnetInfoHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetInfoInput) (*mcp.CallToolResult, SubnetInfoOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetInfoInput) (*mcp.CallToolResult, SubnetInfoOut, error) {
		emptyOut := SubnetInfoOut{Data: make([]sdsclient.DataInnerIpamNetworkData, 0)}

		if err := ValidatePositiveInt32(in.ID, "id"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("fetching subnet details", "subnet_id", in.ID)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkInfo(authCtx).NetworkId(in.ID)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_subnet_info", "error", err)
			return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
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

func subnetCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, SubnetCreateInput) (*mcp.CallToolResult, SubnetCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetCreateInput) (*mcp.CallToolResult, SubnetCreateOut, error) {
		emptyOut := SubnetCreateOut{Data: make([]sdsclient.DataInnerIpamNetworkAddSuccess, 0)}

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
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateSubnetPrefix(in.Address, in.Prefix); err != nil {
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
			return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
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
			return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
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
