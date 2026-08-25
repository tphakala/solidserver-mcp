package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// DNS Input Structs
type DNSRecordCreateInput struct {
	Zone   string `json:"zone" jsonschema:"DNS zone name where the record will be created."`
	Name   string `json:"name" jsonschema:"Record name (e.g. 'host' or 'host.example.com')."`
	Type   string `json:"type" jsonschema:"DNS record type (e.g. 'A', 'AAAA', 'CNAME', 'PTR', 'TXT', 'MX', 'SRV', 'CAA')."`
	Value  string `json:"value" jsonschema:"Record value/target (e.g. '192.168.1.10' for A, 'target.example.com' for CNAME)."`
	TTL    int32  `json:"ttl,omitempty" jsonschema:"Time-to-live in seconds (optional)."`
	Server string `json:"server,omitempty" jsonschema:"DNS server name (optional, if required by your SolidServer setup)."`
	View   string `json:"view,omitempty" jsonschema:"DNS view name (optional, defaults to default view)."`
}

type DNSRecordDeleteInput struct {
	Zone   string `json:"zone" jsonschema:"DNS zone name."`
	Name   string `json:"name" jsonschema:"Record name to delete."`
	Type   string `json:"type" jsonschema:"DNS record type (e.g. 'A', 'CNAME')."`
	Server string `json:"server,omitempty" jsonschema:"DNS server name (optional)."`
	View   string `json:"view,omitempty" jsonschema:"DNS view name (optional)."`
}

type DNSRecordUpdateInput struct {
	RrID   int32  `json:"rr_id" jsonschema:"Numeric resource-record ID from solidserver_dns_record_list."`
	Value  string `json:"value,omitempty" jsonschema:"New record value or target (e.g. an IP for an A record). Omit to leave the value unchanged."`
	TTL    int32  `json:"ttl,omitempty" jsonschema:"New time-to-live in seconds. Omit (or 0) to leave the TTL unchanged."`
	Zone   string `json:"zone,omitempty" jsonschema:"DNS zone name the record belongs to (used for the protected-zone guardrail)."`
	Server string `json:"server,omitempty" jsonschema:"DNS server name (optional, if required by your SolidServer setup)."`
	View   string `json:"view,omitempty" jsonschema:"DNS view name (optional, defaults to default view)."`
}

type DNSZoneCreateInput struct {
	Zone   string `json:"zone" jsonschema:"DNS zone name to create (e.g. 'example.com')."`
	Type   string `json:"type" jsonschema:"Zone type: one of 'master', 'slave', 'forward', 'stub', 'hint', 'delegation-only'."`
	Server string `json:"server,omitempty" jsonschema:"DNS server name to host the zone (optional)."`
	View   string `json:"view,omitempty" jsonschema:"DNS view name (optional, defaults to default view)."`
}

type DNSZoneDeleteInput struct {
	Zone   string `json:"zone" jsonschema:"DNS zone name to delete."`
	Server string `json:"server,omitempty" jsonschema:"DNS server name hosting the zone (optional)."`
	View   string `json:"view,omitempty" jsonschema:"DNS view name (optional)."`
}

type DNSRecordListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like filter expression (e.g. \"zone_name='example.com' and rr_type='A'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Max number of records to return (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

type DNSZoneListInput struct {
	Where  string `json:"where,omitempty" jsonschema:"SQL-like filter expression (e.g. \"zone_name like '%.example.com'\")."`
	Limit  int32  `json:"limit,omitempty" jsonschema:"Max number of zones to return (default 50)."`
	Offset int32  `json:"offset,omitempty" jsonschema:"Offset for pagination."`
}

// DNS Output Structs
type DNSRecordCreateOut struct {
	Data []sdsclient.DataInnerDnsRrAddSuccess `json:"data" jsonschema:"Created DNS record response."`
}

type DNSRecordDeleteOut struct {
	Data []sdsclient.DataInnerDnsRrDeleteSuccess `json:"data" jsonschema:"Deleted DNS record response."`
}

type DNSRecordUpdateOut struct {
	Data []sdsclient.DataInnerDnsRrEditSuccess `json:"data" jsonschema:"Updated DNS record response."`
}

type DNSZoneCreateOut struct {
	Data []sdsclient.DataInnerDnsZoneAddSuccess `json:"data" jsonschema:"Created DNS zone response."`
}

type DNSZoneDeleteOut struct {
	Data []sdsclient.DataInnerDnsZoneDeleteSuccess `json:"data" jsonschema:"Deleted DNS zone response."`
}

type DNSRecordListOut = ListOutput[sdsclient.DataInnerDnsRrData]
type DNSZoneListOut = ListOutput[sdsclient.DataInnerDnsZoneData]

// RegisterDNSTools registers DNS management tools.
func RegisterDNSTools(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) {
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
	}, dnsRecordCreateHandler(client, logger, g))

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
	}, dnsRecordDeleteHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_record_update",
		Title:       "Update a DNS record",
		Annotations: additiveTool("Update a DNS record"),
		Description: "Edits an existing resource record in place, identified by its numeric rr_id from " +
			"solidserver_dns_record_list, changing its value or TTL without deleting and recreating " +
			"it. Resolve the exact rr_id first, since editing the wrong record silently repoints a " +
			"live name. Changing a value changes what resolvers hand out, and clients may keep the " +
			"old answer until the record's TTL expires. Use solidserver_dns_record_create instead to " +
			"add a new record rather than change one. Returns the updated record as JSON.",
	}, dnsRecordUpdateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_zone_create",
		Title:       "Create a DNS zone",
		Annotations: additiveTool("Create a DNS zone"),
		Description: "Creates a new DNS zone on the appliance so that records can be added to it with " +
			"solidserver_dns_record_create. Check whether the zone already exists with " +
			"solidserver_dns_zone_list first, since creating a zone that overlaps existing " +
			"authority can change which server answers a name. A slave zone needs its masters " +
			"configured on the appliance to transfer, and a freshly created master zone serves only " +
			"the records you then add. Changes appliance state and is undone only by " +
			"solidserver_dns_zone_delete. Returns the created zone as JSON.",
	}, dnsZoneCreateHandler(client, logger, g))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "solidserver_dns_zone_delete",
		Title:       "Delete a DNS zone",
		Annotations: destructiveTool("Delete a DNS zone"),
		Description: "Permanently removes a DNS zone and every resource record it contains. This is " +
			"destructive and cannot be undone from this server; recreating the zone with " +
			"solidserver_dns_zone_create does not restore the records that were inside it. Confirm " +
			"the exact zone with solidserver_dns_zone_list first, because deleting a zone takes every " +
			"name under it out of service at once. Resolvers may keep serving cached answers until " +
			"their TTLs expire. Returns a confirmation message.",
	}, dnsZoneDeleteHandler(client, logger, g))

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

func validateDNSRecordDeleteInput(in *DNSRecordDeleteInput) (string, error) {
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
	if err := ValidateOptionalString(in.Server, "server"); err != nil {
		return "", err
	}
	if err := ValidateOptionalString(in.View, "view"); err != nil {
		return "", err
	}
	return rrType, nil
}

func dnsRecordCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DNSRecordCreateInput) (*mcp.CallToolResult, DNSRecordCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordCreateInput) (*mcp.CallToolResult, DNSRecordCreateOut, error) {
		emptyOut := DNSRecordCreateOut{Data: make([]sdsclient.DataInnerDnsRrAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedZone(in.Zone); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		rrType, err := validateDNSRecordCreateInput(&in)
		if err != nil {
			return validationErrorResult(err, emptyOut)
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
			logger.Error("API error", "tool", "solidserver_dns_record_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
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

func dnsRecordDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DNSRecordDeleteInput) (*mcp.CallToolResult, DNSRecordDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordDeleteInput) (*mcp.CallToolResult, DNSRecordDeleteOut, error) {
		emptyOut := DNSRecordDeleteOut{Data: make([]sdsclient.DataInnerDnsRrDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedZone(in.Zone); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		rrType, err := validateDNSRecordDeleteInput(&in)
		if err != nil {
			return validationErrorResult(err, emptyOut)
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
			logger.Error("API error", "tool", "solidserver_dns_record_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
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
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}

// validateDNSRecordUpdateInput checks the update parameters. The record type is
// unknown without a lookup, so the value is only checked for null bytes here;
// the appliance rejects a value that is malformed for the record's real type.
func validateDNSRecordUpdateInput(in *DNSRecordUpdateInput) error {
	if err := ValidatePositiveInt32(in.RrID, "rr_id"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Value, "value"); err != nil {
		return err
	}
	if err := ValidateTTL(in.TTL); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Server, "server"); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.View, "view"); err != nil {
		return err
	}
	if strings.TrimSpace(in.Value) == "" && in.TTL <= 0 {
		return fmt.Errorf("no fields to update: set value, ttl, or both")
	}
	return nil
}

func buildDNSRrEditInput(in *DNSRecordUpdateInput) sdsclient.DnsRrEditInput {
	input := sdsclient.DnsRrEditInput{RrId: &in.RrID}
	if strings.TrimSpace(in.Value) != "" {
		input.RrValue1 = &in.Value
	}
	if in.TTL > 0 {
		input.RrTtl = &in.TTL
	}
	if in.Zone != "" {
		input.ZoneName = &in.Zone
	}
	if in.Server != "" {
		input.ServerName = &in.Server
	}
	if in.View != "" {
		input.ViewName = &in.View
	}
	return input
}

// guardrailsNeedZoneLookup reports whether a protected-zone rule is configured,
// which the rr_id-only update input cannot be checked against directly.
func guardrailsNeedZoneLookup(g *Guardrails) bool {
	return g != nil && len(g.ProtectedZones) > 0
}

// lookupRecordZone resolves a resource-record ID to its zone name so the
// protected-zone guardrail can be enforced before an edit. It fails CLOSED: an
// API error, a missing record, or a row without a zone all return an error
// result, so an edit whose zone cannot be verified is refused rather than
// allowed. rrID is an int formatted into the filter, so no value-escaping is
// needed.
func lookupRecordZone(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, rrID int32) (zone string, errResult *mcp.CallToolResult) {
	authCtx := client.AuthContext(ctx)
	resp, httpResp, apiErr := client.DnsAPI.DnsRrList(authCtx).Where(fmt.Sprintf("rr_id='%d'", rrID)).Limit(1).Execute()
	closeBody(httpResp)
	if apiErr != nil {
		logger.Error("API error", "tool", "solidserver_dns_record_update", "error", apiErr)
		return "", apiErrorResult(apiErr, httpResp)
	}
	if resp == nil || len(resp.Data) == 0 {
		return "", errorResult("rr_id %d not found", rrID)
	}
	if resp.Data[0].ZoneName == nil || *resp.Data[0].ZoneName == "" {
		return "", errorResult("cannot verify protected-zone rules: rr_id %d resolved no zone", rrID)
	}
	return *resp.Data[0].ZoneName, nil
}

// applyDNSRecordUpdateProtections enforces the protected-zone guardrail against
// the record's real zone, resolved from its rr_id, since the update input's
// zone is optional and a caller could omit it to sidestep a zone-name check.
func applyDNSRecordUpdateProtections(ctx context.Context, client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails, rrID int32) *mcp.CallToolResult {
	zone, errResult := lookupRecordZone(ctx, client, logger, rrID)
	if errResult != nil {
		return errResult
	}
	if err := g.CheckProtectedZone(zone); err != nil {
		return errorResult("%v", err)
	}
	return nil
}

func dnsRecordUpdateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DNSRecordUpdateInput) (*mcp.CallToolResult, DNSRecordUpdateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSRecordUpdateInput) (*mcp.CallToolResult, DNSRecordUpdateOut, error) {
		emptyOut := DNSRecordUpdateOut{Data: make([]sdsclient.DataInnerDnsRrEditSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedZone(in.Zone); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := validateDNSRecordUpdateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		// The input's zone is optional and the appliance edits by rr_id alone, so
		// the zone-name check above is a no-op when the caller omits zone. When a
		// protected zone is configured, resolve the record's real zone and re-check
		// so a protected record cannot be edited by leaving zone unset.
		if guardrailsNeedZoneLookup(g) {
			if res := applyDNSRecordUpdateProtections(ctx, client, logger, g, in.RrID); res != nil {
				return res, emptyOut, nil
			}
		}

		logger.Info("updating DNS record", "rr_id", in.RrID, "zone", in.Zone)
		input := buildDNSRrEditInput(&in)

		authCtx := client.AuthContext(ctx)
		req := client.DnsAPI.DnsRrEdit(authCtx).DnsRrEditInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dns_record_update", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDnsRrEditSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDnsRrEditSuccess, 0)
		}
		out := DNSRecordUpdateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func validateDNSZoneCreateInput(in *DNSZoneCreateInput) error {
	if err := ValidateDomainName(in.Zone, "zone"); err != nil {
		return err
	}
	if err := ValidateDNSZoneType(in.Type); err != nil {
		return err
	}
	if err := ValidateOptionalString(in.Server, "server"); err != nil {
		return err
	}
	return ValidateOptionalString(in.View, "view")
}

func dnsZoneCreateHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DNSZoneCreateInput) (*mcp.CallToolResult, DNSZoneCreateOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSZoneCreateInput) (*mcp.CallToolResult, DNSZoneCreateOut, error) {
		emptyOut := DNSZoneCreateOut{Data: make([]sdsclient.DataInnerDnsZoneAddSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedZone(in.Zone); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := validateDNSZoneCreateInput(&in); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("creating DNS zone", "zone", in.Zone, "type", in.Type)
		input := sdsclient.DnsZoneAddInput{
			ZoneName: &in.Zone,
			ZoneType: &in.Type,
		}
		if in.Server != "" {
			input.ServerName = &in.Server
		}
		if in.View != "" {
			input.ViewName = &in.View
		}

		authCtx := client.AuthContext(ctx)
		req := client.DnsAPI.DnsZoneAdd(authCtx).DnsZoneAddInput(input)
		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dns_zone_create", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDnsZoneAddSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDnsZoneAddSuccess, 0)
		}
		out := DNSZoneCreateOut{Data: data}
		return jsonResult(out), out, nil
	}
}

func dnsZoneDeleteHandler(client *services.APIClientWrapper, logger *slog.Logger, g *Guardrails) func(context.Context, *mcp.CallToolRequest, DNSZoneDeleteInput) (*mcp.CallToolResult, DNSZoneDeleteOut, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, in DNSZoneDeleteInput) (*mcp.CallToolResult, DNSZoneDeleteOut, error) {
		emptyOut := DNSZoneDeleteOut{Data: make([]sdsclient.DataInnerDnsZoneDeleteSuccess, 0)}

		if err := g.CheckReadOnly(); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}
		if err := g.CheckProtectedZone(in.Zone); err != nil {
			return errorResult("%v", err), emptyOut, nil
		}

		if err := ValidateDomainName(in.Zone, "zone"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.Server, "server"); err != nil {
			return validationErrorResult(err, emptyOut)
		}
		if err := ValidateOptionalString(in.View, "view"); err != nil {
			return validationErrorResult(err, emptyOut)
		}

		logger.Info("deleting DNS zone", "zone", in.Zone)
		authCtx := client.AuthContext(ctx)
		req := client.DnsAPI.DnsZoneDelete(authCtx).ZoneName(in.Zone)
		if in.Server != "" {
			req = req.ServerName(in.Server)
		}
		if in.View != "" {
			req = req.ViewName(in.View)
		}

		resp, httpResp, err := req.Execute()
		closeBody(httpResp)
		if err != nil {
			logger.Error("API error", "tool", "solidserver_dns_zone_delete", "error", err)
			return apiErrorResult(err, httpResp), emptyOut, nil
		}

		var data []sdsclient.DataInnerDnsZoneDeleteSuccess
		if resp != nil && resp.Data != nil {
			data = resp.Data
		} else {
			data = make([]sdsclient.DataInnerDnsZoneDeleteSuccess, 0)
		}
		out := DNSZoneDeleteOut{Data: data}
		return jsonResult(out), out, nil
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
				if resp == nil || resp.Data == nil {
					return nil, httpResp, nil
				}
				return resp.Data, httpResp, nil
			})
	}
}
