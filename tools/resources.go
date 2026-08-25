package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// MCP resources expose read-only IPAM/DNS topology snapshots under
// solidserver:// URIs, so a client can attach appliance context without issuing
// a tool call. Each resource delegates to the matching read tool handler and
// renders that handler's fenced output, so a resource read hits the same
// endpoint, validation, and untrusted-data fence as the tool it mirrors.
const (
	// resourceMIMEType is text/plain, not application/json, because the body is
	// the untrusted-data envelope (prose markers wrapping JSON), not raw JSON.
	// Unwrapping the fence yields valid JSON; see fenceUntrusted.
	resourceMIMEType = "text/plain; charset=utf-8"

	// resourceListLimit requests as complete a snapshot as the tool ceiling
	// allows. A resource is a document view, so it pulls the full first page
	// rather than the tool default of defaultListLimit.
	resourceListLimit = maxListLimit

	uriSpaces      = "solidserver://spaces"
	uriDNSZones    = "solidserver://dns/zones"
	uriVLANDomains = "solidserver://vlan/domains"
	uriDHCPServers = "solidserver://dhcp/servers"

	tmplSubnetByID  = "solidserver://subnets/{id}"
	tmplZoneRecords = "solidserver://dns/zones/{zone}/records"
	tmplDomainVLANs = "solidserver://vlan/domains/{domain}/vlans"

	prefixSubnets = "solidserver://subnets/"
	prefixZones   = "solidserver://dns/zones/"
	suffixRecords = "/records"
	prefixDomains = "solidserver://vlan/domains/"
	suffixVLANs   = "/vlans"
)

// contentText concatenates the text of every TextContent block in a tool
// result. On success a delegated read handler returns exactly one fenced-JSON
// TextContent, so this recovers that fenced body for the resource contents.
func contentText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// resourceResult turns a delegated read handler's (result, error) into a
// resource read. Every failure, a non-nil handler error, a nil result, or the
// tool's own IsError path whose text is the fenced appliance error, collapses
// to ONE generic error. The resource error channel is NOT fenced, so the
// attacker-controllable appliance errmsg must never reach it through any path;
// the tool handler already logged the detail. Routing the handler error through
// here too (rather than wrapping it with %w at each call site) keeps that
// redaction single-sourced. On success it returns the handler's already-fenced
// JSON as the resource text.
func resourceResult(uri string, res *mcp.CallToolResult, err error) (*mcp.ReadResourceResult, error) {
	if err != nil || res == nil || res.IsError {
		return nil, fmt.Errorf("failed to read resource %s: appliance request failed", uri)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: resourceMIMEType,
			Text:     contentText(res),
		}},
	}, nil
}

// RegisterResources registers the static list resources and the URI-template
// resources on the server. Resources reuse read-only tool handlers, so they are
// not gated by guardrails and take no *Guardrails.
func RegisterResources(s *mcp.Server, client *services.APIClientWrapper, logger *slog.Logger) {
	s.AddResource(&mcp.Resource{
		Name:        "spaces",
		Title:       "IPAM spaces",
		URI:         uriSpaces,
		MIMEType:    resourceMIMEType,
		Description: "All IPAM spaces on the appliance. A space is the top-level container that subnets and addresses live in; separate spaces may reuse the same RFC 1918 ranges. Mirrors solidserver_space_list.",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, _, err := spaceListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, SpaceListInput{Limit: resourceListLimit})
		return resourceResult(uriSpaces, res, err)
	})

	s.AddResource(&mcp.Resource{
		Name:        "dns-zones",
		Title:       "DNS zones",
		URI:         uriDNSZones,
		MIMEType:    resourceMIMEType,
		Description: "All hosted DNS zones on the appliance. Mirrors solidserver_dns_zone_list.",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, _, err := dnsZoneListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, DNSZoneListInput{Limit: resourceListLimit})
		return resourceResult(uriDNSZones, res, err)
	})

	s.AddResource(&mcp.Resource{
		Name:        "vlan-domains",
		Title:       "VLAN domains",
		URI:         uriVLANDomains,
		MIMEType:    resourceMIMEType,
		Description: "All defined VLAN domains on the appliance. Mirrors solidserver_vlan_domain_list.",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, _, err := vlanDomainListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, VlanDomainListInput{Limit: resourceListLimit})
		return resourceResult(uriVLANDomains, res, err)
	})

	s.AddResource(&mcp.Resource{
		Name:        "dhcp-servers",
		Title:       "DHCP servers",
		URI:         uriDHCPServers,
		MIMEType:    resourceMIMEType,
		Description: "All DHCP servers the appliance manages. Mirrors solidserver_dhcp_server_list.",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, _, err := dhcpServerListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, DhcpServerListInput{Limit: resourceListLimit})
		return resourceResult(uriDHCPServers, res, err)
	})

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "subnet",
		Title:       "Subnet detail by ID",
		URITemplate: tmplSubnetByID,
		MIMEType:    resourceMIMEType,
		Description: "Full detail and usage for one subnet, addressed by its numeric ID, e.g. solidserver://subnets/42. Mirrors solidserver_subnet_info.",
	}, subnetResourceHandler(client, logger))

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "dns-zone-records",
		Title:       "Records in a DNS zone",
		URITemplate: tmplZoneRecords,
		MIMEType:    resourceMIMEType,
		Description: "Resource records within a DNS zone, addressed by zone name, e.g. solidserver://dns/zones/example.com/records. Mirrors solidserver_dns_record_list filtered to the zone.",
	}, zoneRecordsResourceHandler(client, logger))

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "vlan-domain-vlans",
		Title:       "VLANs in a domain",
		URITemplate: tmplDomainVLANs,
		MIMEType:    resourceMIMEType,
		Description: "VLANs within a VLAN domain, addressed by domain name, e.g. solidserver://vlan/domains/corp/vlans. Mirrors solidserver_vlan_list filtered to the domain.",
	}, vlanDomainVLANsResourceHandler(client, logger))
}

// subnetResourceHandler serves solidserver://subnets/{id}. The SDK matches the
// template and passes the raw URI; the handler parses {id} out of it.
func subnetResourceHandler(client *services.APIClientWrapper, logger *slog.Logger) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		idStr := strings.TrimPrefix(uri, prefixSubnets)
		if idStr == uri || idStr == "" {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		id64, err := strconv.ParseInt(idStr, 10, 32)
		if err != nil || id64 <= 0 {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		res, _, herr := subnetInfoHandler(client, logger)(ctx, &mcp.CallToolRequest{}, SubnetInfoInput{ID: int32(id64)})
		return resourceResult(uri, res, herr)
	}
}

// zoneRecordsResourceHandler serves solidserver://dns/zones/{zone}/records.
func zoneRecordsResourceHandler(client *services.APIClientWrapper, logger *slog.Logger) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		mid := strings.TrimPrefix(uri, prefixZones)
		if mid == uri {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		zoneEnc := strings.TrimSuffix(mid, suffixRecords)
		if zoneEnc == mid || zoneEnc == "" {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		zone, err := url.PathUnescape(zoneEnc)
		if err != nil || zone == "" {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		where := fmt.Sprintf("zone_name='%s'", EscapeWhereValue(zone))
		res, _, herr := dnsRecordListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, DNSRecordListInput{Where: where, Limit: resourceListLimit})
		return resourceResult(uri, res, herr)
	}
}

// vlanDomainVLANsResourceHandler serves solidserver://vlan/domains/{domain}/vlans.
func vlanDomainVLANsResourceHandler(client *services.APIClientWrapper, logger *slog.Logger) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		mid := strings.TrimPrefix(uri, prefixDomains)
		if mid == uri {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		domainEnc := strings.TrimSuffix(mid, suffixVLANs)
		if domainEnc == mid || domainEnc == "" {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		domain, err := url.PathUnescape(domainEnc)
		if err != nil || domain == "" {
			return nil, mcp.ResourceNotFoundError(uri)
		}
		// vlanListHandler builds the domain_name filter from VlanListInput.Domain.
		res, _, herr := vlanListHandler(client, logger)(ctx, &mcp.CallToolRequest{}, VlanListInput{Domain: domain, Limit: resourceListLimit})
		return resourceResult(uri, res, herr)
	}
}
