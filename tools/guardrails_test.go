package tools

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
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

	t.Run("subnet_update refused", func(t *testing.T) {
		handler := subnetUpdateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetUpdateInput{SubnetID: 1, Name: "x"})
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

	t.Run("dhcp_scope_delete refused", func(t *testing.T) {
		res, _, err := dhcpScopeDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpScopeDeleteInput{
			Server: "dhcp1", Address: "192.168.1.0",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_range_delete refused", func(t *testing.T) {
		res, _, err := dhcpRangeDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpRangeDeleteInput{
			Server: "dhcp1", Start: "192.168.1.10", End: "192.168.1.50",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_shared_network_create refused", func(t *testing.T) {
		res, _, err := dhcpSharedNetworkCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpSharedNetworkCreateInput{
			Server: "dhcp1", Name: "campus",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_shared_network_delete refused", func(t *testing.T) {
		res, _, err := dhcpSharedNetworkDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpSharedNetworkDeleteInput{
			Server: "dhcp1", Name: "campus",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_group_create refused", func(t *testing.T) {
		res, _, err := dhcpGroupCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpGroupCreateInput{
			Server: "dhcp1", Name: "printers",
		})
		assertReadOnlyError(t, res, err)
	})

	t.Run("dhcp_group_delete refused", func(t *testing.T) {
		res, _, err := dhcpGroupDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpGroupDeleteInput{
			Server: "dhcp1", Name: "printers",
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

	t.Run("protected subnet in dhcp_scope_delete", func(t *testing.T) {
		// The scope's network address 10.2.0.0 sits inside protected 10.0.0.0/8,
		// so the cheap bare-address check refuses it with no appliance lookup.
		res, _, err := dhcpScopeDeleteHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpScopeDeleteInput{
			Server: "dhcp1", Address: "10.2.0.0",
		})
		assertRefusal(t, res, err, "protected subnet")
	})

	t.Run("dhcp_range_delete span crossing into protected subnet", func(t *testing.T) {
		spanG := &Guardrails{ProtectedSubnets: []string{"192.168.1.0/24"}}
		res, _, err := dhcpRangeDeleteHandler(nil, logger, spanG)(t.Context(), &mcp.CallToolRequest{}, DhcpRangeDeleteInput{
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

// TestSubnetUpdateGuardrailLookup covers the subnet_update path that resolves a
// subnet's real extent and space by numeric id before editing, so a
// protected-subnet or protected-space rule cannot be sidestepped by an edit
// that only names the id, and so that a resize cannot grow the subnet into a
// protected neighbour.
func TestSubnetUpdateGuardrailLookup(t *testing.T) {
	const infoPath = "/api/v2.0/ipam/network/info"
	const editPath = "/api/v2.0/ipam/network/edit"
	logger := testLogger()

	t.Run("refuses a subnet inside a protected subnet", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"10.1.0.0","network_end_hostaddr":"10.1.0.255","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Name: "renamed"})
		assertRefusal(t, res, err, "overlapping protected subnet")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the guardrail refusal")
		}
	})

	t.Run("refuses a resize that would grow into a protected subnet", func(t *testing.T) {
		// The subnet (10.0.0.0/24) does not overlap the protected 10.5.0.0/16, so
		// the pre-edit check passes, but resizing it to /8 would swallow the
		// protected subnet. The post-edit CIDR check must catch that.
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.0.0.255","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Prefix: "8"})
		assertRefusal(t, res, err, "overlapping protected subnet")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the resize guardrail refusal")
		}
	})

	t.Run("proceeds to edit when the subnet is not protected", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"192.0.2.0","network_end_hostaddr":"192.0.2.255","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Name: "renamed"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if !fakeCalled(fake, infoPath) {
			t.Error("expected the subnet extent lookup to run")
		}
		if !fakeCalled(fake, editPath) {
			t.Error("expected the edit to proceed after the guardrail passed")
		}
	})

	t.Run("fails closed when the lookup resolves no extent", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Name: "renamed"})
		assertRefusal(t, res, err, "resolved no address extent")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite an unverifiable protection")
		}
	})
}

// TestSubnetDeleteEnclosesProtectedSubnet covers the subnet_delete path that
// resolves a subnet's real extent before deleting, so a larger subnet whose
// start address sits outside every protected subnet but which encloses one
// cannot be deleted; a bare-address check alone would let that through.
func TestSubnetDeleteEnclosesProtectedSubnet(t *testing.T) {
	const listPath = "/api/v2.0/ipam/network/list"
	const deletePath = "/api/v2.0/ipam/network/delete"
	logger := testLogger()

	t.Run("refuses deleting a subnet that encloses a protected subnet", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.255.255.255","network_is_terminal":"1"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := subnetDeleteHandler(client, logger, g)(t.Context(), nil, SubnetDeleteInput{Space: "dev", Address: "10.0.0.0"})
		assertRefusal(t, res, err, "overlapping protected subnet")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite the enclosed protected subnet")
		}
	})

	t.Run("proceeds when the resolved subnet does not overlap", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"192.0.2.0","network_end_hostaddr":"192.0.2.255","network_is_terminal":"1"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := subnetDeleteHandler(client, logger, g)(t.Context(), nil, SubnetDeleteInput{Space: "dev", Address: "192.0.2.0"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if !fakeCalled(fake, listPath) {
			t.Error("expected the subnet extent lookup to run")
		}
		if !fakeCalled(fake, deletePath) {
			t.Error("expected the delete to proceed after the guardrail passed")
		}
	})
}

// TestSubnetDeleteChecksAllNetworksAtAddress covers the multi-row extent lookup:
// a benign terminal subnet and an enclosing non-terminal block can share a start
// address, and the delete API is not restricted to terminal subnets, so every
// network at the address must be checked, not just the first row.
func TestSubnetDeleteChecksAllNetworksAtAddress(t *testing.T) {
	const deletePath = "/api/v2.0/ipam/network/delete"
	logger := testLogger()

	// Row 0 is a benign /24; row 1 is a block that encloses the protected subnet.
	client, fake := newFakeAppliance(t, http.StatusOK,
		`{"data":[`+
			`{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.0.0.255","network_is_terminal":"1"},`+
			`{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.255.255.255","network_is_terminal":"0"}]}`)
	g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
	res, _, err := subnetDeleteHandler(client, logger, g)(t.Context(), nil, SubnetDeleteInput{Space: "dev", Address: "10.0.0.0"})
	assertRefusal(t, res, err, "overlapping protected subnet")
	if fakeCalled(fake, deletePath) {
		t.Error("delete endpoint was called despite an enclosing block subnet at the address")
	}
}

// TestSubnetDeleteFailsClosedOnUnresolvableExtent covers a resolved network row
// that lacks an end address: its span cannot be checked, so the delete is
// refused rather than allowed on an unverifiable extent.
func TestSubnetDeleteFailsClosedOnUnresolvableExtent(t *testing.T) {
	const deletePath = "/api/v2.0/ipam/network/delete"
	logger := testLogger()
	client, fake := newFakeAppliance(t, http.StatusOK,
		`{"data":[{"network_start_hostaddr":"192.0.2.0"}]}`)
	g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
	res, _, err := subnetDeleteHandler(client, logger, g)(t.Context(), nil, SubnetDeleteInput{Space: "dev", Address: "192.0.2.0"})
	assertRefusal(t, res, err, "resolved no address extent")
	if fakeCalled(fake, deletePath) {
		t.Error("delete endpoint was called despite an unverifiable subnet extent")
	}
}

// TestDhcpScopeDeleteResolvesExtent covers the scope-delete guardrail resolving
// the scope's real extent from the appliance, so an enclosing scope whose net
// address sits outside every protected subnet is still refused, and a caller
// cannot narrow the guard because there is no caller-supplied prefix.
func TestDhcpScopeDeleteResolvesExtent(t *testing.T) {
	const listPath = "/api/v2.0/dhcp/scope/list"
	const deletePath = "/api/v2.0/dhcp/scope/delete"
	logger := testLogger()

	t.Run("refuses a scope that encloses a protected subnet", func(t *testing.T) {
		// Scope net address 10.0.0.0 is outside the protected 10.5.0.0/16, but the
		// scope spans 10.0.0.0-10.255.255.255 (size 2^24) and encloses it. The
		// scope_*_address_addr fields carry the hexadecimal encoding the appliance
		// actually returns, which the guard must ignore in favour of the dotted
		// scope_net_addr + scope_size.
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"scope_net_addr":"10.0.0.0","scope_size":"16777216","scope_start_address_addr":"0a000000","scope_end_address_addr":"0affffff"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := dhcpScopeDeleteHandler(client, logger, g)(t.Context(), nil, DhcpScopeDeleteInput{Server: "dhcp1", Address: "10.0.0.0"})
		assertRefusal(t, res, err, "overlapping protected subnet")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite the enclosing scope")
		}
		if !fakeCalled(fake, listPath) {
			t.Error("expected the scope extent lookup to run")
		}
	})

	t.Run("proceeds when the scope does not overlap", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"scope_net_addr":"192.0.2.0","scope_size":"256"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpScopeDeleteHandler(client, logger, g)(t.Context(), nil, DhcpScopeDeleteInput{Server: "dhcp1", Address: "192.0.2.0"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if got := fake.paths(); len(got) != 2 || got[0] != listPath || got[1] != deletePath {
			t.Errorf("expected scope/list then scope/delete in order, got %v", got)
		}
	})

	t.Run("fails closed when a resolved scope has no derivable extent", func(t *testing.T) {
		// The scope row carries no size or netmask, so its span cannot be derived
		// from the dotted fields; the delete must be refused rather than proceed on
		// an unverifiable extent.
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"scope_net_addr":"192.0.2.0"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpScopeDeleteHandler(client, logger, g)(t.Context(), nil, DhcpScopeDeleteInput{Server: "dhcp1", Address: "192.0.2.0"})
		assertRefusal(t, res, err, "resolved no usable address extent")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite an unverifiable scope extent")
		}
	})
}

// TestDhcpSharedNetworkDeleteResolvesChildScopes covers the resolve-before-enforce
// guard on shared_network_delete. A shared network has no address of its own, but
// deleting it can take its member scopes with it, so the member scopes are
// resolved from the appliance and the delete is refused when any overlaps a
// protected subnet, and fails closed when a member scope has no usable extent.
func TestDhcpSharedNetworkDeleteResolvesChildScopes(t *testing.T) {
	const listPath = "/api/v2.0/dhcp/scope/list"
	const deletePath = "/api/v2.0/dhcp/sharednetwork/delete"
	logger := testLogger()

	t.Run("refuses when a member scope overlaps a protected subnet", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"scope_net_addr":"10.5.0.0","scope_size":"256"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		assertRefusal(t, res, err, "overlaps protected subnet")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite a protected member scope")
		}
		if !fakeCalled(fake, listPath) {
			t.Error("expected the member-scope lookup to run")
		}
	})

	t.Run("proceeds when no member scope overlaps", func(t *testing.T) {
		// Per-path fidelity: the scope/list resolve returns scope rows while the
		// sharednetwork/delete returns a delete-success body, so the mutation does
		// not decode the resolve body as its own success type.
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		fake.setResponse(listPath, http.StatusOK, `{"data":[{"scope_net_addr":"192.0.2.0","scope_size":"256"}]}`)
		fake.setResponse(deletePath, http.StatusOK, `{"data":[{"sharednetwork_id":"7"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, out, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if got := fake.paths(); len(got) != 2 || got[0] != listPath || got[1] != deletePath {
			t.Errorf("expected scope/list then sharednetwork/delete in order, got %v", got)
		}
		// Per-path fidelity: the mutation decodes the sharednetwork/delete body, not
		// the scope/list resolve body it would otherwise share.
		if len(out.Data) != 1 || out.Data[0].SharednetworkId == nil || *out.Data[0].SharednetworkId != "7" {
			t.Errorf("delete result did not decode the sharednetwork/delete body: %+v", out.Data)
		}
	})
}

// TestDhcpSharedNetworkDeleteGuardEdgeCases covers the shared-network delete
// guard's fail-closed and fail-open edges: an empty resolve (with a WHERE-filter
// assertion the fake cannot otherwise pin), a failed member-scope resolve, a
// member scope whose extent cannot be derived, and a member-scope page that
// fills to the enumeration cap.
func TestDhcpSharedNetworkDeleteGuardEdgeCases(t *testing.T) {
	const listPath = "/api/v2.0/dhcp/scope/list"
	const deletePath = "/api/v2.0/dhcp/sharednetwork/delete"
	logger := testLogger()

	t.Run("proceeds when the shared network has no member scopes", func(t *testing.T) {
		// The resolve legitimately returns no rows, so there is nothing to protect
		// and the delete proceeds. This exercises the empty-resolve branch a
		// wrong WHERE filter would also hit, so it is paired with the WHERE check.
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		fake.setResponse(deletePath, http.StatusOK, `{"data":[{"sharednetwork_id":"9"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		// Pin the resolve WHERE clause: the fake ignores it, so only this assertion
		// catches a filter that matched zero rows in production and let a protected
		// member scope slip through (fail-open).
		where := resolveWhere(t, fake, listPath)
		if !strings.Contains(where, "sharednetwork_name='campus'") || !strings.Contains(where, "server_name='dhcp1'") {
			t.Errorf("resolve WHERE did not scope to the shared network and server: %q", where)
		}
	})

	t.Run("fails closed when the resolve call errors", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"sharednetwork_id":"1"}]}`)
		fake.setResponse(listPath, http.StatusInternalServerError, `{"errno":"1","errmsg":"boom"}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || !res.IsError {
			t.Fatal("expected an error result when the resolve call fails")
		}
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite a failed member-scope resolve")
		}
	})

	t.Run("fails closed when a member scope has no derivable extent", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"scope_net_addr":"192.0.2.0"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		assertRefusal(t, res, err, "resolved no usable address extent")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite an unverifiable member scope")
		}
	})

	t.Run("fails closed when the member-scope page is truncated at the cap", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, scopeRowsBody(t, maxListLimit))
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpSharedNetworkDeleteHandler(client, logger, g)(t.Context(), nil, DhcpSharedNetworkDeleteInput{Server: "dhcp1", Name: "campus"})
		assertRefusal(t, res, err, "more than can be enumerated")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite a truncated member-scope page")
		}
	})
}

// TestDhcpGroupDeleteResolvesChildStatics covers the resolve-before-enforce guard
// on group_delete. A group has no address of its own, but deleting it can take
// its member static reservations with it, so the reservations are resolved and
// the delete is refused when any address sits inside a protected subnet, and
// fails closed when a member reservation has no usable address.
func TestDhcpGroupDeleteResolvesChildStatics(t *testing.T) {
	const listPath = "/api/v2.0/dhcp/static/list"
	const deletePath = "/api/v2.0/dhcp/group/delete"
	logger := testLogger()

	t.Run("refuses when a member reservation is inside a protected subnet", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"static_addr":"10.5.0.10"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := dhcpGroupDeleteHandler(client, logger, g)(t.Context(), nil, DhcpGroupDeleteInput{Server: "dhcp1", Name: "printers"})
		assertRefusal(t, res, err, "protected subnet")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite a protected member reservation")
		}
		if !fakeCalled(fake, listPath) {
			t.Error("expected the member-reservation lookup to run")
		}
	})

	t.Run("proceeds when no member reservation is protected", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		fake.setResponse(listPath, http.StatusOK, `{"data":[{"static_addr":"192.0.2.50"}]}`)
		fake.setResponse(deletePath, http.StatusOK, `{"data":[{"group_id":"3"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, out, err := dhcpGroupDeleteHandler(client, logger, g)(t.Context(), nil, DhcpGroupDeleteInput{Server: "dhcp1", Name: "printers"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		if got := fake.paths(); len(got) != 2 || got[0] != listPath || got[1] != deletePath {
			t.Errorf("expected static/list then group/delete in order, got %v", got)
		}
		// Per-path fidelity: the mutation decodes the group/delete body, not the
		// static/list resolve body it would otherwise share.
		if len(out.Data) != 1 || out.Data[0].GroupId == nil || *out.Data[0].GroupId != "3" {
			t.Errorf("delete result did not decode the group/delete body: %+v", out.Data)
		}
	})

	t.Run("fails closed when a member reservation has no address", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[{"static_name":"noaddr"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpGroupDeleteHandler(client, logger, g)(t.Context(), nil, DhcpGroupDeleteInput{Server: "dhcp1", Name: "printers"})
		assertRefusal(t, res, err, "resolved no usable address")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite an unverifiable member reservation")
		}
	})

}

// TestDhcpGroupDeleteGuardEdgeCases covers the group delete guard's edges: an
// empty resolve (with a WHERE-filter assertion the fake cannot otherwise pin)
// and a reservation page that fills to the enumeration cap.
func TestDhcpGroupDeleteGuardEdgeCases(t *testing.T) {
	const listPath = "/api/v2.0/dhcp/static/list"
	const deletePath = "/api/v2.0/dhcp/group/delete"
	logger := testLogger()

	t.Run("proceeds when the group has no member reservations", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, `{"data":[]}`)
		fake.setResponse(deletePath, http.StatusOK, `{"data":[{"group_id":"5"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpGroupDeleteHandler(client, logger, g)(t.Context(), nil, DhcpGroupDeleteInput{Server: "dhcp1", Name: "printers"})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if res == nil || res.IsError {
			t.Fatalf("expected success, got error result: %s", resultText(res))
		}
		// Pin the resolve WHERE, which the fake ignores: a filter that matched zero
		// rows in production would fail open and only this assertion catches it.
		where := resolveWhere(t, fake, listPath)
		if !strings.Contains(where, "group_name='printers'") || !strings.Contains(where, "server_name='dhcp1'") {
			t.Errorf("resolve WHERE did not scope to the group and server: %q", where)
		}
	})

	t.Run("fails closed when the reservation page is truncated at the cap", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK, staticRowsBody(t, maxListLimit))
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := dhcpGroupDeleteHandler(client, logger, g)(t.Context(), nil, DhcpGroupDeleteInput{Server: "dhcp1", Name: "printers"})
		assertRefusal(t, res, err, "more than can be enumerated")
		if fakeCalled(fake, deletePath) {
			t.Error("delete endpoint was called despite a truncated reservation page")
		}
	})
}

// TestScopeRowExtent pins the extent derivation at the heart of the scope and
// shared-network delete guards: it must use the dotted scope_net_addr plus the
// address count (scope_size, else scope_net_mask), never the hexadecimal
// scope_*_address_addr fields, and must fail closed (empty) when the extent
// cannot be derived.
func TestScopeRowExtent(t *testing.T) {
	sp := func(s string) *string { return &s }
	tests := []struct {
		name               string
		row                sdsclient.DataInnerDhcpScopeData
		wantStart, wantEnd string
	}{
		{
			name:      "size-based extent",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeNetAddr: sp("10.0.0.0"), ScopeSize: sp("16777216")},
			wantStart: "10.0.0.0", wantEnd: "10.255.255.255",
		},
		{
			name:      "netmask fallback when size absent",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeNetAddr: sp("192.0.2.0"), ScopeNetMask: sp("255.255.255.0")},
			wantStart: "192.0.2.0", wantEnd: "192.0.2.255",
		},
		{
			// The hex fields deliberately encode a DIFFERENT range (10.0.0.0-
			// 10.255.255.255) than the dotted net_addr + size derive (192.0.2.0/24),
			// so the test fails if the code ever reads the hex fields at all, not
			// only if it feeds them to netip.ParseAddr.
			name: "hexadecimal address fields are ignored",
			row: sdsclient.DataInnerDhcpScopeData{
				ScopeNetAddr:          sp("192.0.2.0"),
				ScopeSize:             sp("256"),
				ScopeStartAddressAddr: sp("0a000000"),
				ScopeEndAddressAddr:   sp("0affffff"),
			},
			wantStart: "192.0.2.0", wantEnd: "192.0.2.255",
		},
		{
			name:      "no size or mask is underivable",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeNetAddr: sp("192.0.2.0")},
			wantStart: "", wantEnd: "",
		},
		{
			// A size larger than the 32-bit space must fail closed, not wrap the
			// address arithmetic back to a small (bogus) end address.
			name:      "oversized size fails closed",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeNetAddr: sp("192.0.2.0"), ScopeSize: sp("18446744073709551615")},
			wantStart: "", wantEnd: "",
		},
		{
			name:      "missing net address is underivable",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeSize: sp("256")},
			wantStart: "", wantEnd: "",
		},
		{
			name:      "non-IPv4 net address is underivable",
			row:       sdsclient.DataInnerDhcpScopeData{ScopeNetAddr: sp("2001:db8::"), ScopeSize: sp("256")},
			wantStart: "", wantEnd: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := scopeRowExtent(&tt.row)
			if gotStart != tt.wantStart || gotEnd != tt.wantEnd {
				t.Errorf("scopeRowExtent = (%q, %q), want (%q, %q)", gotStart, gotEnd, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

// TestGuardrailPrefixCanonicalization covers the fail-open bypass a non-canonical
// prefix used to create: netip.ParsePrefix rejects "08" (leading zero) and a
// family-invalid length, while the strconv.Atoi-based validators accept "08", so
// concatenating address+"/"+prefix silently skipped the Overlaps check.
// canonicalCIDR normalizes the prefix so the guard fires.
func TestGuardrailPrefixCanonicalization(t *testing.T) {
	logger := testLogger()
	g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}

	t.Run("subnet_create leading-zero prefix cannot bypass the guard", func(t *testing.T) {
		res, _, err := subnetCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space: "dev", Address: "10.0.0.0", Prefix: "08", Name: "sneaky",
		})
		assertRefusal(t, res, err, "protected subnet")
	})

	t.Run("dhcp_scope_create leading-zero prefix cannot bypass the guard", func(t *testing.T) {
		res, _, err := dhcpScopeCreateHandler(nil, logger, g)(t.Context(), &mcp.CallToolRequest{}, DhcpScopeCreateInput{
			Server: "dhcp1", Address: "10.0.0.0", Prefix: "08",
		})
		assertRefusal(t, res, err, "protected subnet")
	})
}

// TestSubnetUpdateInputAndSpaceGuards covers subnet_update guards the resolve
// test does not: the empty-edit validation, the input-level protected-space
// check, the resolved protected-space refusal, and the resize prefixes that used
// to fail the guard open (leading zero and an IPv4-invalid /128).
func TestSubnetUpdateInputAndSpaceGuards(t *testing.T) {
	const editPath = "/api/v2.0/ipam/network/edit"
	logger := testLogger()

	t.Run("no fields to update is rejected", func(t *testing.T) {
		res, _, err := subnetUpdateHandler(nil, logger, nil)(t.Context(), nil, SubnetUpdateInput{SubnetID: 1})
		assertRefusal(t, res, err, "no fields to update")
	})

	t.Run("protected space via input is refused without a lookup", func(t *testing.T) {
		g := &Guardrails{ProtectedSpaces: []string{"production"}}
		res, _, err := subnetUpdateHandler(nil, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 1, Name: "x", Space: "production"})
		assertRefusal(t, res, err, "protected space")
	})

	t.Run("protected space resolved from the subnet is refused", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"192.0.2.0","network_end_hostaddr":"192.0.2.255","space_name":"production"}]}`)
		g := &Guardrails{ProtectedSpaces: []string{"production"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Name: "x"})
		assertRefusal(t, res, err, "protected space")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the resolved protected space")
		}
	})

	t.Run("leading-zero resize prefix cannot bypass the resize guard", func(t *testing.T) {
		// start 10.0.0.0, resize to "08": canonicalCIDR normalizes to 10.0.0.0/8,
		// which equals the protected subnet, so the guard must fire.
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.0.0.255","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.0.0.0/8"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Prefix: "08"})
		assertRefusal(t, res, err, "protected subnet")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite the leading-zero resize overlap")
		}
	})

	t.Run("IPv4-invalid resize prefix fails closed", func(t *testing.T) {
		client, fake := newFakeAppliance(t, http.StatusOK,
			`{"data":[{"network_start_hostaddr":"10.0.0.0","network_end_hostaddr":"10.0.0.255","space_name":"dev"}]}`)
		g := &Guardrails{ProtectedSubnets: []string{"10.5.0.0/16"}}
		res, _, err := subnetUpdateHandler(client, logger, g)(t.Context(), nil, SubnetUpdateInput{SubnetID: 7, Prefix: "128"})
		assertRefusal(t, res, err, "not valid for subnet")
		if fakeCalled(fake, editPath) {
			t.Error("edit endpoint was called despite an unverifiable resize prefix")
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
