package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The prompts package bundles coordinated, multi-step DDI (DNS, DHCP, IPAM)
// workflows into guided templates. The handlers are pure: they emit guidance that
// tells the agent which existing tools to call in what order, and never call
// the appliance themselves. That keeps GetPrompt fast and deterministic, keeps
// live appliance data out of the (unfenced) prompt messages, and means prompts
// are outside the guardrails surface because they change nothing on their own.
const (
	promptProvisionHost    = "solidserver_provision_host"
	promptDecommissionHost = "solidserver_decommission_host"
	promptAuditSubnet      = "solidserver_audit_subnet"
	promptPlanVLANSubnet   = "solidserver_plan_vlan_subnet"
)

// Prompt argument names. Naming them once keeps each prompt's declared argument
// in lockstep with the key its handler reads, so a rename cannot silently
// desync the two.
const (
	argSpace      = "space"
	argSubnet     = "subnet"
	argHostname   = "hostname"
	argZone       = "zone"
	argMAC        = "mac"
	argDHCPServer = "dhcp_server"
	argIPAddress  = "ip_address"
	argSubnetID   = "subnet_id"
	argDomain     = "domain"
	argPrefix     = "prefix"
	argName       = "name"
)

// arg returns the trimmed value of a prompt argument.
func arg(args map[string]string, name string) string {
	return strings.TrimSpace(args[name])
}

// requireArgs returns an invalid-params JSON-RPC error naming the first missing
// required argument, so the client sees a proper invalid-params failure of
// GetPrompt rather than an internal error.
func requireArgs(args map[string]string, names ...string) error {
	for _, n := range names {
		if arg(args, n) == "" {
			return &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: fmt.Sprintf("missing required argument %q", n),
			}
		}
	}
	return nil
}

// userMessage wraps guidance text as a single user-role prompt message.
func userMessage(text string) *mcp.PromptMessage {
	return &mcp.PromptMessage{
		Role:    "user",
		Content: &mcp.TextContent{Text: text},
	}
}

// RegisterPrompts registers the guided DDI workflow prompts. Prompts are pure
// guidance generators, so this takes no client and no *Guardrails.
func RegisterPrompts(s *mcp.Server, logger *slog.Logger) {
	s.AddPrompt(&mcp.Prompt{
		Name:        promptProvisionHost,
		Title:       "Provision a host",
		Description: "Guided workflow to allocate an IP, register its DNS record, and optionally pin a DHCP reservation for a new host.",
		Arguments: []*mcp.PromptArgument{
			{Name: argSpace, Description: "IPAM space the host lives in.", Required: true},
			{Name: argSubnet, Description: "Subnet (CIDR or name) to allocate the address from.", Required: true},
			{Name: argHostname, Description: "Short hostname for the new host.", Required: true},
			{Name: argZone, Description: "DNS zone the forward record goes in.", Required: true},
			{Name: argMAC, Description: "Client MAC address, if a static DHCP reservation is wanted.", Required: false},
			{Name: argDHCPServer, Description: "DHCP server name for the reservation, if a MAC is given.", Required: false},
		},
	}, provisionHostPrompt(logger))

	s.AddPrompt(&mcp.Prompt{
		Name:        promptDecommissionHost,
		Title:       "Decommission a host",
		Description: "Guided workflow to cleanly remove a host: delete its DNS record, remove any DHCP reservation, and release its IPAM allocation.",
		Arguments: []*mcp.PromptArgument{
			{Name: argIPAddress, Description: "IP address to release.", Required: true},
			{Name: argSpace, Description: "IPAM space the address is in.", Required: true},
			{Name: argHostname, Description: "Hostname whose DNS record is removed.", Required: true},
			{Name: argZone, Description: "DNS zone the record is in.", Required: true},
			{Name: argDHCPServer, Description: "DHCP server holding a static reservation, if any.", Required: false},
		},
	}, decommissionHostPrompt(logger))

	s.AddPrompt(&mcp.Prompt{
		Name:        promptAuditSubnet,
		Title:       "Audit a subnet",
		Description: "Guided read-only audit of a subnet: capacity and usage, active allocations, and DHCP pools and leases, highlighting orphaned or out-of-range entries.",
		Arguments: []*mcp.PromptArgument{
			{Name: argSpace, Description: "IPAM space the subnet is in.", Required: true},
			{Name: argSubnetID, Description: "Numeric subnet ID (int32).", Required: true},
		},
	}, auditSubnetPrompt(logger))

	s.AddPrompt(&mcp.Prompt{
		Name:        promptPlanVLANSubnet,
		Title:       "Plan a VLAN and subnet",
		Description: "Guided workflow to check for collisions and then create a matching VLAN and IPAM subnet together.",
		Arguments: []*mcp.PromptArgument{
			{Name: argDomain, Description: "VLAN domain the VLAN is created in.", Required: true},
			{Name: argSpace, Description: "IPAM space the subnet is created in.", Required: true},
			{Name: argPrefix, Description: "CIDR prefix for the new subnet.", Required: true},
			{Name: argName, Description: "Name shared by the new VLAN and subnet.", Required: true},
		},
	}, planVLANSubnetPrompt(logger))
}

func provisionHostPrompt(logger *slog.Logger) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := req.Params.Arguments
		if err := requireArgs(args, argSpace, argSubnet, argHostname, argZone); err != nil {
			return nil, err
		}
		space, subnet := arg(args, argSpace), arg(args, argSubnet)
		hostname, zone := arg(args, argHostname), arg(args, argZone)
		mac, dhcpServer := arg(args, argMAC), arg(args, argDHCPServer)

		logger.Debug("building provision_host prompt", argHostname, hostname, argSpace, space)

		var b strings.Builder
		fmt.Fprintf(&b, "Provision host %q in space %q, subnet %q, DNS zone %q. Work through these steps and confirm each before the next.\n\n", hostname, space, subnet, zone)
		fmt.Fprintf(&b, "1. Confirm the space exists: call solidserver_space_list and check %q is present.\n", space)
		fmt.Fprintf(&b, "2. Locate the subnet: call solidserver_subnet_list (optionally scoped to the space) to find %q, then solidserver_subnet_info on its ID to confirm there is free capacity.\n", subnet)
		b.WriteString("3. Pick a free address: call solidserver_ip_find_free for the subnet.\n")
		fmt.Fprintf(&b, "4. Allocate it: call solidserver_ip_create for %q in space %q with the address from step 3.\n", hostname, space)
		fmt.Fprintf(&b, "5. Register DNS: call solidserver_dns_record_create for an A/AAAA record named %q in zone %q pointing at the allocated address.\n", hostname, zone)
		if mac != "" && dhcpServer != "" {
			fmt.Fprintf(&b, "6. Pin the reservation: call solidserver_dhcp_static_add on server %q binding the allocated address to MAC %q. Prefer an address outside the dynamic range (check solidserver_dhcp_range_list).\n", dhcpServer, mac)
			b.WriteString("7. Summarize the provisioned host: allocated IP, FQDN, MAC, and reservation.\n")
		} else {
			b.WriteString("6. A MAC and a DHCP server are both required for a static reservation; one or both are missing, so skip it.\n")
			b.WriteString("7. Summarize the provisioned host: allocated IP and FQDN.\n")
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Provision host %s", hostname),
			Messages:    []*mcp.PromptMessage{userMessage(b.String())},
		}, nil
	}
}

func decommissionHostPrompt(logger *slog.Logger) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := req.Params.Arguments
		if err := requireArgs(args, argIPAddress, argSpace, argHostname, argZone); err != nil {
			return nil, err
		}
		ip, space := arg(args, argIPAddress), arg(args, argSpace)
		hostname, zone := arg(args, argHostname), arg(args, argZone)
		dhcpServer := arg(args, argDHCPServer)

		logger.Debug("building decommission_host prompt", argIPAddress, ip, argSpace, space)

		var b strings.Builder
		fmt.Fprintf(&b, "Decommission host %q at address %q in space %q, DNS zone %q. This is destructive and cannot be undone from this server; confirm each reference before removing it.\n\n", hostname, ip, space, zone)
		fmt.Fprintf(&b, "1. Verify state: call solidserver_ip_list for %q in space %q and solidserver_dns_record_list for %q in zone %q so you know exactly what will be removed.\n", ip, space, hostname, zone)
		fmt.Fprintf(&b, "2. Remove DNS: call solidserver_dns_record_delete for the record named %q in zone %q.\n", hostname, zone)
		if dhcpServer != "" {
			fmt.Fprintf(&b, "3. Remove the DHCP reservation: confirm it with solidserver_dhcp_lease_list, then call solidserver_dhcp_static_delete for address %q on server %q.\n", ip, dhcpServer)
			fmt.Fprintf(&b, "4. Release the allocation: call solidserver_ip_delete for %q in space %q.\n", ip, space)
			b.WriteString("5. Confirm every reference (DNS, DHCP, IPAM) is gone.\n")
		} else {
			b.WriteString("3. No DHCP server was given, so skip the static reservation removal.\n")
			fmt.Fprintf(&b, "4. Release the allocation: call solidserver_ip_delete for %q in space %q.\n", ip, space)
			b.WriteString("5. Confirm the DNS and IPAM references are gone.\n")
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Decommission host %s", hostname),
			Messages:    []*mcp.PromptMessage{userMessage(b.String())},
		}, nil
	}
}

func auditSubnetPrompt(logger *slog.Logger) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := req.Params.Arguments
		if err := requireArgs(args, argSpace, argSubnetID); err != nil {
			return nil, err
		}
		space := arg(args, argSpace)
		idStr := arg(args, argSubnetID)
		id, err := strconv.ParseInt(idStr, 10, 32)
		if err != nil || id <= 0 {
			return nil, &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: fmt.Sprintf("argument %q must be a positive int32, got %q", argSubnetID, idStr),
			}
		}

		logger.Debug("building audit_subnet prompt", argSubnetID, id, argSpace, space)

		var b strings.Builder
		fmt.Fprintf(&b, "Audit subnet ID %d in space %q. This is read-only; do not change anything.\n\n", id, space)
		fmt.Fprintf(&b, "1. Capacity: call solidserver_subnet_info with id %d for size and utilisation.\n", id)
		b.WriteString("2. Allocations: call solidserver_ip_list scoped to the subnet to enumerate the recorded IPAM addresses.\n")
		b.WriteString("3. DHCP pools and leases: call solidserver_dhcp_range_list for the dynamic ranges and solidserver_dhcp_lease_list for the live leases.\n")
		b.WriteString("4. Cross-reference and summarize: utilisation, free capacity, and anomalies such as leases outside the configured ranges or allocations with no matching lease.\n")

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Audit subnet %d", id),
			Messages:    []*mcp.PromptMessage{userMessage(b.String())},
		}, nil
	}
}

func planVLANSubnetPrompt(logger *slog.Logger) mcp.PromptHandler {
	return func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := req.Params.Arguments
		if err := requireArgs(args, argDomain, argSpace, argPrefix, argName); err != nil {
			return nil, err
		}
		domain, space := arg(args, argDomain), arg(args, argSpace)
		prefix, name := arg(args, argPrefix), arg(args, argName)

		logger.Debug("building plan_vlan_subnet prompt", argDomain, domain, argSpace, space)

		var b strings.Builder
		fmt.Fprintf(&b, "Plan VLAN and subnet %q: create a VLAN in domain %q and a matching subnet %q in space %q. Check for collisions before creating anything.\n\n", name, domain, prefix, space)
		fmt.Fprintf(&b, "1. Confirm the VLAN domain: call solidserver_vlan_domain_list and check %q exists.\n", domain)
		fmt.Fprintf(&b, "2. Find a free VLAN ID: call solidserver_vlan_list scoped to domain %q and pick an unused ID.\n", domain)
		b.WriteString("3. Check subnet overlap: call solidserver_subnet_list in the space and confirm the prefix does not overlap an existing subnet.\n")
		fmt.Fprintf(&b, "4. Create the VLAN: call solidserver_vlan_create in domain %q named %q with the chosen VLAN ID.\n", domain, name)
		fmt.Fprintf(&b, "5. Create the subnet: call solidserver_subnet_create in space %q with prefix %q named %q.\n", space, prefix, name)
		b.WriteString("6. Report the created VLAN ID and subnet.\n")

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Plan VLAN and subnet %s", name),
			Messages:    []*mcp.PromptMessage{userMessage(b.String())},
		}, nil
	}
}
