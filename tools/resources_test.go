package tools

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// lastQuery returns the query values of the most recent recorded request.
func (f *fakeAppliance) lastQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[len(f.requests)-1].query
}

// connectResources registers the resources against a fake appliance and returns
// a connected client session plus the fake, so a test asserts what a real
// client receives and which appliance endpoint was hit.
func connectResources(t *testing.T, status int, body string) (*mcp.ClientSession, *fakeAppliance) {
	t.Helper()
	client, fake := newFakeAppliance(t, status, body)
	cs := connectServer(t, func(s *mcp.Server) { RegisterResources(s, client, testLogger()) })
	return cs, fake
}

// readResourceJSON reads a resource, asserts it has one text-plain fenced
// content block, and returns the parsed JSON body from inside the fence.
func readResourceJSON(t *testing.T, cs *mcp.ClientSession, uri string) map[string]any {
	t.Helper()
	res, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource(%s): %v", uri, err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("ReadResource(%s): expected 1 content, got %d", uri, len(res.Contents))
	}
	c := res.Contents[0]
	if c.MIMEType != resourceMIMEType {
		t.Errorf("ReadResource(%s): MIMEType = %q, want %q", uri, c.MIMEType, resourceMIMEType)
	}
	if strings.Count(c.Text, untrustedOpen) != 1 || strings.Count(c.Text, untrustedClose) != 1 {
		t.Errorf("ReadResource(%s): expected exactly one untrusted-data fence, got %q", uri, c.Text)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(unfence(t, c.Text)), &parsed); err != nil {
		t.Fatalf("ReadResource(%s): fenced body is not valid JSON: %v", uri, err)
	}
	return parsed
}

func TestResourcesRegistered(t *testing.T) {
	cs, _ := connectResources(t, http.StatusOK, `{"data":[]}`)
	ctx := t.Context()

	res, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	gotStatic := make(map[string]string)
	for _, r := range res.Resources {
		gotStatic[r.URI] = r.MIMEType
	}
	for _, want := range []string{uriSpaces, uriDNSZones, uriVLANDomains, uriDHCPServers} {
		mime, ok := gotStatic[want]
		if !ok {
			t.Errorf("static resource %q is not registered", want)
			continue
		}
		if mime != resourceMIMEType {
			t.Errorf("resource %q MIMEType = %q, want %q", want, mime, resourceMIMEType)
		}
	}
	if len(gotStatic) != 4 {
		t.Errorf("expected exactly 4 static resources, got %d: %v", len(gotStatic), gotStatic)
	}

	tmpl, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	gotTmpl := make(map[string]bool)
	for _, r := range tmpl.ResourceTemplates {
		gotTmpl[r.URITemplate] = true
	}
	for _, want := range []string{tmplSubnetByID, tmplZoneRecords, tmplDomainVLANs} {
		if !gotTmpl[want] {
			t.Errorf("resource template %q is not registered", want)
		}
	}
	if len(gotTmpl) != 3 {
		t.Errorf("expected exactly 3 resource templates, got %d: %v", len(gotTmpl), gotTmpl)
	}
}

func TestReadStaticResources(t *testing.T) {
	cases := []struct {
		uri      string
		wantPath string
	}{
		{uriSpaces, "/api/v2.0/ipam/space/list"},
		{uriDNSZones, "/api/v2.0/dns/zone/list"},
		{uriVLANDomains, "/api/v2.0/vlan/domain/list"},
		{uriDHCPServers, "/api/v2.0/dhcp/server/list"},
	}
	for _, tc := range cases {
		t.Run(tc.uri, func(t *testing.T) {
			cs, fake := connectResources(t, http.StatusOK, `{"data":[{}]}`)
			parsed := readResourceJSON(t, cs, tc.uri)
			data, ok := parsed["data"].([]any)
			if !ok || len(data) != 1 {
				t.Errorf("expected a data array of length 1, got %v", parsed["data"])
			}
			if got := fake.lastPath(); got != tc.wantPath {
				t.Errorf("resource %q hit path %q, want %q", tc.uri, got, tc.wantPath)
			}
		})
	}
}

func TestReadSubnetTemplate(t *testing.T) {
	cs, fake := connectResources(t, http.StatusOK, `{"data":[{}]}`)
	parsed := readResourceJSON(t, cs, "solidserver://subnets/42")

	if _, ok := parsed["data"].([]any); !ok {
		t.Errorf("expected a data array, got %v", parsed["data"])
	}
	if got := fake.lastPath(); got != "/api/v2.0/ipam/network/info" {
		t.Errorf("subnet template hit path %q, want /api/v2.0/ipam/network/info", got)
	}
	if got := fake.lastQuery().Get("network_id"); got != "42" {
		t.Errorf("subnet template query network_id = %q, want 42", got)
	}
}

func TestReadSubnetTemplateBadID(t *testing.T) {
	cs, _ := connectResources(t, http.StatusOK, `{"data":[]}`)
	bad := []string{
		"solidserver://subnets/abc",        // non-numeric
		"solidserver://subnets/-1",         // negative
		"solidserver://subnets/",           // empty id
		"solidserver://subnets/2147483648", // exceeds int32, ParseInt bitSize 32 -> ErrRange
	}
	for _, uri := range bad {
		if _, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("ReadResource(%s): expected an error for an invalid id", uri)
		}
	}
}

func TestReadZoneRecordsTemplateBadURI(t *testing.T) {
	cs, _ := connectResources(t, http.StatusOK, `{"data":[]}`)
	bad := []string{
		"solidserver://dns/zones//records",    // empty zone segment
		"solidserver://dns/zones/%zz/records", // invalid percent-encoding -> PathUnescape error
	}
	for _, uri := range bad {
		if _, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("ReadResource(%s): expected an error for a malformed zone-records URI", uri)
		}
	}
}

func TestReadVlanDomainVlansTemplateBadURI(t *testing.T) {
	cs, _ := connectResources(t, http.StatusOK, `{"data":[]}`)
	bad := []string{
		"solidserver://vlan/domains//vlans",    // empty domain segment
		"solidserver://vlan/domains/%zz/vlans", // invalid percent-encoding -> PathUnescape error
	}
	for _, uri := range bad {
		if _, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("ReadResource(%s): expected an error for a malformed domain-vlans URI", uri)
		}
	}
}

func TestReadZoneRecordsTemplate(t *testing.T) {
	cs, fake := connectResources(t, http.StatusOK, `{"data":[{}]}`)
	readResourceJSON(t, cs, "solidserver://dns/zones/example.com/records")

	if got := fake.lastPath(); got != "/api/v2.0/dns/rr/list" {
		t.Errorf("zone-records template hit path %q, want /api/v2.0/dns/rr/list", got)
	}
	if got := fake.lastQuery().Get("where"); !strings.Contains(got, "zone_name='example.com'") {
		t.Errorf("zone-records template WHERE = %q, want it to contain zone_name='example.com'", got)
	}
}

func TestReadVlanDomainVlansTemplate(t *testing.T) {
	cs, fake := connectResources(t, http.StatusOK, `{"data":[{}]}`)
	readResourceJSON(t, cs, "solidserver://vlan/domains/corp/vlans")

	if got := fake.lastPath(); got != "/api/v2.0/vlan/vlan/list" {
		t.Errorf("domain-vlans template hit path %q, want /api/v2.0/vlan/vlan/list", got)
	}
	if got := fake.lastQuery().Get("where"); !strings.Contains(got, "domain_name='corp'") {
		t.Errorf("domain-vlans template WHERE = %q, want it to contain domain_name='corp'", got)
	}
}

func TestResourceContentsAreFenced(t *testing.T) {
	// readResourceJSON already asserts exactly one fence; this names the intent
	// and would fail if a resource ever returned raw, unfenced JSON.
	cs, _ := connectResources(t, http.StatusOK, `{"data":[{}]}`)
	readResourceJSON(t, cs, uriSpaces)
}

func TestReadResourceApplianceError(t *testing.T) {
	const secret = "SENSITIVE_APPLIANCE_ERRMSG"
	cs, _ := connectResources(t, http.StatusInternalServerError, `{"errno":"500","errmsg":"`+secret+`"}`)

	_, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uriSpaces})
	if err == nil {
		t.Fatal("expected an error when the appliance returns 500")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("resource error leaked the appliance errmsg: %v", err)
	}
}

func TestReadUnknownResource(t *testing.T) {
	cs, _ := connectResources(t, http.StatusOK, `{"data":[]}`)
	if _, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "solidserver://nope"}); err == nil {
		t.Error("expected an error for an unregistered resource URI")
	}
}
