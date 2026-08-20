package tools

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// fakeAppliance stands in for a SolidServer appliance. The real client is
// pointed at it over TLS, so these tests exercise request construction,
// signing, transport and response decoding rather than mocking them away.
type fakeAppliance struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest

	// status and body are what every request gets back.
	status int
	body   string
}

type recordedRequest struct {
	method string
	path   string
	query  url.Values
}

// newFakeAppliance starts a TLS test server returning the given status and body
// for every request, and returns a client wired to it.
func newFakeAppliance(t *testing.T, status int, body string) (*services.APIClientWrapper, *fakeAppliance) {
	t.Helper()

	fake := &fakeAppliance{status: status, body: body}
	fake.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
		})
		fake.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fake.status)
		_, _ = w.Write([]byte(fake.body))
	}))
	t.Cleanup(fake.server.Close)

	// The client builds https://<host>/api/v2.0, and sslVerify=false sets
	// InsecureSkipVerify, which is what lets it accept the test certificate.
	host := strings.TrimPrefix(fake.server.URL, "https://")
	client, err := services.NewSolidServerClient(host, "token-id", "token-secret", false)
	if err != nil {
		t.Fatalf("NewSolidServerClient: %v", err)
	}
	return client, fake
}

// lastPath returns the path of the most recent request, or "" if none.
func (f *fakeAppliance) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return ""
	}
	return f.requests[len(f.requests)-1].path
}

func (f *fakeAppliance) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// paths returns a copy of the recorded request paths. The test server writes
// f.requests from its own goroutine, so callers must not reach into the slice
// directly even after the response has been read.
func (f *fakeAppliance) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.requests))
	for i, r := range f.requests {
		out[i] = r.path
	}
	return out
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// handlerCase drives one tool handler against the fake appliance. invoke wraps
// the handler so the table can hold handlers with different input types.
type handlerCase struct {
	name string
	// wantPath is the API endpoint the handler is expected to call. Asserting
	// it catches a handler wired to the wrong SolidServer endpoint, which a
	// success-only assertion would miss.
	wantPath string
	invoke   func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error)
}

// Shared fixture values, named so the table reads as one scenario rather than
// a scatter of unrelated literals.
const (
	testSpace    = "default"
	testSubnet   = "192.0.2.0"
	testVlanDom  = "corp"
	testRecord   = "web"
	pathAddrList = "/api/v2.0/ipam/address/list"
	pathAddrAdd  = "/api/v2.0/ipam/address/add"
)

func handlerCases() []handlerCase {
	l := testLogger()
	return []handlerCase{
		{"vlan_domain_list", "/api/v2.0/vlan/domain/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return vlanDomainListHandler(c, l)(ctx, nil, VlanDomainListInput{})
		}},
		{"vlan_list", "/api/v2.0/vlan/vlan/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return vlanListHandler(c, l)(ctx, nil, VlanListInput{Domain: testVlanDom})
		}},
		{"vlan_create", "/api/v2.0/vlan/vlan/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return vlanCreateHandler(c, l)(ctx, nil, VlanCreateInput{Domain: testVlanDom, VlanID: 10, Name: "guest"})
		}},
		{"vlan_delete", "/api/v2.0/vlan/vlan/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return vlanDeleteHandler(c, l)(ctx, nil, VlanDeleteInput{Domain: testVlanDom, Name: "guest"})
		}},
		{"dns_record_create", "/api/v2.0/dns/rr/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dnsRecordCreateHandler(c, l)(ctx, nil, DNSRecordCreateInput{Zone: "example.com", Name: testRecord, Type: "A", Value: "192.0.2.10"})
		}},
		{"dns_record_delete", "/api/v2.0/dns/rr/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dnsRecordDeleteHandler(c, l)(ctx, nil, DNSRecordDeleteInput{Zone: "example.com", Name: testRecord, Type: "A"})
		}},
		{"dns_record_list", "/api/v2.0/dns/rr/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dnsRecordListHandler(c, l)(ctx, nil, DNSRecordListInput{})
		}},
		{"dns_zone_list", "/api/v2.0/dns/zone/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dnsZoneListHandler(c, l)(ctx, nil, DNSZoneListInput{})
		}},
		{"subnet_list", "/api/v2.0/ipam/network/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return subnetListHandler(c, l)(ctx, nil, SubnetListInput{Space: testSpace})
		}},
		{"subnet_info", "/api/v2.0/ipam/network/info", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return subnetInfoHandler(c, l)(ctx, nil, SubnetInfoInput{ID: 42})
		}},
		{"subnet_create", "/api/v2.0/ipam/network/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return subnetCreateHandler(c, l)(ctx, nil, SubnetCreateInput{Space: testSpace, Address: testSubnet, Prefix: "24", Name: "lan"})
		}},
		{"subnet_delete", "/api/v2.0/ipam/network/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return subnetDeleteHandler(c, l)(ctx, nil, SubnetDeleteInput{Space: testSpace, Address: testSubnet})
		}},
		{"space_list", "/api/v2.0/ipam/space/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return spaceListHandler(c, l)(ctx, nil, SpaceListInput{})
		}},
		{"ip_delete", "/api/v2.0/ipam/address/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return ipDeleteHandler(c, l)(ctx, nil, IPDeleteInput{IPAddress: "192.0.2.10", Space: testSpace})
		}},
		{"ip_find_free", pathAddrList, func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return ipFindFreeHandler(c, l)(ctx, nil, IPFindFreeInput{Space: testSpace, Subnet: testSubnet})
		}},
		{"ip_list", pathAddrList, func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return ipListHandler(c, l)(ctx, nil, IPListInput{Space: testSpace})
		}},
		{"dhcp_server_list", "/api/v2.0/dhcp/server/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpServerListHandler(c, l)(ctx, nil, DhcpServerListInput{})
		}},
		{"dhcp_scope_list", "/api/v2.0/dhcp/scope/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpScopeListHandler(c, l)(ctx, nil, DhcpScopeListInput{})
		}},
		{"dhcp_range_list", "/api/v2.0/dhcp/range/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpRangeListHandler(c, l)(ctx, nil, DhcpRangeListInput{})
		}},
		{"dhcp_lease_list", "/api/v2.0/dhcp/lease/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpLeaseListHandler(c, l)(ctx, nil, DhcpLeaseListInput{})
		}},
		{"dhcp_static_add", "/api/v2.0/dhcp/static/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpStaticAddHandler(c, l)(ctx, nil, DhcpStaticAddInput{Server: "dhcp1", Name: "printer", IP: "192.0.2.50", MAC: "01:00:11:22:33:44:55"})
		}},
		{"dhcp_static_delete", "/api/v2.0/dhcp/static/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			return dhcpStaticDeleteHandler(c, l)(ctx, nil, DhcpStaticDeleteInput{Server: "dhcp1", IP: "192.0.2.50"})
		}},
	}
}

// TestHandlersSuccess checks every handler reaches its intended endpoint and
// reports success when the appliance answers normally.
func TestHandlersSuccess(t *testing.T) {
	for _, tc := range handlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"id":"1"}]}`)

			res, _, err := tc.invoke(t.Context(), client)
			if err != nil {
				t.Fatalf("handler returned a transport error: %v", err)
			}
			if res == nil {
				t.Fatal("handler returned a nil result")
			}
			if res.IsError {
				t.Errorf("handler reported an error for a healthy response: %s", resultText(res))
			}
			if fake.count() == 0 {
				t.Fatal("handler made no request to the appliance")
			}
			if got := fake.lastPath(); got != tc.wantPath {
				t.Errorf("called %s, want %s", got, tc.wantPath)
			}
		})
	}
}

// TestHandlersReportAPIErrors checks that an appliance failure comes back as a
// tool error result rather than a Go error. Returning err would surface as a
// JSON-RPC protocol error and lose the message; the handlers deliberately
// convert it into IsError content the model can read.
func TestHandlersReportAPIErrors(t *testing.T) {
	for _, tc := range handlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newFakeAppliance(t, http.StatusInternalServerError, `{"errno":"1","errmsg":"appliance exploded"}`)

			res, _, err := tc.invoke(t.Context(), client)
			if err != nil {
				t.Fatalf("handler returned a Go error instead of an error result: %v", err)
			}
			if res == nil {
				t.Fatal("handler returned a nil result")
			}
			if !res.IsError {
				t.Errorf("handler reported success for a 500 response: %s", resultText(res))
			}
			if text := resultText(res); text == "" {
				t.Error("error result carries no text for the model to read")
			}
		})
	}
}

// TestIPCreateFindsFreeAddress covers the two-step path in ipCreateHandler:
// with no hostaddr it first lists free addresses, then allocates the first one.
func TestIPCreateFindsFreeAddress(t *testing.T) {
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"address_hostaddr":"192.0.2.25"}]}`)

	res, _, err := ipCreateHandler(client, testLogger())(t.Context(), nil,
		IPCreateInput{Space: testSpace, Subnet: testSubnet})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(res))
	}
	got := fake.paths()
	if len(got) != 2 {
		t.Fatalf("expected a list call then an add call, got %d request(s)", len(got))
	}
	if got[0] != pathAddrList {
		t.Errorf("first call was %s, want the free-address lookup", got[0])
	}
	if got[1] != pathAddrAdd {
		t.Errorf("second call was %s, want the allocation", got[1])
	}
}

// TestIPCreateWithExplicitAddressSkipsLookup checks the other branch: a caller
// supplying hostaddr must not trigger the free-address search.
func TestIPCreateWithExplicitAddressSkipsLookup(t *testing.T) {
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"id":"1"}]}`)

	res, _, err := ipCreateHandler(client, testLogger())(t.Context(), nil,
		IPCreateInput{Space: testSpace, Subnet: testSubnet, Hostaddr: "192.0.2.99", Name: testRecord, Mac: "00:11:22:33:44:55"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(res))
	}
	if fake.count() != 1 {
		t.Fatalf("expected a single add call, got %d request(s)", fake.count())
	}
	if got := fake.lastPath(); got != pathAddrAdd {
		t.Errorf("called %s, want the allocation endpoint", got)
	}
}

// TestIPCreateNoFreeAddress covers the exhausted-subnet path: the lookup
// succeeds but returns nothing, which must be an error result rather than an
// allocation attempt with an empty address.
func TestIPCreateNoFreeAddress(t *testing.T) {
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)

	res, _, err := ipCreateHandler(client, testLogger())(t.Context(), nil,
		IPCreateInput{Space: testSpace, Subnet: testSubnet})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result when the subnet has no free address")
	}
	if !strings.Contains(resultText(res), "no free IP found") {
		t.Errorf("error text does not explain the cause: %s", resultText(res))
	}
	if fake.count() != 1 {
		t.Errorf("expected only the lookup call, got %d request(s); an allocation must not be attempted", fake.count())
	}
}

// TestHandlerInputValidationRejectsInvalidParameters checks that malformed inputs
// are rejected on the client side without contacting the remote appliance.
func TestHandlerInputValidationRejectsInvalidParameters(t *testing.T) {
	l := testLogger()
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)

	cases := []struct {
		name    string
		invoke  func() (*mcp.CallToolResult, any, error)
		wantMsg string
	}{
		{
			name: "ip_create invalid subnet",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return ipCreateHandler(client, l)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: "999.999.999.999"})
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "ip_create invalid hostaddr",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return ipCreateHandler(client, l)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: testSubnet, Hostaddr: "invalid-ip"})
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "ip_create invalid mac",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return ipCreateHandler(client, l)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: testSubnet, Hostaddr: "192.0.2.10", Mac: "bad-mac"})
			},
			wantMsg: "is not a valid MAC address",
		},
		{
			name: "ip_delete invalid ip",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return ipDeleteHandler(client, l)(t.Context(), nil, IPDeleteInput{Space: testSpace, IPAddress: "not-an-ip"})
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "subnet_create invalid prefix",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return subnetCreateHandler(client, l)(t.Context(), nil, SubnetCreateInput{Space: testSpace, Address: testSubnet, Prefix: "45", Name: "test"})
			},
			wantMsg: "prefix 45 is invalid for IPv4",
		},
		{
			name: "subnet_info non-positive id",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{ID: 0})
			},
			wantMsg: "must be a positive integer",
		},
		{
			name: "vlan_create invalid vlan id",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return vlanCreateHandler(client, l)(t.Context(), nil, VlanCreateInput{Domain: testVlanDom, VlanID: 5000, Name: "guest"})
			},
			wantMsg: "vlan_id must be between 1 and 4094",
		},
		{
			name: "dns_record_create invalid type",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return dnsRecordCreateHandler(client, l)(t.Context(), nil, DNSRecordCreateInput{Zone: "example.com", Name: "host", Type: "INVALID_TYPE", Value: "192.0.2.1"})
			},
			wantMsg: "unsupported DNS record type",
		},
		{
			name: "dns_record_create A record with IPv6 value",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return dnsRecordCreateHandler(client, l)(t.Context(), nil, DNSRecordCreateInput{Zone: "example.com", Name: "host", Type: "A", Value: "2001:db8::1"})
			},
			wantMsg: "is not a valid IPv4 address",
		},
		{
			name: "dhcp_static_add invalid mac",
			invoke: func() (*mcp.CallToolResult, any, error) {
				return dhcpStaticAddHandler(client, l)(t.Context(), nil, DhcpStaticAddInput{Server: "srv1", Name: "printer", IP: "192.0.2.50", MAC: "invalid-mac"})
			},
			wantMsg: "is not a valid DHCP MAC address",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeCount := fake.count()
			res, _, err := tc.invoke()
			if err != nil {
				t.Fatalf("unexpected transport error: %v", err)
			}
			if res == nil || !res.IsError {
				t.Fatalf("expected error result, got %v", res)
			}
			if !strings.Contains(resultText(res), tc.wantMsg) {
				t.Errorf("error text %q does not contain expected substring %q", resultText(res), tc.wantMsg)
			}
			if fake.count() != beforeCount {
				t.Errorf("handler contacted the appliance for invalid input; requests before=%d, after=%d", beforeCount, fake.count())
			}
		})
	}
}

// TestWHEREClauseSanitizationAndEscaping checks that quotes in identifiers are properly
// escaped when interpolated into WHERE clauses.
func TestWHEREClauseSanitizationAndEscaping(t *testing.T) {
	l := testLogger()
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)

	t.Run("ip_find_free escapes quote in space and subnet", func(t *testing.T) {
		_, _, _ = ipFindFreeHandler(client, l)(t.Context(), nil, IPFindFreeInput{
			Space:  "corp's space",
			Subnet: testSubnet,
		})
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if len(fake.requests) == 0 {
			t.Fatal("expected request to fake appliance")
		}
		lastReq := fake.requests[len(fake.requests)-1]
		whereQuery := lastReq.query.Get("where")
		if !strings.Contains(whereQuery, `space_name='corp\'s space'`) {
			t.Errorf("WHERE query %q does not contain properly escaped space_name", whereQuery)
		}
	})

	t.Run("vlan_list escapes quote in domain name", func(t *testing.T) {
		_, _, _ = vlanListHandler(client, l)(t.Context(), nil, VlanListInput{
			Domain: "dom'ain",
		})
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if len(fake.requests) == 0 {
			t.Fatal("expected request to fake appliance")
		}
		lastReq := fake.requests[len(fake.requests)-1]
		whereQuery := lastReq.query.Get("where")
		if !strings.Contains(whereQuery, `domain_name='dom\'ain'`) {
			t.Errorf("WHERE query %q does not contain properly escaped domain_name", whereQuery)
		}
	})
}

// TestUnbalancedWHEREClauseRejected checks that WHERE clauses with unclosed quotes
// are rejected on the client side.
func TestUnbalancedWHEREClauseRejected(t *testing.T) {
	l := testLogger()
	client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)

	res, _, err := ipListHandler(client, l)(t.Context(), nil, IPListInput{
		Space: testSpace,
		Where: "address_name = 'unterminated",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result for unbalanced quote, got %v", res)
	}
	if !strings.Contains(resultText(res), "unclosed single quote") {
		t.Errorf("expected unclosed quote error message, got %s", resultText(res))
	}
	if fake.count() != 0 {
		t.Errorf("handler contacted appliance despite invalid where clause")
	}
}

// resultText flattens a tool result's content for assertions.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
