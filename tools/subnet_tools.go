package tools

import (
	"context"
	"fmt"
	"log/slog"

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

// RegisterSubnetTools registers subnet and space management tools.
func RegisterSubnetTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_list",
		Description: "Lists subnets in a space with optional filtering.",
	}, subnetListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_info",
		Description: "Returns detailed information for a specific subnet by ID.",
	}, subnetInfoHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_create",
		Description: "Creates a new subnet within a space.",
	}, subnetCreateHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_subnet_delete",
		Description: "Deletes a specific subnet from a space.",
	}, subnetDeleteHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_space_list",
		Description: "Lists IPAM spaces.",
	}, spaceListHandler(client, logger))
}

//nolint:dupl // similar list logic across modules
func subnetListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_subnet_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				w := fmt.Sprintf("site_name='%s'", in.Space)
				if where != "" {
					w = fmt.Sprintf("(%s) AND (%s)", w, where)
				}
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamNetworkList(authCtx).
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

func subnetInfoHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetInfoInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetInfoInput) (*mcp.CallToolResult, any, error) {
		logger.Debug("getting subnet info", "id", in.ID)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkInfo(authCtx).NetworkId(in.ID)
		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func subnetCreateHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetCreateInput) (*mcp.CallToolResult, any, error) {
		logger.Info("creating subnet", "name", in.Name, "address", in.Address, "prefix", in.Prefix, "space", in.Space)
		input := sdsclient.IpamNetworkAddInput{
			SpaceName:     &in.Space,
			NetworkAddr:   &in.Address,
			NetworkPrefix: &in.Prefix,
			NetworkName:   &in.Name,
		}

		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkAdd(authCtx).IpamNetworkAddInput(input)
		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func subnetDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SubnetDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SubnetDeleteInput) (*mcp.CallToolResult, any, error) {
		logger.Info("deleting subnet", "address", in.Address, "space", in.Space)
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamNetworkDelete(authCtx).
			SpaceName(in.Space).
			NetworkAddr(in.Address)

		resp, _, err := req.Execute()
		if err != nil {
			r, a := errorResult("SolidServer API error: %v", err)
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

//nolint:dupl // similar list logic across modules
func spaceListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, SpaceListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in SpaceListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts, logger, "solidserver_space_list",
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.IpamAPI.IpamSpaceList(authCtx).Limit(limit).Offset(offset)
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
