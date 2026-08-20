package tools

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func getTextContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty tool result content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", res.Content[0])
	}
	return tc.Text
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
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
		text := getTextContent(t, res)
		if text != "server is in read-only mode: mutating operations are disabled" {
			t.Errorf("unexpected error text: %s", text)
		}
	})

	t.Run("ip_delete refused", func(t *testing.T) {
		handler := ipDeleteHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, IPDeleteInput{
			Space:     "dev",
			IPAddress: "192.168.1.10",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
	})
}

func testReadOnlySubnetAndDNSRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("subnet_create refused", func(t *testing.T) {
		handler := subnetCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, SubnetCreateInput{
			Space:   "dev",
			Address: "10.1.0.0",
			Prefix:  "24",
			Name:    "dev-sub",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
	})

	t.Run("dns_record_create refused", func(t *testing.T) {
		handler := dnsRecordCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DNSRecordCreateInput{
			Zone:  "example.com",
			Name:  "app",
			Type:  "A",
			Value: "10.0.0.1",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
	})
}

func testReadOnlyVlanAndDHCPRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
	t.Helper()
	t.Run("vlan_create refused", func(t *testing.T) {
		handler := vlanCreateHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, VlanCreateInput{
			Domain: "default",
			Name:   "vlan100",
			VlanID: 100,
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
	})

	t.Run("dhcp_static_add refused", func(t *testing.T) {
		handler := dhcpStaticAddHandler(nil, logger, g)
		res, _, err := handler(t.Context(), &mcp.CallToolRequest{}, DhcpStaticAddInput{
			Server: "dhcp1",
			Name:   "host1",
			IP:     "192.168.1.50",
			MAC:    "00:11:22:33:44:55",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !res.IsError {
			t.Error("expected tool error result")
		}
	})
}

func TestGuardrails_ReadOnlyRefusal(t *testing.T) {
	g := &Guardrails{ReadOnly: true}
	logger := slog.Default()

	testReadOnlyIPRefusal(t, g, logger)
	testReadOnlySubnetAndDNSRefusal(t, g, logger)
	testReadOnlyVlanAndDHCPRefusal(t, g, logger)
}

func testProtectedSpaceRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
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
		text := getTextContent(t, res)
		if text != "cannot modify or delete protected space \"production\"" {
			t.Errorf("unexpected error text: %s", text)
		}
	})
}

func testProtectedSubnetRefusal(t *testing.T, g *Guardrails, logger *slog.Logger) {
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
		text := getTextContent(t, res)
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
		text := getTextContent(t, res)
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
		text := getTextContent(t, res)
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

	testProtectedSpaceRefusal(t, g, logger)
	testProtectedSubnetRefusal(t, g, logger)
	testProtectedZoneRefusal(t, g, logger)
}
