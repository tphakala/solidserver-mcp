package tools

import (
	"context"
	"encoding/json"
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

// fakeCalled reports whether the fake appliance recorded a request to path.
func fakeCalled(fake *fakeAppliance, path string) bool {
	for _, p := range fake.paths() {
		if p == path {
			return true
		}
	}
	return false
}

// connectServer registers a caller-supplied set on an in-memory MCP server and
// returns a connected client session. It is the single place the NewServer +
// NewInMemoryTransports + server/client Connect + Cleanup wiring lives, shared
// by the resources, prompts, and quality tests so those three call sites could
// not drift.
func connectServer(t *testing.T, register func(s *mcp.Server)) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "solidserver-mcp", Version: "test"}, nil)
	register(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := t.Context()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

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
			res, out, err := vlanDomainListHandler(c, l)(ctx, nil, VlanDomainListInput{})
			return res, out, err
		}},
		{"vlan_list", "/api/v2.0/vlan/vlan/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := vlanListHandler(c, l)(ctx, nil, VlanListInput{Domain: testVlanDom})
			return res, out, err
		}},
		{"vlan_create", "/api/v2.0/vlan/vlan/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := vlanCreateHandler(c, l, nil)(ctx, nil, VlanCreateInput{Domain: testVlanDom, VlanID: 10, Name: "guest"})
			return res, out, err
		}},
		{"vlan_delete", "/api/v2.0/vlan/vlan/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := vlanDeleteHandler(c, l, nil)(ctx, nil, VlanDeleteInput{Domain: testVlanDom, Name: "guest"})
			return res, out, err
		}},
		{"dns_record_create", "/api/v2.0/dns/rr/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsRecordCreateHandler(c, l, nil)(ctx, nil, DNSRecordCreateInput{Zone: "example.com", Name: testRecord, Type: "A", Value: "192.0.2.10"})
			return res, out, err
		}},
		{"dns_record_delete", "/api/v2.0/dns/rr/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsRecordDeleteHandler(c, l, nil)(ctx, nil, DNSRecordDeleteInput{Zone: "example.com", Name: testRecord, Type: "A"})
			return res, out, err
		}},
		{"dns_record_list", "/api/v2.0/dns/rr/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsRecordListHandler(c, l)(ctx, nil, DNSRecordListInput{})
			return res, out, err
		}},
		{"dns_zone_list", "/api/v2.0/dns/zone/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsZoneListHandler(c, l)(ctx, nil, DNSZoneListInput{})
			return res, out, err
		}},
		{"subnet_list", "/api/v2.0/ipam/network/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := subnetListHandler(c, l)(ctx, nil, SubnetListInput{Space: testSpace})
			return res, out, err
		}},
		{"subnet_info", "/api/v2.0/ipam/network/info", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := subnetInfoHandler(c, l)(ctx, nil, SubnetInfoInput{ID: 42})
			return res, out, err
		}},
		{"subnet_create", "/api/v2.0/ipam/network/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := subnetCreateHandler(c, l, nil)(ctx, nil, SubnetCreateInput{Space: testSpace, Address: testSubnet, Prefix: "24", Name: "lan"})
			return res, out, err
		}},
		{"subnet_delete", "/api/v2.0/ipam/network/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := subnetDeleteHandler(c, l, nil)(ctx, nil, SubnetDeleteInput{Space: testSpace, Address: testSubnet})
			return res, out, err
		}},
		{"space_list", "/api/v2.0/ipam/space/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := spaceListHandler(c, l)(ctx, nil, SpaceListInput{})
			return res, out, err
		}},
		{"ip_delete", "/api/v2.0/ipam/address/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := ipDeleteHandler(c, l, nil)(ctx, nil, IPDeleteInput{IPAddress: "192.0.2.10", Space: testSpace})
			return res, out, err
		}},
		{"ip_find_free", pathAddrList, func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := ipFindFreeHandler(c, l)(ctx, nil, IPFindFreeInput{Space: testSpace, Subnet: testSubnet})
			return res, out, err
		}},
		{"ip_list", pathAddrList, func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := ipListHandler(c, l)(ctx, nil, IPListInput{Space: testSpace})
			return res, out, err
		}},
		{"dhcp_server_list", "/api/v2.0/dhcp/server/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpServerListHandler(c, l)(ctx, nil, DhcpServerListInput{})
			return res, out, err
		}},
		{"dhcp_scope_list", "/api/v2.0/dhcp/scope/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpScopeListHandler(c, l)(ctx, nil, DhcpScopeListInput{})
			return res, out, err
		}},
		{"dhcp_range_list", "/api/v2.0/dhcp/range/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpRangeListHandler(c, l)(ctx, nil, DhcpRangeListInput{})
			return res, out, err
		}},
		{"dhcp_lease_list", "/api/v2.0/dhcp/lease/list", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpLeaseListHandler(c, l)(ctx, nil, DhcpLeaseListInput{})
			return res, out, err
		}},
		{"dhcp_static_add", "/api/v2.0/dhcp/static/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpStaticAddHandler(c, l, nil)(ctx, nil, DhcpStaticAddInput{Server: "dhcp1", Name: "printer", IP: "192.0.2.50", MAC: "01:00:11:22:33:44:55"})
			return res, out, err
		}},
		{"dhcp_static_delete", "/api/v2.0/dhcp/static/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpStaticDeleteHandler(c, l, nil)(ctx, nil, DhcpStaticDeleteInput{Server: "dhcp1", IP: "192.0.2.50"})
			return res, out, err
		}},
		{"dns_record_update", "/api/v2.0/dns/rr/edit", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsRecordUpdateHandler(c, l, nil)(ctx, nil, DNSRecordUpdateInput{RrID: 1, Value: "192.0.2.20"})
			return res, out, err
		}},
		{"dns_zone_create", "/api/v2.0/dns/zone/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsZoneCreateHandler(c, l, nil)(ctx, nil, DNSZoneCreateInput{Zone: "example.com", Type: ZoneTypeMaster})
			return res, out, err
		}},
		{"dns_zone_delete", "/api/v2.0/dns/zone/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dnsZoneDeleteHandler(c, l, nil)(ctx, nil, DNSZoneDeleteInput{Zone: "example.com"})
			return res, out, err
		}},
		{"ip_update", "/api/v2.0/ipam/address/edit", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := ipUpdateHandler(c, l, nil)(ctx, nil, IPUpdateInput{AddressID: 1, Name: "web01"})
			return res, out, err
		}},
		{"space_create", "/api/v2.0/ipam/space/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := spaceCreateHandler(c, l, nil)(ctx, nil, SpaceCreateInput{Name: "newspace"})
			return res, out, err
		}},
		{"space_delete", "/api/v2.0/ipam/space/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := spaceDeleteHandler(c, l, nil)(ctx, nil, SpaceDeleteInput{Name: "newspace"})
			return res, out, err
		}},
		{"vlan_domain_create", "/api/v2.0/vlan/domain/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := vlanDomainCreateHandler(c, l, nil)(ctx, nil, VlanDomainCreateInput{Name: testVlanDom})
			return res, out, err
		}},
		{"vlan_domain_delete", "/api/v2.0/vlan/domain/delete", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := vlanDomainDeleteHandler(c, l, nil)(ctx, nil, VlanDomainDeleteInput{Name: testVlanDom})
			return res, out, err
		}},
		{"dhcp_scope_create", "/api/v2.0/dhcp/scope/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpScopeCreateHandler(c, l, nil)(ctx, nil, DhcpScopeCreateInput{Server: "dhcp1", Address: testSubnet, Prefix: "24"})
			return res, out, err
		}},
		{"dhcp_range_create", "/api/v2.0/dhcp/range/add", func(ctx context.Context, c *services.APIClientWrapper) (*mcp.CallToolResult, any, error) {
			res, out, err := dhcpRangeCreateHandler(c, l, nil)(ctx, nil, DhcpRangeCreateInput{Server: "dhcp1", Start: "192.0.2.10", End: "192.0.2.50"})
			return res, out, err
		}},
	}
}

// TestSubnetInfoResolvesByCIDR covers the CIDR resolution path added to
// subnet_info: a CIDR is looked up via network/list and then the numeric ID is
// used against network/info, and an unresolvable or ambiguous CIDR fails
// clearly instead of hitting network/info with a bogus ID.
func TestSubnetInfoResolvesByCIDR(t *testing.T) {
	l := testLogger()
	const listPath = "/api/v2.0/ipam/network/list"
	const infoPath = "/api/v2.0/ipam/network/info"

	t.Run("single match resolves then fetches detail", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_id":"42","network_start_hostaddr":"10.0.0.0","network_is_terminal":"1","network_size":"256"}]}`)
		res, _, err := subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{CIDR: "10.0.0.0/24"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if !fakeCalled(fake, listPath) || !fakeCalled(fake, infoPath) {
			t.Errorf("expected network/list then network/info, got %v", fake.paths())
		}
	})

	t.Run("no match fails without hitting network/info", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		res, _, err := subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{CIDR: "10.0.0.0/24"})
		assertRefusal(t, res, err, "no terminal subnet found")
		if fakeCalled(fake, infoPath) {
			t.Error("network/info was called despite no resolution")
		}
	})

	t.Run("ambiguous match asks for disambiguation", func(t *testing.T) {
		client, _ := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_id":"1","network_start_hostaddr":"10.0.0.0","network_is_terminal":"1","network_size":"64"},{"network_id":"2","network_start_hostaddr":"10.0.0.0","network_is_terminal":"1","network_size":"32"}]}`)
		res, _, err := subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{CIDR: "10.0.0.0/24"})
		assertRefusal(t, res, err, "matches 2 subnets")
	})
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

	res, _, err := ipCreateHandler(client, testLogger(), nil)(t.Context(), nil,
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

	res, _, err := ipCreateHandler(client, testLogger(), nil)(t.Context(), nil,
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

	res, _, err := ipCreateHandler(client, testLogger(), nil)(t.Context(), nil,
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

// TestFindFreeIPErrorIsFenced covers the one path where appliance error text is
// folded into a wrapped Go error and surfaced through the unfenced errorResult:
// the free-IP lookup inside ip_create. The appliance portion must still arrive
// fenced so it cannot be read as instructions.
func TestFindFreeIPErrorIsFenced(t *testing.T) {
	client, _ := newFakeAppliance(t, http.StatusInternalServerError, `{"errno":"9","errmsg":"lookup blew up"}`)

	res, _, err := ipCreateHandler(client, testLogger(), nil)(t.Context(), nil,
		IPCreateInput{Space: testSpace, Subnet: testSubnet})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result when the free-IP lookup fails")
	}
	text := resultText(res)
	if !strings.Contains(text, "finding free IP in subnet") {
		t.Errorf("expected the trusted wrapper prose, got: %s", text)
	}
	if strings.Count(text, untrustedOpen) != 1 || strings.Count(text, untrustedClose) != 1 {
		t.Errorf("expected the appliance error portion fenced exactly once, got: %s", text)
	}
}

// TestHandlerInputValidationRejectsLocally confirms every handler's client-side
// validation fires before making an HTTP request. The fake appliance returns
// HTTP 200 with an empty body; if a request is made, the test fails on the count.
func TestHandlerInputValidationRejectsLocally(t *testing.T) {
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
				res, out, err := ipCreateHandler(client, l, nil)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: "999.999.999.999"})
				return res, out, err
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "ip_create invalid hostaddr",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := ipCreateHandler(client, l, nil)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: testSubnet, Hostaddr: "invalid-ip"})
				return res, out, err
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "ip_create invalid mac",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := ipCreateHandler(client, l, nil)(t.Context(), nil, IPCreateInput{Space: testSpace, Subnet: testSubnet, Hostaddr: "192.0.2.10", Mac: "bad-mac"})
				return res, out, err
			},
			wantMsg: "is not a valid MAC address",
		},
		{
			name: "ip_delete invalid ip",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := ipDeleteHandler(client, l, nil)(t.Context(), nil, IPDeleteInput{Space: testSpace, IPAddress: "not-an-ip"})
				return res, out, err
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "subnet_create invalid prefix",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := subnetCreateHandler(client, l, nil)(t.Context(), nil, SubnetCreateInput{Space: testSpace, Address: testSubnet, Prefix: "45", Name: "test"})
				return res, out, err
			},
			wantMsg: "prefix 45 is invalid for IPv4",
		},
		{
			name: "subnet_info without id or cidr",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{})
				return res, out, err
			},
			wantMsg: "provide either id or cidr",
		},
		{
			name: "subnet_info invalid cidr",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := subnetInfoHandler(client, l)(t.Context(), nil, SubnetInfoInput{CIDR: "not-a-cidr"})
				return res, out, err
			},
			wantMsg: "is not a valid CIDR",
		},
		{
			name: "vlan_create invalid vlan id",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := vlanCreateHandler(client, l, nil)(t.Context(), nil, VlanCreateInput{Domain: testVlanDom, VlanID: 5000, Name: "guest"})
				return res, out, err
			},
			wantMsg: "vlan_id must be between 1 and 4094",
		},
		{
			name: "dns_record_create invalid type",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsRecordCreateHandler(client, l, nil)(t.Context(), nil, DNSRecordCreateInput{Zone: "example.com", Name: "host", Type: "INVALID_TYPE", Value: "192.0.2.1"})
				return res, out, err
			},
			wantMsg: "unsupported DNS record type",
		},
		{
			name: "dns_record_create A record with IPv6 value",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsRecordCreateHandler(client, l, nil)(t.Context(), nil, DNSRecordCreateInput{Zone: "example.com", Name: "host", Type: "A", Value: "2001:db8::1"})
				return res, out, err
			},
			wantMsg: "is not a valid IPv4 address",
		},
		{
			name: "dhcp_static_add invalid mac",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dhcpStaticAddHandler(client, l, nil)(t.Context(), nil, DhcpStaticAddInput{Server: "srv1", Name: "printer", IP: "192.0.2.50", MAC: "invalid-mac"})
				return res, out, err
			},
			wantMsg: "is not a valid DHCP MAC address",
		},
		{
			name: "ip_list null byte in space rejected",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := ipListHandler(client, l)(t.Context(), nil, IPListInput{Space: "corp\x00space"})
				return res, out, err
			},
			wantMsg: "cannot contain null bytes",
		},
		{
			name: "vlan_list null byte in domain rejected",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := vlanListHandler(client, l)(t.Context(), nil, VlanListInput{Domain: "dom\x00ain"})
				return res, out, err
			},
			wantMsg: "cannot contain null bytes",
		},
		{
			name: "dns_record_create null byte in zone rejected",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsRecordCreateHandler(client, l, nil)(t.Context(), nil, DNSRecordCreateInput{Zone: "example\x00.com", Name: "host", Type: "A", Value: "192.0.2.1"})
				return res, out, err
			},
			wantMsg: "cannot contain null bytes",
		},
		{
			name: "dns_record_update with nothing to change",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsRecordUpdateHandler(client, l, nil)(t.Context(), nil, DNSRecordUpdateInput{RrID: 1})
				return res, out, err
			},
			wantMsg: "no fields to update",
		},
		{
			name: "dns_zone_create invalid type",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsZoneCreateHandler(client, l, nil)(t.Context(), nil, DNSZoneCreateInput{Zone: "example.com", Type: "bogus"})
				return res, out, err
			},
			wantMsg: "unsupported DNS zone type",
		},
		{
			name: "ip_update with nothing to change",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := ipUpdateHandler(client, l, nil)(t.Context(), nil, IPUpdateInput{AddressID: 1})
				return res, out, err
			},
			wantMsg: "no fields to update",
		},
		{
			name: "space_create missing name",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := spaceCreateHandler(client, l, nil)(t.Context(), nil, SpaceCreateInput{})
				return res, out, err
			},
			wantMsg: "name parameter is required",
		},
		{
			name: "dhcp_scope_create rejects IPv6",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dhcpScopeCreateHandler(client, l, nil)(t.Context(), nil, DhcpScopeCreateInput{Server: "dhcp1", Address: "2001:db8::", Prefix: "64"})
				return res, out, err
			},
			wantMsg: "must be IPv4",
		},
		{
			name: "dhcp_range_create invalid end",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dhcpRangeCreateHandler(client, l, nil)(t.Context(), nil, DhcpRangeCreateInput{Server: "dhcp1", Start: "192.0.2.10", End: "not-an-ip"})
				return res, out, err
			},
			wantMsg: "is not a valid IP address",
		},
		{
			name: "dhcp_range_create reversed range",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dhcpRangeCreateHandler(client, l, nil)(t.Context(), nil, DhcpRangeCreateInput{Server: "dhcp1", Start: "192.0.2.50", End: "192.0.2.10"})
				return res, out, err
			},
			wantMsg: "must not be before start",
		},
		{
			name: "dhcp_range_create mixed address family",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dhcpRangeCreateHandler(client, l, nil)(t.Context(), nil, DhcpRangeCreateInput{Server: "dhcp1", Start: "192.0.2.10", End: "2001:db8::1"})
				return res, out, err
			},
			wantMsg: "same address family",
		},
		{
			name: "dns_record_update null byte in zone",
			invoke: func() (*mcp.CallToolResult, any, error) {
				res, out, err := dnsRecordUpdateHandler(client, l, nil)(t.Context(), nil, DNSRecordUpdateInput{RrID: 1, Value: "192.0.2.20", Zone: "corp\x00.internal"})
				return res, out, err
			},
			wantMsg: "cannot contain null bytes",
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
// resultText delegates to the production contentText so the test extraction of
// a result's text cannot silently diverge from what resources render.
func resultText(res *mcp.CallToolResult) string {
	return contentText(res)
}

// unfence returns the body between the untrusted-data markers, so a test can
// parse the JSON that tool output carries inside its untrusted-data envelope.
func unfence(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, untrustedOpen)
	if start < 0 {
		t.Fatalf("no untrusted-data opening marker in %q", text)
	}
	start += len(untrustedOpen)
	rel := strings.Index(text[start:], untrustedClose)
	if rel < 0 {
		t.Fatalf("no untrusted-data closing marker in %q", text)
	}
	return strings.TrimSpace(text[start : start+rel])
}

// TestStructuredAPIErrorDetails verifies that errno, errmsg, HTTP status, and remediation hints
// are surfaced in error responses for model consumption.
func TestStructuredAPIErrorDetails(t *testing.T) {
	client, _ := newFakeAppliance(t, http.StatusNotFound, `{"errno":"1404","errmsg":"space not found"}`)

	res, out, err := ipListHandler(client, testLogger())(t.Context(), nil, IPListInput{
		Space: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError to be true on 404 response")
	}
	text := resultText(res)
	if !strings.Contains(text, "status 404") {
		t.Errorf("expected error to mention status 404, got: %s", text)
	}
	if !strings.Contains(text, "errno 1404") {
		t.Errorf("expected error to mention errno 1404, got: %s", text)
	}
	if !strings.Contains(text, "space not found") {
		t.Errorf("expected error to contain errmsg 'space not found', got: %s", text)
	}
	if !strings.Contains(text, "verify target space") {
		t.Errorf("expected remediation hint in error text, got: %s", text)
	}
	if out.Data == nil {
		t.Errorf("expected non-nil Data slice on error, got nil")
	}

	// The error is surfaced as a structured APIError so a client can branch on
	// fields rather than parse prose. It rides inside the untrusted-data fence.
	var apiErr APIError
	if err := json.Unmarshal([]byte(unfence(t, text)), &apiErr); err != nil {
		t.Fatalf("error content is not parseable structured JSON: %v (text: %s)", err, text)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("expected structured Status=404, got %d", apiErr.Status)
	}
	if apiErr.Errno != "1404" {
		t.Errorf("expected structured Errno=1404, got %q", apiErr.Errno)
	}
	if apiErr.Errmsg != "space not found" {
		t.Errorf("expected structured Errmsg='space not found', got %q", apiErr.Errmsg)
	}
	if apiErr.Hint == "" {
		t.Error("expected a non-empty structured Hint")
	}
}

// TestListOutputsGuaranteeArraysAndPagination verifies that empty listings serialize
// as empty arrays rather than null and carry pagination metadata.
func TestListOutputsGuaranteeArraysAndPagination(t *testing.T) {
	client, _ := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)

	res, out, err := subnetListHandler(client, testLogger())(t.Context(), nil, SubnetListInput{
		Space:  testSpace,
		Limit:  25,
		Offset: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", resultText(res))
	}
	if out.Data == nil {
		t.Fatal("expected non-nil Data slice, got nil")
	}
	if len(out.Data) != 0 {
		t.Errorf("expected 0 items, got %d", len(out.Data))
	}
	if out.Count != 0 {
		t.Errorf("expected Count=0, got %d", out.Count)
	}
	if out.Limit != 25 {
		t.Errorf("expected Limit=25, got %d", out.Limit)
	}
	if out.Offset != 10 {
		t.Errorf("expected Offset=10, got %d", out.Offset)
	}
	if out.HasMore {
		t.Errorf("expected HasMore=false for an empty page, got true")
	}
	if out.NextOffset != 0 {
		t.Errorf("expected NextOffset=0 for an empty page, got %d", out.NextOffset)
	}

	text := resultText(res)
	if strings.Contains(text, `"data": null`) {
		t.Errorf("output serialized null data array: %s", text)
	}
	if !strings.Contains(text, `"data": []`) {
		t.Errorf("output should contain empty JSON array: %s", text)
	}
}
