package tools

import (
	"context"
	"fmt"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// DNS Input Structs
type DNSRecordCreateInput struct {
	Zone   string `json:"zone" jsonschema:"The name of the DNS zone."`
	Name   string `json:"name" jsonschema:"The name of the record (relative to zone)."`
	Type   string `json:"type" jsonschema:"The type of record (e.g., 'A', 'AAAA', 'CNAME')."`
	Value  string `json:"value" jsonschema:"The value of the record (e.g., IP address or target FQDN)."`
	TTL    int32  `json:"ttl,omitempty" jsonschema:"Time to live (seconds, default 3600)."`
	Server string `json:"server,omitempty" jsonschema:"The DNS server name (optional)."`
	View   string `json:"view,omitempty" jsonschema:"The DNS view name (optional)."`
}

type DNSRecordDeleteInput struct {
	Zone   string `json:"zone" jsonschema:"The name of the DNS zone."`
	Name   string `json:"name" jsonschema:"The name of the record."`
	Type   string `json:"type" jsonschema:"The type of record."`
	Server string `json:"server,omitempty" jsonschema:"The DNS server name (optional)."`
	View   string `json:"view,omitempty" jsonschema:"The DNS view name (optional)."`
}

type DNSRecordListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering (e.g., \"rr_name LIKE 'web%'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DNSZoneListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like where clause for filtering."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

// RegisterDNSTools registers DNS management tools.
func RegisterDNSTools(s *mcp.Server, client *services.APIClientWrapper) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_create",
		Description: "Creates a new DNS resource record (A, AAAA, CNAME, etc.).",
	}, dnsRecordCreateHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_delete",
		Description: "Deletes a specific DNS resource record.",
	}, dnsRecordDeleteHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_list",
		Description: "Lists DNS records with filtering.",
	}, dnsRecordListHandler(client))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_zone_list",
		Description: "Lists DNS zones.",
	}, dnsZoneListHandler(client))
}

func dnsRecordCreateHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DNSRecordCreateInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordCreateInput) (*mcp.CallToolResult, any, error) {
		input := sdsclient.DnsRrAddInput{
			ZoneName: &in.Zone,
			RrName:   &in.Name,
			RrType:   &in.Type,
			RrValue1: &in.Value,
		}
		if in.TTL > 0 {
			input.RrTtl = &in.TTL
		}
		if in.Server != "" {
			input.ServerName = &in.Server
		}
		if in.View != "" {
			input.ViewName = &in.View
		}

		authCtx := client.AuthContext(ctx)
		req := client.DnsApi.DnsRrAdd(authCtx).DnsRrAddInput(input)
		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func dnsRecordDeleteHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DNSRecordDeleteInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordDeleteInput) (*mcp.CallToolResult, any, error) {
		authCtx := client.AuthContext(ctx)
		req := client.DnsApi.DnsRrDelete(authCtx).
			ZoneName(in.Zone).
			RrName(in.Name).
			RrType(in.Type)

		if in.Server != "" {
			req = req.ServerName(in.Server)
		}
		if in.View != "" {
			req = req.ViewName(in.View)
		}

		resp, _, err := req.Execute()
		if err.Error() != "" {
			r, a := errorResult("SolidServer API error: %v", err.Error())
			return r, a, nil
		}

		r, a := jsonResult(resp)
		return r, a, nil
	}
}

func dnsRecordListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DNSRecordListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DnsApi.DnsRrList(authCtx).Limit(limit).Offset(offset)
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

//nolint:dupl // similar list logic across modules
func dnsZoneListHandler(client *services.APIClientWrapper) func(context.Context, *mcp.CallToolRequest, DNSZoneListInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSZoneListInput) (*mcp.CallToolResult, any, error) {
		//nolint:staticcheck // Identical underlying types but conversion is tricky here.
		opts := ListOptions{Where: in.Where, Limit: in.Limit, Offset: in.Offset}
		return commonListHandler(ctx, opts,
			func(c context.Context, where string, limit, offset int32) (any, error) {
				authCtx := client.AuthContext(c)
				req := client.DnsApi.DnsZoneList(authCtx).Limit(limit).Offset(offset)
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
