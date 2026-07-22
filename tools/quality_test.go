package tools

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
)

// These tests encode the Tool Definition Quality Score rubric published at
// github.com/glama-ai/tool-definition-quality-score as build gates, so a tool
// definition cannot silently regress below the bar the rest of the set meets.
//
// The rubric scores six dimensions. The ones that can be checked mechanically
// are asserted here:
//
//   - Purpose Clarity is hard-gated to at most 2 when a description is missing
//     or merely restates the name, so both are rejected outright.
//   - Behavioral Transparency depends on annotation coverage, so every tool
//     must carry annotations and every mutating tool must state its hints.
//   - Usage Guidelines reward naming the sibling tool to use instead, so every
//     tool must reference at least one other tool by name.
//
// The remaining dimensions need human or model judgement and are not asserted.

// minDescriptionLen is a floor, not a target. The rubric penalises
// "under-specification presented as brevity"; a description that cannot clear
// this has no room to say when to use the tool, let alone when not to.
const minDescriptionLen = 200

// toolClass is how a tool relates to appliance state, which drives which
// annotations it must carry.
type toolClass string

const (
	classRead        toolClass = "read"
	classAdditive    toolClass = "additive"
	classDestructive toolClass = "destructive"
)

// expectedTools is every tool this server is expected to register, with the
// mutation class each one belongs to. Listing them explicitly means an
// accidentally dropped or renamed tool fails the build.
var expectedTools = map[string]toolClass{
	// read-only
	"solidserver_vlan_domain_list": classRead,
	"solidserver_vlan_list":        classRead,
	"solidserver_dns_record_list":  classRead,
	"solidserver_dns_zone_list":    classRead,
	"solidserver_subnet_list":      classRead,
	"solidserver_subnet_info":      classRead,
	"solidserver_space_list":       classRead,
	"solidserver_dhcp_server_list": classRead,
	"solidserver_dhcp_scope_list":  classRead,
	"solidserver_dhcp_range_list":  classRead,
	"solidserver_dhcp_lease_list":  classRead,
	"solidserver_ip_find_free":     classRead,
	"solidserver_ip_list":          classRead,
	// additive
	"solidserver_vlan_create":       classAdditive,
	"solidserver_dns_record_create": classAdditive,
	"solidserver_subnet_create":     classAdditive,
	"solidserver_ip_create":         classAdditive,
	"solidserver_dhcp_static_add":   classAdditive,
	// destructive
	"solidserver_vlan_delete":        classDestructive,
	"solidserver_dns_record_delete":  classDestructive,
	"solidserver_subnet_delete":      classDestructive,
	"solidserver_ip_delete":          classDestructive,
	"solidserver_dhcp_static_delete": classDestructive,
}

// listRegisteredTools registers every tool on a real server and reads the set
// back over an in-memory transport, so the assertions run against what a client
// actually receives rather than against internal state.
func listRegisteredTools(t *testing.T) []*mcp.Tool {
	t.Helper()

	client, err := services.NewSolidServerClient("solidserver.invalid", "id", "secret", true)
	if err != nil {
		t.Fatalf("NewSolidServerClient: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "solidserver-mcp", Version: "test"}, nil)
	RegisterAll(server, client, slog.New(slog.DiscardHandler))

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

	res, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return res.Tools
}

// referencesSibling reports whether desc names at least one other tool.
func referencesSibling(desc, self string, all []string) bool {
	for _, other := range all {
		if other != self && strings.Contains(desc, other) {
			return true
		}
	}
	return false
}

// TestAllExpectedToolsRegistered fails if a tool is dropped, renamed or added
// without updating expectedTools.
func TestAllExpectedToolsRegistered(t *testing.T) {
	got := make(map[string]int)
	for _, tool := range listRegisteredTools(t) {
		got[tool.Name]++
	}

	for name := range expectedTools {
		switch got[name] {
		case 1:
			// registered exactly once, as intended
		case 0:
			t.Errorf("tool %q is not registered", name)
		default:
			t.Errorf("tool %q is registered %d times, want 1", name, got[name])
		}
	}
	for name := range got {
		if _, want := expectedTools[name]; !want {
			t.Errorf("tool %q is registered but not listed in expectedTools", name)
		}
	}
}

// TestToolDescriptionsAreInformative covers the Purpose Clarity hard gates and
// the Usage Guidelines dimension.
func TestToolDescriptionsAreInformative(t *testing.T) {
	tools := listRegisteredTools(t)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			desc := strings.TrimSpace(tool.Description)
			if desc == "" {
				t.Fatal("description is empty, which hard-gates every TDQS dimension to 1")
			}
			if len(desc) < minDescriptionLen {
				t.Errorf("description is %d chars, want at least %d; too short to say when to use the tool and when not to",
					len(desc), minDescriptionLen)
			}

			// Tautology gate: a description that only restates the name adds
			// nothing beyond the structured fields.
			spaced := strings.ReplaceAll(tool.Name, "_", " ")
			if strings.EqualFold(strings.TrimSuffix(desc, "."), spaced) {
				t.Errorf("description %q merely restates the name", desc)
			}

			// Usage Guidelines: point at the sibling to use instead.
			if !referencesSibling(desc, tool.Name, names) {
				t.Error("description names no sibling tool, so it gives no guidance on when to use something else")
			}
		})
	}
}

// assertClassHints checks the annotations match the tool's mutation class.
func assertClassHints(t *testing.T, ann *mcp.ToolAnnotations, class toolClass) {
	t.Helper()

	switch class {
	case classRead:
		if !ann.ReadOnlyHint {
			t.Error("read-only tool does not set ReadOnlyHint")
		}
	case classAdditive:
		if ann.ReadOnlyHint {
			t.Error("mutating tool sets ReadOnlyHint")
		}
		// DestructiveHint defaults to true when unset, which would misreport
		// a create as destructive.
		if ann.DestructiveHint == nil || *ann.DestructiveHint {
			t.Error("additive tool must set DestructiveHint explicitly false")
		}
	case classDestructive:
		if ann.ReadOnlyHint {
			t.Error("destructive tool sets ReadOnlyHint")
		}
		if ann.DestructiveHint == nil || !*ann.DestructiveHint {
			t.Error("destructive tool must set DestructiveHint explicitly true")
		}
	default:
		t.Fatal("tool is not classified in expectedTools")
	}
}

// TestToolAnnotations covers the Behavioral Transparency dimension. A mutating
// tool with no annotations gives a client no way to know it changes state.
func TestToolAnnotations(t *testing.T) {
	for _, tool := range listRegisteredTools(t) {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Title == "" {
				t.Error("Title is empty")
			}
			if tool.Title == tool.Name {
				t.Errorf("Title %q duplicates Name, so it adds no human-readable value", tool.Title)
			}

			ann := tool.Annotations
			if ann == nil {
				t.Fatal("Annotations is nil, so readOnly/destructive intent is unstated")
			}

			assertClassHints(t, ann, expectedTools[tool.Name])

			// OpenWorldHint defaults to true, but every tool here reaches an
			// external appliance, so state it rather than inherit it.
			if ann.OpenWorldHint == nil || !*ann.OpenWorldHint {
				t.Error("tool calls an external appliance and must set OpenWorldHint true")
			}
		})
	}
}

// TestDestructiveToolsWarnInDescription checks that a destructive tool says so
// in prose, not only in the annotation. Clients that do not surface annotations
// still show the description to the model.
func TestDestructiveToolsWarnInDescription(t *testing.T) {
	for _, tool := range listRegisteredTools(t) {
		if expectedTools[tool.Name] != classDestructive {
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			desc := strings.ToLower(tool.Description)
			if !strings.Contains(desc, "destructive") {
				t.Error("destructive tool does not describe itself as destructive")
			}
			if !strings.Contains(desc, "cannot be undone") && !strings.Contains(desc, "permanent") {
				t.Error("destructive tool does not state that the change is irreversible")
			}
		})
	}
}

// TestToolInputSchemasDocumentEveryParameter covers the Parameter Semantics
// dimension: the rubric drops the baseline score when schema description
// coverage falls below 80%, so require full coverage.
func TestToolInputSchemasDocumentEveryParameter(t *testing.T) {
	for _, tool := range listRegisteredTools(t) {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("input schema has unexpected type %T", tool.InputSchema)
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				// A zero-parameter tool is fine and scores its own baseline.
				return
			}
			for param, raw := range props {
				p, ok := raw.(map[string]any)
				if !ok {
					t.Errorf("parameter %q has unexpected schema type %T", param, raw)
					continue
				}
				if desc, _ := p["description"].(string); strings.TrimSpace(desc) == "" {
					t.Errorf("parameter %q has no description", param)
				}
			}
		})
	}
}
