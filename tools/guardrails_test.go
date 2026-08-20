package tools

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const readOnlyErrMsg = "server is in read-only mode: mutating operations are disabled"

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
	t.Run("protected space in subnet_create", func(t *testing.T) {
		handler := subnetCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space:   "production",
			Address: "192.168.10.0",
			Prefix:  "24",
			Name:    "sub1",
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

	t.Run("protected space in subnet_delete", func(t *testing.T) {
		handler := subnetDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetDeleteInput{
			Space:   "production",
			Address: "192.168.10.0",
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
		handler := dhcpStaticAddHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpStaticAddInput{
			Server: "dhcp1",
			Name:   "host1",
			IP:     "10.1.2.3",
			MAC:    "00:11:22:33:44:55",
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

	t.Run("protected IP in dhcp_static_delete", func(t *testing.T) {
		handler := dhcpStaticDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpStaticDeleteInput{
			Server: "dhcp1",
			IP:     "10.1.2.3",
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

func testProtectedZoneRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("protected zone in dns_record_create", func(t *testing.T) {
		handler := dnsRecordCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordCreateInput{
			Zone:  "corp.internal",
			Name:  "test",
			Type:  "A",
			Value: "10.0.0.1",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if text != "cannot modify or delete protected DNS zone \"corp.internal\"" {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("protected zone in dns_record_delete", func(t *testing.T) {
		handler := dnsRecordDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordDeleteInput{
			Zone: "corp.internal",
			Name: "test",
			Type: "A",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := resultText(res)
		if text != "cannot modify or delete protected DNS zone \"corp.internal\"" {
			t.Errorf("unexpected error text: %s", text)
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
