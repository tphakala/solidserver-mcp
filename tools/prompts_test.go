package tools

import (
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectPrompts registers the prompts on a server and returns a connected
// client session. Prompts are pure, so no appliance client is needed.
func connectPrompts(t *testing.T) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "solidserver-mcp", Version: "test"}, nil)
	RegisterPrompts(server, testLogger())

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

// promptText concatenates the text of every message in a prompt result.
func promptText(t *testing.T, res *mcp.GetPromptResult) string {
	t.Helper()
	if len(res.Messages) == 0 {
		t.Fatal("prompt returned no messages")
	}
	var b strings.Builder
	for _, m := range res.Messages {
		if m.Role != "user" {
			t.Errorf("expected user role, got %q", m.Role)
		}
		tc, ok := m.Content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected TextContent, got %T", m.Content)
		}
		b.WriteString(tc.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

var toolRefRegexp = regexp.MustCompile(`solidserver_[a-z_]+`)

func TestPromptsRegistered(t *testing.T) {
	cs := connectPrompts(t)
	res, err := cs.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}

	// name -> required-arg names
	wantRequired := map[string][]string{
		promptProvisionHost:    {"space", "subnet", "hostname", "zone"},
		promptDecommissionHost: {"ip_address", "space", "hostname", "zone"},
		promptAuditSubnet:      {"space", "subnet_id"},
		promptPlanVLANSubnet:   {"domain", "space", "prefix", "name"},
	}

	got := make(map[string]*mcp.Prompt)
	for _, p := range res.Prompts {
		got[p.Name] = p
	}
	if len(got) != len(wantRequired) {
		t.Errorf("expected %d prompts, got %d: %v", len(wantRequired), len(got), got)
	}

	for name, required := range wantRequired {
		p, ok := got[name]
		if !ok {
			t.Errorf("prompt %q is not registered", name)
			continue
		}
		assertPromptDefinition(t, p, required)
	}
}

// assertPromptDefinition checks one prompt has a title, a description, described
// arguments, and every expected required argument marked required.
func assertPromptDefinition(t *testing.T, p *mcp.Prompt, required []string) {
	t.Helper()
	if p.Title == "" || p.Description == "" {
		t.Errorf("prompt %q must set Title and Description", p.Name)
	}
	gotRequired := make(map[string]bool)
	for _, a := range p.Arguments {
		if a.Description == "" {
			t.Errorf("prompt %q argument %q has no description", p.Name, a.Name)
		}
		if a.Required {
			gotRequired[a.Name] = true
		}
	}
	for _, r := range required {
		if !gotRequired[r] {
			t.Errorf("prompt %q must mark argument %q as required", p.Name, r)
		}
	}
}

func TestGetProvisionHostPrompt(t *testing.T) {
	cs := connectPrompts(t)

	// Without mac/dhcp_server the reservation step is skipped.
	res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: promptProvisionHost,
		Arguments: map[string]string{
			"space": "corp", "subnet": "192.0.2.0/24", "hostname": "web01", "zone": "example.com",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	text := promptText(t, res)
	for _, want := range []string{"web01", "example.com", "solidserver_ip_create", "solidserver_dns_record_create", "solidserver_ip_find_free"} {
		if !strings.Contains(text, want) {
			t.Errorf("provision prompt missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "solidserver_dhcp_static_add") {
		t.Error("provision prompt must not reference dhcp_static_add without a mac and dhcp_server")
	}

	// With mac + dhcp_server the reservation step appears.
	res2, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: promptProvisionHost,
		Arguments: map[string]string{
			"space": "corp", "subnet": "192.0.2.0/24", "hostname": "web01", "zone": "example.com",
			"mac": "00:11:22:33:44:55", "dhcp_server": "dhcp1",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt (with dhcp): %v", err)
	}
	text2 := promptText(t, res2)
	if !strings.Contains(text2, "solidserver_dhcp_static_add") {
		t.Errorf("provision prompt with mac+dhcp_server must reference dhcp_static_add in:\n%s", text2)
	}
	if !strings.Contains(text2, "00:11:22:33:44:55") || !strings.Contains(text2, "dhcp1") {
		t.Errorf("provision prompt must interpolate mac and dhcp_server in:\n%s", text2)
	}
}

func TestGetDecommissionHostPrompt(t *testing.T) {
	cs := connectPrompts(t)
	res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name: promptDecommissionHost,
		Arguments: map[string]string{
			"ip_address": "192.0.2.10", "space": "corp", "hostname": "web01", "zone": "example.com",
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	text := promptText(t, res)
	for _, want := range []string{"solidserver_dns_record_delete", "solidserver_ip_delete", "192.0.2.10"} {
		if !strings.Contains(text, want) {
			t.Errorf("decommission prompt missing %q in:\n%s", want, text)
		}
	}
	if !strings.Contains(strings.ToLower(text), "destructive") && !strings.Contains(strings.ToLower(text), "cannot be undone") {
		t.Errorf("decommission prompt must warn it is destructive in:\n%s", text)
	}
	// No dhcp_server given: static delete step is skipped.
	if strings.Contains(text, "solidserver_dhcp_static_delete") {
		t.Error("decommission prompt must not reference dhcp_static_delete without a dhcp_server")
	}
}

func TestGetAuditSubnetPrompt(t *testing.T) {
	cs := connectPrompts(t)
	res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      promptAuditSubnet,
		Arguments: map[string]string{"space": "corp", "subnet_id": "42"},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	text := promptText(t, res)
	for _, want := range []string{"solidserver_subnet_info", "solidserver_ip_list", "42"} {
		if !strings.Contains(text, want) {
			t.Errorf("audit prompt missing %q in:\n%s", want, text)
		}
	}
}

func TestGetPlanVLANSubnetPrompt(t *testing.T) {
	cs := connectPrompts(t)
	res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      promptPlanVLANSubnet,
		Arguments: map[string]string{"domain": "corp", "space": "corp", "prefix": "192.0.2.0/24", "name": "servers"},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	text := promptText(t, res)
	for _, want := range []string{"solidserver_vlan_create", "solidserver_subnet_create", "servers"} {
		if !strings.Contains(text, want) {
			t.Errorf("plan prompt missing %q in:\n%s", want, text)
		}
	}
}

func TestAuditSubnetRejectsNonNumericID(t *testing.T) {
	cs := connectPrompts(t)
	for _, bad := range []string{"abc", "0", "-3"} {
		_, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
			Name:      promptAuditSubnet,
			Arguments: map[string]string{"space": "corp", "subnet_id": bad},
		})
		if err == nil {
			t.Errorf("audit prompt accepted invalid subnet_id %q", bad)
		}
	}
}

func TestPromptMissingRequiredArg(t *testing.T) {
	cs := connectPrompts(t)
	// omit "zone" from provision_host
	_, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      promptProvisionHost,
		Arguments: map[string]string{"space": "corp", "subnet": "192.0.2.0/24", "hostname": "web01"},
	})
	if err == nil {
		t.Error("expected an error when a required prompt argument is missing")
	}
}

// TestPromptMessagesReferenceRealTools keeps prompts honest: every solidserver_*
// tool a prompt names must be a tool this server actually registers, so a
// renamed or removed tool cannot leave a prompt pointing at a phantom.
func TestPromptMessagesReferenceRealTools(t *testing.T) {
	cs := connectPrompts(t)
	fullArgs := map[string]map[string]string{
		promptProvisionHost: {
			"space": "corp", "subnet": "192.0.2.0/24", "hostname": "web01", "zone": "example.com",
			"mac": "00:11:22:33:44:55", "dhcp_server": "dhcp1",
		},
		promptDecommissionHost: {
			"ip_address": "192.0.2.10", "space": "corp", "hostname": "web01", "zone": "example.com",
			"dhcp_server": "dhcp1",
		},
		promptAuditSubnet:    {"space": "corp", "subnet_id": "42"},
		promptPlanVLANSubnet: {"domain": "corp", "space": "corp", "prefix": "192.0.2.0/24", "name": "servers"},
	}

	for name, args := range fullArgs {
		res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("GetPrompt(%s): %v", name, err)
		}
		text := promptText(t, res)
		refs := toolRefRegexp.FindAllString(text, -1)
		if len(refs) == 0 {
			t.Errorf("prompt %q references no tools", name)
		}
		for _, ref := range refs {
			if _, ok := expectedTools[ref]; !ok {
				t.Errorf("prompt %q references unknown tool %q", name, ref)
			}
		}
	}
}
