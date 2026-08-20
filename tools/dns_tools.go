package tools

import (
	"context"
	"log/slog"
	"net/http"

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

// DNS Output Structs
type DNSRecordCreateOut struct {
	Data []sdsclient.DataInnerDnsRrAddSuccess `json:"data" jsonschema:"Created DNS record response."`
}

type DNSRecordDeleteOut struct {
	Data []sdsclient.DataInnerDnsRrDeleteSuccess `json:"data" jsonschema:"Deleted DNS record response."`
}

type DNSRecordListOut = ListOutput[sdsclient.DataInnerDnsRrData]
type DNSZoneListOut = ListOutput[sdsclient.DataInnerDnsZoneData]

// RegisterDNSTools registers DNS management tools.
func RegisterDNSTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_create",
		Title:       "Create a DNS record",
		Annotations: additiveTool("Create a DNS record"),
		Description: "Adds a resource record (A, AAAA, CNAME, MX, TXT and so on) to an existing DNS " +
			"zone. The zone must already exist; find it with solidserver_dns_zone_list. This adds a " +
			"record rather than replacing one, so calling it twice for the same name can leave two " +
			"records answering the same query: check with solidserver_dns_record_list first if the " +
			"name may already resolve. Publishing a record changes what resolvers hand out, and " +
			"clients may cache the answer for the record's TTL after it is later removed. Returns " +
			"the created record as JSON.",
	}, dnsRecordCreateHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_delete",
		Title:       "Delete a DNS record",
		Annotations: destructiveTool("Delete a DNS record"),
		Description: "Permanently removes one resource record from a zone. This is destructive and " +
			"cannot be undone from this server. Resolve the exact record first with " +
			"solidserver_dns_record_list, because a name can carry several records and removing the " +
			"wrong one takes a live name out of service. Resolvers may keep serving the old answer " +
			"until its TTL expires, so the effect is not immediate everywhere. Returns a " +
			"confirmation message.",
	}, dnsRecordDeleteHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_list",
		Title:       "List DNS records",
		Annotations: readOnlyTool("List DNS records"),
		Description: "Enumerates individual resource records, optionally narrowed by a where clause. " +
			"Use this to resolve what a name currently points at, to confirm a record exists before " +
			"calling solidserver_dns_record_delete, or to check for a conflict before " +
			"solidserver_dns_record_create. Use solidserver_dns_zone_list instead when you need the " +
			"zones themselves rather than the records inside them. Returns the matching records as " +
			"JSON.",
	}, dnsRecordListHandler(client, logger))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_zone_list",
		Title:       "List DNS zones",
		Annotations: readOnlyTool("List DNS zones"),
		Description: "Enumerates the DNS zones hosted on the appliance. Start here when you do not " +
			"already know the zone name, since solidserver_dns_record_create requires one and a " +
			"record name is meaningless without its zone. Use solidserver_dns_record_list instead to " +
			"see the records within a zone. Returns the zone records as JSON.",
	}, dnsZoneListHandler(client, logger))
}

func validateDNSRecordCreateInput(in *DNSRecordCreateInput) (string, error) {
	if err := ValidateRequiredString(in.Zone, "zone"); err != nil {
		return "", err
	}
	if err := ValidateRequiredString(in.Name, "name"); err != nil {
		return "", err
	}
	rrType, err := ValidateDNSRecordType(in.Type)
	if err != nil {
		return "", err
	}
	if err := ValidateDNSRecordValue(rrType, in.Value); err != nil {
		return "", err
	}
	if err := ValidateTTL(in.TTL); err != nil {
		return "", err
	}
	if err := ValidateOptionalString(in.Server, "server"); err != nil {
		return "", err
	}
	if err := ValidateOptionalString(in.View, "view"); err != nil {
		return "", err
	}
	return rrType, nil
}

func dnsRecordCreateHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DNSRecordCreateInput) (*mcp.CallToolResult, DNSRecordCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordCreateInput) (*mcp.CallToolResult, DNSRecordCreateOut, error) {
		emptyOut := DNSRecordCreateOut{Data: make([]sdsclient.DataInnerDnsRrAddSuccess, 0)}

		rrType, err := validateDNSRecordCreateInput(&in)
		if err != nil {
			return validationErrorResult[DNSRecordCreateOut](err)
		}
		in.Type = rrType

		logger.Info("creating DNS record", "name", in.Name, "zone", in.Zone, "type", in.Type, "value", in.Value)
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
		req := client.DnsAPI.DnsRrAdd(authCtx).DnsRrAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
		}

		var data []sdsclient.DataInnerDnsRrAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDnsRrAddSuccess, 0)
		}
		out := DNSRecordCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dnsRecordDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DNSRecordDeleteInput) (*mcp.CallToolResult, DNSRecordDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordDeleteInput) (*mcp.CallToolResult, DNSRecordDeleteOut, error) {
		emptyOut := DNSRecordDeleteOut{Data: make([]sdsclient.DataInnerDnsRrDeleteSuccess, 0)}

		if err := ValidateRequiredString(in.Zone, "zone"); err != nil {
			return validationErrorResult[DNSRecordDeleteOut](err)
		}
		if err := ValidateRequiredString(in.Name, "name"); err != nil {
			return validationErrorResult[DNSRecordDeleteOut](err)
		}
		rrType, err := ValidateDNSRecordType(in.Type)
		if err != nil {
			return validationErrorResult[DNSRecordDeleteOut](err)
		}
		if err := ValidateOptionalString(in.Server, "server"); err != nil {
			return validationErrorResult[DNSRecordDeleteOut](err)
		}
		if err := ValidateOptionalString(in.View, "view"); err != nil {
			return validationErrorResult[DNSRecordDeleteOut](err)
		}
		in.Type = rrType

		logger.Info("deleting DNS record", "name", in.Name, "zone", in.Zone, "type", in.Type)
		authCtx := client.AuthContext(ctx)
		req := client.DnsAPI.DnsRrDelete(authCtx).
			ZoneName(in.Zone).
			RrName(in.Name).
			RrType(in.Type)

		if in.Server != "" {
			req = req.ServerName(in.Server)
		}
		if in.View != "" {
			req = req.ViewName(in.View)
		}

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			return errorResult("%s", formatAPIError(err, httpResp)), emptyOut, nil
		}

		var data []sdsclient.DataInnerDnsRrDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDnsRrDeleteSuccess, 0)
		}
		out := DNSRecordDeleteOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dnsRecordListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DNSRecordListInput) (*mcp.CallToolResult, DNSRecordListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordListInput) (*mcp.CallToolResult, DNSRecordListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dns_record_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDnsRrData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DnsAPI.DnsRrList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				return resp.Data, httpResp, nil
			})
	}
}

//nolint:dupl // similar list logic across modules
func dnsZoneListHandler(client *services.APIClientWrapper, logger *slog.Logger) func(context.Context, *mcp.CallToolRequest, DNSZoneListInput) (*mcp.CallToolResult, DNSZoneListOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSZoneListInput) (*mcp.CallToolResult, DNSZoneListOut, error) {
		opts := ListOptions(in)
		return commonListHandler(ctx, opts, logger, "solidserver_dns_zone_list",
			func(c context.Context, where string, limit, offset int32) ([]sdsclient.DataInnerDnsZoneData, *http.Response, error) {
				authCtx := client.AuthContext(c)
				req := client.DnsAPI.DnsZoneList(authCtx).Limit(limit).Offset(offset)
				if where != "" {
					req = req.Where(where)
				}
				resp, httpResp, apiErr := req.Execute()
				if apiErr != nil {
					return nil, httpResp, apiErr
				}
				return resp.Data, httpResp, nil
			})
	}
}
