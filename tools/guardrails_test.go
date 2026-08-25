package tools

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const readOnlyErrMsg = "server is in read-only mode: mutating operations are disabled"

// assertRefusal checks that a handler returned a client-side refusal whose text
// contains want, without a transport error. It keeps the guardrail subtests
// small enough to read as one scenario each.
func assertRefusal(t *testing.T, res *mcp.CallToolResult, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected tool error result")
	}
	if text := resultText(res); !strings.Contains(text, want) {
		t.Errorf("unexpected error text: %s", text)
	}
}

func assertReadOnlyError(t *testing.T, res *mcp.CallToolResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected tool error result")
	}
	text := resultText(res)
	if text != readOnlyErrMsg {
		t.Errorf("expected %q, got %q", readOnlyErrMsg, text)
	}
}

func TestGuardrails_Checkers(t *testing.T) {
	g := &Guardrails{
		ReadOnly:         true,
		ProtectedSpaces:  []string{"prod-space", "global"},
		ProtectedZones:   []string{"corp.internal", "prod.example.com."},
		ProtectedSubnets: []string{"10.0.0.0/8", "192.168.1.0/24"},
	}

	if err := g.CheckReadOnly(); err == nil {
		t.Error("expected error for ReadOnly true, got nil")
	}

	// Space check
	if err := g.CheckProtectedSpace("prod-space"); err == nil {
		t.Error("expected error for protected space prod-space, got nil")
	}
	if err := g.CheckProtectedSpace("PROD-SPACE"); err == nil {
		t.Error("expected case-insensitive match for PROD-SPACE, got nil")
	}
	if err := g.CheckProtectedSpace("dev-space"); err != nil {
		t.Errorf("expected nil for dev-space, got %v", err)
	}

	// Zone check
	if err := g.CheckProtectedZone("corp.internal"); err == nil {
		t.Error("expected error for protected zone corp.internal, got nil")
	}
	if err := g.CheckProtectedZone("corp.internal."); err == nil {
		t.Error("expected error for trailing dot in corp.internal., got nil")
	}
	if err := g.CheckProtectedZone("prod.example.com"); err == nil {
		t.Error("expected error for prod.example.com, got nil")
	}
	if err := g.CheckProtectedZone("dev.example.com"); err != nil {
		t.Errorf("expected nil for dev.example.com, got %v", err)
	}

	// Subnet check (direct match and CIDR containment)
	if err := g.CheckProtectedSubnet("10.0.0.0/8"); err == nil {
		t.Error("expected error for protected subnet 10.0.0.0/8, got nil")
	}
	if err := g.CheckProtectedSubnet("10.1.2.3"); err == nil {
		t.Error("expected error for IP within protected subnet 10.0.0.0/8, got nil")
	}
	if err := g.CheckProtectedSubnet("172.16.0.0/12"); err != nil {
		t.Errorf("expected nil for 172.16.0.0/12, got %v", err)
	}
}

func TestCheckProtectedRange(t *testing.T) {
	g := &Guardrails{ProtectedSubnets: []string{"192.168.1.0/24"}}
	tests := []struct {
		name       string
		start, end string
		wantErr    bool
	}{
		{"fully below", "192.168.0.1", "192.168.0.200", false},
		{"fully above", "192.168.2.1", "192.168.2.200", false},
		{"start inside", "192.168.1.10", "192.168.3.10", true},
		{"end inside", "192.168.0.10", "192.168.1.10", true},
		{"span encloses subnet", "192.168.0.10", "192.168.2.10", true},
		{"reversed span still caught", "192.168.2.10", "192.168.0.10", true},
		{"unparseable is left to validation", "not-an-ip", "192.168.9.9", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := g.CheckProtectedRange(tt.start, tt.end); (err != nil) != tt.wantErr {
				t.Errorf("CheckProtectedRange(%q,%q) error = %v, wantErr %v", tt.start, tt.end, err, tt.wantErr)
			}
		})
	}
	if err := (&Guardrails{}).CheckProtectedRange("10.0.0.1", "10.0.0.9"); err != nil {
		t.Errorf("no protected subnets should never refuse, got %v", err)
	}
}

func testReadOnlyIPRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("ip_create refused", func(t *testing.T) {
		handler := ipCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPCreateInput{
			Space:  "dev",
			Subnet: "192.168.1.0",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("ip_delete refused", func(t *testing.T) {
		handler := ipDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPDeleteInput{
			Space:     "dev",
			IPAddress: "192.168.1.10",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("ip_update refused", func(t *testing.T) {
		handler := ipUpdateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPUpdateInput{
			AddressID: 1,
			Name:      "web01",
		})
		assertReadOnlyError(t, res, err)
	})
}

func testReadOnlySubnetRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("subnet_create refused", func(t *testing.T) {
		handler := subnetCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space:   "dev",
			Address: "10.1.0.0",
			Prefix:  "24",
			Name:    "dev-sub",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("subnet_delete refused", func(t *testing.T) {
		handler := subnetDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetDeleteInput{
			Space:   "dev",
			Address: "10.1.0.0",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("space_create refused", func(t *testing.T) {
		handler := spaceCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SpaceCreateInput{Name: "dev"})
		assertReadOnlyError(t, res, err)
	})

	t.Run("space_delete refused", func(t *testing.T) {
		handler := spaceDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SpaceDeleteInput{Name: "dev"})
		assertReadOnlyError(t, res, err)
	})
}

func testReadOnlyDNSRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("dns_record_create refused", func(t *testing.T) {
		handler := dnsRecordCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordCreateInput{
			Zone:  "example.com",
			Name:  "app",
			Type:  "A",
			Value: "10.0.0.1",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dns_record_delete refused", func(t *testing.T) {
		handler := dnsRecordDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordDeleteInput{
			Zone: "example.com",
			Name: "app",
			Type: "A",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dns_record_update refused", func(t *testing.T) {
		handler := dnsRecordUpdateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordUpdateInput{RrID: 1, Value: "10.0.0.2"})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dns_zone_create refused", func(t *testing.T) {
		handler := dnsZoneCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSZoneCreateInput{Zone: "example.com", Type: ZoneTypeMaster})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dns_zone_delete refused", func(t *testing.T) {
		handler := dnsZoneDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSZoneDeleteInput{Zone: "example.com"})
		assertReadOnlyError(t, res, err)
	})
}

func testReadOnlyVlanRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("vlan_create refused", func(t *testing.T) {
		handler := vlanCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, VlanCreateInput{
			Domain: "default",
			Name:   "vlan100",
			VlanID: 100,
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("vlan_delete refused", func(t *testing.T) {
		handler := vlanDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, VlanDeleteInput{
			Domain: "default",
			Name:   "vlan100",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("vlan_domain_create refused", func(t *testing.T) {
		handler := vlanDomainCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, VlanDomainCreateInput{Name: "corp"})
		assertReadOnlyError(t, res, err)
	})

	t.Run("vlan_domain_delete refused", func(t *testing.T) {
		handler := vlanDomainDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, VlanDomainDeleteInput{Name: "corp"})
		assertReadOnlyError(t, res, err)
	})
}

func testReadOnlyDHCPRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("dhcp_static_add refused", func(t *testing.T) {
		handler := dhcpStaticAddHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpStaticAddInput{
			Server: "dhcp1",
			Name:   "host1",
			IP:     "192.168.1.50",
			MAC:    "00:11:22:33:44:55",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_static_delete refused", func(t *testing.T) {
		handler := dhcpStaticDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpStaticDeleteInput{
			Server: "dhcp1",
			IP:     "192.168.1.50",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_scope_create refused", func(t *testing.T) {
		handler := dhcpScopeCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpScopeCreateInput{
			Server:  "dhcp1",
			Address: "192.168.1.0",
			Prefix:  "24",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_range_create refused", func(t *testing.T) {
		handler := dhcpRangeCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpRangeCreateInput{
			Server: "dhcp1",
			Start:  "192.168.1.10",
			End:    "192.168.1.50",
		})
		assertReadOnlyError(t, res, err)
	})
}

func TestGuardrails_ReadOnlyRefusal(t *testing.T) {
	g := &Guardrails{ReadOnly: true}
	logger := slog.Default()

	testReadOnlyIPRefusal(t, g, logger)
	testReadOnlySubnetRefusal(t, g, logger)
	testReadOnlyDNSRefusal(t, g, logger)
	testReadOnlyVlanRefusal(t, g, logger)
	testReadOnlyDHCPRefusal(t, g, logger)
}

func testProtectedSpaceIPRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("protected space in ip_create", func(t *testing.T) {
		handler := ipCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPCreateInput{
			Space:  "production",
			Subnet: "10.1.0.0",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if text != "cannot modify or delete protected space \"production\"" {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("protected space in ip_delete", func(t *testing.T) {
		handler := ipDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPDeleteInput{
			Space:     "production",
			IPAddress: "192.168.1.1",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if text != "cannot modify or delete protected space \"production\"" {
			t.Errorf("unexpected error text: %s", text)
		}
	})
}

func testProtectedSpaceSubnetRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	const want = "cannot modify or delete protected space \"production\""

	t.Run("protected space in subnet_create", func(t *testing.T) {
		res, _, err := subnetCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space: "production", Address: "192.168.10.0", Prefix: "24", Name: "sub1",
		})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected space in subnet_delete", func(t *testing.T) {
		res, _, err := subnetDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, SubnetDeleteInput{
			Space: "production", Address: "192.168.10.0",
		})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected space in space_create", func(t *testing.T) {
		res, _, err := spaceCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, SpaceCreateInput{Name: "production"})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected space in space_delete", func(t *testing.T) {
		res, _, err := spaceDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, SpaceDeleteInput{Name: "production"})
		assertRefusal(t, res, err, want)
	})
}

func testProtectedIPSubnetRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("protected subnet in ip_create", func(t *testing.T) {
		handler := ipCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPCreateInput{
			Space:  "dev",
			Subnet: "10.0.0.0/8",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if text != "cannot modify or delete protected subnet \"10.0.0.0/8\"" {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("protected subnet IP in ip_delete", func(t *testing.T) {
		handler := ipDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPDeleteInput{
			Space:     "dev",
			IPAddress: "10.1.2.3",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if !strings.Contains(text, "within protected subnet") {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("protected hostaddr in ip_create", func(t *testing.T) {
		handler := ipCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPCreateInput{
			Space:    "dev",
			Subnet:   "192.168.1.0",
			Hostaddr: "10.1.2.3",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if !strings.Contains(text, "within protected subnet") {
			t.Errorf("unexpected error text: %s", text)
		}
	})
}

func testProtectedSubnetOverlapRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("protected subnet overlap in subnet_create", func(t *testing.T) {
		handler := subnetCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space:   "dev",
			Address: "10.2.0.0",
			Prefix:  "16",
			Name:    "overlapping-sub",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if !strings.Contains(text, "overlapping protected subnet") {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("protected subnet in subnet_delete", func(t *testing.T) {
		handler := subnetDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetDeleteInput{
			Space:   "dev",
			Address: "10.0.0.0",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if !strings.Contains(text, "protected subnet") {
			t.Errorf("unexpected error text: %s", text)
		}
	})
}

func testProtectedDHCPSubnetRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("protected IP in dhcp_static_add", func(t *testing.T) {
		res, _, err := dhcpStaticAddHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpStaticAddInput{
			Server: "dhcp1", Name: "host1", IP: "10.1.2.3", MAC: "00:11:22:33:44:55",
		})
		assertRefusal(t, res, err, "within protected subnet")
	})

	t.Run("protected IP in dhcp_static_delete", func(t *testing.T) {
		res, _, err := dhcpStaticDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpStaticDeleteInput{
			Server: "dhcp1", IP: "10.1.2.3",
		})
		assertRefusal(t, res, err, "within protected subnet")
	})

	t.Run("protected subnet in dhcp_scope_create", func(t *testing.T) {
		res, _, err := dhcpScopeCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpScopeCreateInput{
			Server: "dhcp1", Address: "10.2.0.0", Prefix: "24",
		})
		assertRefusal(t, res, err, "protected subnet")
	})

	t.Run("whitespace-padded CIDR cannot bypass dhcp_scope_create guard", func(t *testing.T) {
		// Untrimmed input must not slip past the protected-subnet check by making
		// the CIDR unparseable; the handler trims before the guardrail.
		res, _, err := dhcpScopeCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpScopeCreateInput{
			Server: "dhcp1", Address: " 10.2.0.0 ", Prefix: " 24 ",
		})
		assertRefusal(t, res, err, "protected subnet")
	})

	t.Run("protected start in dhcp_range_create", func(t *testing.T) {
		res, _, err := dhcpRangeCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpRangeCreateInput{
			Server: "dhcp1", Start: "10.1.2.3", End: "10.1.2.99",
		})
		assertRefusal(t, res, err, "protected subnet")
	})

	t.Run("dhcp_range_create span crossing into protected subnet", func(t *testing.T) {
		// Start (192.168.0.10) and end (192.168.2.10) both lie outside the
		// protected 192.168.1.0/24, but the span encloses it, so the range must
		// still be refused (a start-only check would pass this).
		spanG := &Guardrails{ProtectedSubnets: []string{"192.168.1.0/24"}}
		res, _, err := dhcpRangeCreateHandler(nil, logger, spanG)(t.Context(), &mcp.CallToolRequest{}, DhcpRangeCreateInput{
			Server: "dhcp1", Start: "192.168.0.10", End: "192.168.2.10",
		})
		assertRefusal(t, res, err, "overlapping protected subnet")
	})
}

func testProtectedZoneRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	const want = "cannot modify or delete protected DNS zone \"corp.internal\""

	t.Run("protected zone in dns_record_create", func(t *testing.T) {
		res, _, err := dnsRecordCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DNSRecordCreateInput{
			Zone: "corp.internal", Name: "test", Type: "A", Value: "10.0.0.1",
		})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected zone in dns_record_delete", func(t *testing.T) {
		res, _, err := dnsRecordDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DNSRecordDeleteInput{
			Zone: "corp.internal", Name: "test", Type: "A",
		})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected zone in dns_record_update", func(t *testing.T) {
		res, _, err := dnsRecordUpdateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DNSRecordUpdateInput{RrID: 1, Value: "10.0.0.2", Zone: "corp.internal"})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected zone in dns_zone_create", func(t *testing.T) {
		res, _, err := dnsZoneCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DNSZoneCreateInput{Zone: "corp.internal", Type: ZoneTypeMaster})
		assertRefusal(t, res, err, want)
	})

	t.Run("protected zone in dns_zone_delete", func(t *testing.T) {
		res, _, err := dnsZoneDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DNSZoneDeleteInput{Zone: "corp.internal"})
		assertRefusal(t, res, err, want)
	})
}

// TestIPUpdateGuardrailLookup covers the ip_update path that resolves an
// address ID to its real subnet before editing, so a protected-subnet rule
// cannot be sidestepped by editing an address the input never names by IP.
func TestIPUpdateGuardrailLookup(t *testing.T) {
	const infoPath = "/api/v2.0/ipam/address/info"
	const editPath = "/api/v2.0/ipam/address/edit"
	logger := testLogger()

	t.Run("refuses an address inside a protected subnet", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"address_hostaddr":"10.1.2.3","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := ipUpdateHandler(client, logger, g)(t.Context(), nil, IPUpdateInput{AddressID: 7, Name: "web01"})
		assertRefusal(t, res, err, "within protected subnet")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the guardrail refusal")
		}
	})

	t.Run("proceeds to edit when the address is not protected", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"address_hostaddr":"192.0.2.5","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := ipUpdateHandler(client, logger, g)(t.Context(), nil, IPUpdateInput{AddressID: 7, Name: "web01"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if !fakeCalled(fake, infoPath) {
			t.Error("expected the address info lookup to run")
		}
		if !fakeCalled(fake, editPath) {
			t.Error("expected the edit to proceed after the guardrail passed")
		}
	})

	t.Run("fails closed when the lookup resolves no hostaddr", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := ipUpdateHandler(client, logger, g)(t.Context(), nil, IPUpdateInput{AddressID: 7, Name: "web01"})
		assertRefusal(t, res, err, "resolved no hostaddr")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite an unverifiable protection")
		}
	})
}

// TestDNSRecordUpdateGuardrailLookup covers the dns_record_update path that
// resolves a record's real zone by rr_id before editing, so a protected-zone
// rule cannot be sidestepped by omitting the optional zone argument.
func TestDNSRecordUpdateGuardrailLookup(t *testing.T) {
	const listPath = "/api/v2.0/dns/rr/list"
	const editPath = "/api/v2.0/dns/rr/edit"
	logger := testLogger()

	t.Run("refuses a record in a protected zone when zone is omitted", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"rr_id":"5","zone_name":"corp.internal"}]}`)
		g := &Guardrails{ProtectedZones: []string{"corp.internal"}}
		res, _, err := dnsRecordUpdateHandler(client, logger, g)(t.Context(), nil, DNSRecordUpdateInput{RrID: 5, Value: "10.0.0.9"})
		assertRefusal(t, res, err, "protected DNS zone")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the protected-zone refusal")
		}
	})

	t.Run("proceeds when the record's zone is not protected", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"rr_id":"5","zone_name":"lab.example.com"}]}`)
		g := &Guardrails{ProtectedZones: []string{"corp.internal"}}
		res, _, err := dnsRecordUpdateHandler(client, logger, g)(t.Context(), nil, DNSRecordUpdateInput{RrID: 5, Value: "10.0.0.9"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if !fakeCalled(fake, listPath) {
			t.Error("expected the rr_id to zone lookup to run")
		}
		if !fakeCalled(fake, editPath) {
			t.Error("expected the edit to proceed after the guardrail passed")
		}
	})

	t.Run("fails closed when the record cannot be resolved", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		g := &Guardrails{ProtectedZones: []string{"corp.internal"}}
		res, _, err := dnsRecordUpdateHandler(client, logger, g)(t.Context(), nil, DNSRecordUpdateInput{RrID: 5, Value: "10.0.0.9"})
		assertRefusal(t, res, err, "rr_id 5 not found")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite an unresolvable record")
		}
	})
}

func TestGuardrails_ProtectedObjectRefusal(t *testing.T) {
	g := &Guardrails{
		ProtectedSpaces:  []string{"production"},
		ProtectedZones:   []string{"corp.internal"},
		ProtectedSubnets: []string{"10.0.0.0/8"},
	}
	logger := slog.Default()

	testProtectedSpaceIPRefusal(t, g, logger)
	testProtectedSpaceSubnetRefusal(t, g, logger)
	testProtectedIPSubnetRefusal(t, g, logger)
	testProtectedSubnetOverlapRefusal(t, g, logger)
	testProtectedDHCPSubnetRefusal(t, g, logger)
	testProtectedZoneRefusal(t, g, logger)
}
