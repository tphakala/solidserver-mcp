# SolidServer MCP Server

<p align="center">
  <!-- Project Status -->
  <a href="https://github.com/tphakala/solidserver-mcp/releases">
    <img src="https://img.shields.io/github/v/release/tphakala/solidserver-mcp?include_prereleases&style=flat-square&color=blue">
  </a>
  <a href="https://github.com/tphakala/solidserver-mcp/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/tphakala/solidserver-mcp?style=flat-square&color=green">
  </a>
  <a href="https://github.com/tphakala/solidserver-mcp/actions/workflows/test.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/tphakala/solidserver-mcp/test.yml?style=flat-square&label=CI">
  </a>
  <a href="https://github.com/tphakala/solidserver-mcp/actions/workflows/codeql.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/tphakala/solidserver-mcp/codeql.yml?style=flat-square&label=CodeQL">
  </a>

  <br>

  <!-- Code Quality -->
  <a href="https://golang.org">
    <img src="https://img.shields.io/badge/Built%20with-Go-teal?style=flat-square&logo=go">
  </a>

  <br>

  <!-- Community -->
  <a href="https://github.com/tphakala/solidserver-mcp/issues">
    <img src="https://img.shields.io/github/issues/tphakala/solidserver-mcp?style=flat-square&color=red">
  </a>
  <a href="https://coderabbit.ai">
    <img src="https://img.shields.io/coderabbit/prs/github/tphakala/solidserver-mcp?utm_source=oss&utm_medium=github&utm_campaign=tphakala%2Fsolidserver-mcp&labelColor=171717&color=FF570A&link=https%3A%2F%2Fcoderabbit.ai&label=CodeRabbit+Reviews">
  </a>
  <a href="https://github.com/sponsors/tphakala">
    <img src="https://img.shields.io/github/sponsors/tphakala?style=flat-square&logo=github&color=EA4AAA&label=Sponsor">
  </a>
</p>

An MCP (Model Context Protocol) server for EfficientIP SolidServer IPAM/DNS management.

## Installation

Download a prebuilt binary for your platform from the
[releases page](https://github.com/tphakala/solidserver-mcp/releases). Builds are
published for Linux, macOS and Windows on amd64 and arm64, with a `checksums.txt`
alongside them. Extract the archive and put the binary somewhere on your PATH, or
point your MCP client straight at it (see [Stdio Mode](#stdio-mode-standard)).

Build from source instead with Go 1.27 or newer:

```bash
go install github.com/tphakala/solidserver-mcp@latest
```

Or build and run it as a container:

```bash
docker build -t solidserver-mcp .

docker run --rm -i \
  -e SOLIDSERVER_HOST=sds.example.com \
  -e SOLIDSERVER_TOKEN_ID=yourtokenid \
  -e SOLIDSERVER_TOKEN_SECRET=yourtokensecret \
  solidserver-mcp
```

That runs the default stdio transport, so `-i` is required to keep stdin open
for the JSON-RPC channel. For HTTP transport, set `SOLIDSERVER_TRANSPORT=http` and
`SOLIDSERVER_HTTP_HOST=0.0.0.0` and publish the port with `-p 8080:8080`.

## Features

- **IPAM Tools**:
  - `solidserver_ip_create`: Allocate a new IP address (next free or specific).
  - `solidserver_ip_update`: Update an existing IP address allocation.
  - `solidserver_ip_delete`: Release an IP address.
  - `solidserver_ip_find_free`: Find available free IP addresses in a subnet.
  - `solidserver_ip_list`: List and filter IP addresses in a space.
- **Space Tools**:
  - `solidserver_space_list`: List available IPAM spaces.
  - `solidserver_space_create`: Create a new IPAM space.
  - `solidserver_space_delete`: Delete an IPAM space.
- **Subnet Tools**:
  - `solidserver_subnet_list`: List and filter subnets in a space.
  - `solidserver_subnet_info`: Get detailed information for a specific subnet.
  - `solidserver_subnet_create`: Create a new subnet within a space.
  - `solidserver_subnet_delete`: Delete a specific subnet from a space.
- **DNS Tools**:
  - `solidserver_dns_record_create`: Create A, AAAA, CNAME, and other records.
  - `solidserver_dns_record_update`: Update an existing DNS resource record.
  - `solidserver_dns_record_delete`: Delete DNS records.
  - `solidserver_dns_record_list`: List and filter DNS resource records.
  - `solidserver_dns_zone_list`: List DNS zones.
  - `solidserver_dns_zone_create`: Create a new DNS zone.
  - `solidserver_dns_zone_delete`: Delete a DNS zone.
- **VLAN Tools**:
  - `solidserver_vlan_domain_list`: List VLAN domains.
  - `solidserver_vlan_domain_create`: Create a new VLAN domain.
  - `solidserver_vlan_domain_delete`: Delete a VLAN domain.
  - `solidserver_vlan_list`: List and filter VLANs.
  - `solidserver_vlan_create`: Create a new VLAN.
  - `solidserver_vlan_delete`: Delete a specific VLAN.
- **DHCP Tools**:
  - `solidserver_dhcp_server_list`: List DHCP servers.
  - `solidserver_dhcp_scope_list`: List DHCP scopes.
  - `solidserver_dhcp_scope_create`: Create a new DHCP scope.
  - `solidserver_dhcp_range_list`: List DHCP ranges.
  - `solidserver_dhcp_range_create`: Create a new DHCP range within a scope.
  - `solidserver_dhcp_lease_list`: List DHCP leases.
  - `solidserver_dhcp_static_add`: Add a static DHCP reservation.
  - `solidserver_dhcp_static_delete`: Delete a static DHCP reservation.
- **Diagnostics**:
  - `solidserver_doctor`: Run preflight diagnostic checks against the appliance (DNS resolution, network reachability, TLS handshake, and API authentication).

### Resources and prompts

Beyond tools, the server exposes read-only MCP **resources** so a client can pull
inventory without a tool call: `solidserver://spaces`, `solidserver://dns/zones`,
`solidserver://vlan/domains`, and `solidserver://dhcp/servers`, plus templated
resources for a single subnet (`solidserver://subnets/{id}`), the records in a zone
(`solidserver://dns/zones/{zone}/records`), and the VLANs in a domain
(`solidserver://vlan/domains/{domain}/vlans`).

It also ships guided **prompts** for common DDI workflows: `solidserver_provision_host`,
`solidserver_decommission_host`, `solidserver_audit_subnet`, and
`solidserver_plan_vlan_subnet`.

### Guardrails

Mutating tools honour a set of safety controls: a global read-only mode
(`SOLIDSERVER_READ_ONLY`) that rejects every write, and per-object protection lists
(`SOLIDSERVER_PROTECTED_SPACES`, `SOLIDSERVER_PROTECTED_ZONES`,
`SOLIDSERVER_PROTECTED_SUBNETS`) that refuse mutations against named spaces, zones,
or subnets. See [Configuration](#configuration) for the full list.

### Output and error contract

- **Untrusted-data fencing**: SOLIDserver free-text fields (record comments, object names, descriptions, class metadata, appliance error messages) are writable by anyone who can create or edit an object, so they are a prompt-injection surface. Appliance-derived tool output the model reads as text is wrapped in an explicit `<untrusted-data source="solidserver">` envelope with a note telling the model to treat the contents as data, not instructions. Note that the typed structured output the SDK returns alongside the text is clean JSON and is NOT fenced (fencing would make it unparseable); it carries the same appliance free-text, so a client that feeds structured content to a model should treat it as untrusted too.
- **Guaranteed arrays**: list tools always serialize `data` as a JSON array, never `null`, even for an empty result.
- **Pagination signals**: list results carry `has_more` and `next_offset`. The appliance does not return a total count, so `has_more` is a heuristic (a page that fills the requested `limit` is reported as possibly having more); a final page of exactly `limit` items reports `has_more: true`, and the next page returns `count: 0`. If the appliance enforces a page cap below the requested `limit`, `has_more` can be a false negative, so keep `limit` at or below the appliance's page size.
- **Structured errors**: API failures are returned as a JSON object with `message` plus, when available, `status`, `errno`, `errmsg`, and `hint`, so a client can branch on fields instead of parsing prose. The `message` is always present and carries the full human-readable summary with a remediation hint.

## Configuration

The server is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SOLIDSERVER_HOST` | Hostname or IP of the SolidServer appliance | Required |
| `SOLIDSERVER_TOKEN_ID` | API token ID | Required |
| `SOLIDSERVER_TOKEN_ID_FILE` | Path to a file holding the token ID (alternative to `SOLIDSERVER_TOKEN_ID`) | |
| `SOLIDSERVER_TOKEN_SECRET` | API token secret | Required |
| `SOLIDSERVER_TOKEN_SECRET_FILE` | Path to a file holding the token secret (alternative to `SOLIDSERVER_TOKEN_SECRET`) | |
| `SOLIDSERVER_SSL_VERIFY` | Verify the appliance TLS certificate | `true` |
| `SOLIDSERVER_TRANSPORT` | Transport mode (`stdio`, `http`, or `unix`) | `stdio` |
| `SOLIDSERVER_LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |
| `SOLIDSERVER_HTTP_HOST` | Host/IP to bind the HTTP server (http transport) | `127.0.0.1` |
| `SOLIDSERVER_HTTP_PORT` | Port for HTTP transport | `8080` |
| `SOLIDSERVER_SOCKET` | Absolute Unix socket path (unix transport) | `$XDG_RUNTIME_DIR/solidserver-mcp.sock`, else `/tmp/solidserver-mcp.sock` |
| `SOLIDSERVER_READ_ONLY` | Reject every mutating tool when `true` | `false` |
| `SOLIDSERVER_PROTECTED_SPACES` | Comma-separated space names that refuse mutation | |
| `SOLIDSERVER_PROTECTED_ZONES` | Comma-separated DNS zones that refuse mutation | |
| `SOLIDSERVER_PROTECTED_SUBNETS` | Comma-separated subnets that refuse mutation | |
| `SOLIDSERVER_HTTP_TIMEOUT` | Per-request timeout to the appliance (e.g. `30s`, `1m`) | `30s` |
| `SOLIDSERVER_MAX_RETRIES` | Retry attempts for transient appliance failures | `3` |
| `SOLIDSERVER_RATE_LIMIT` | Client-side request rate cap in requests/sec (`0` = unlimited) | `0` |
| `SOLIDSERVER_LOG_REDACT_PII` | Redact PII from logs | `false` |

An unset optional variable falls back to its default. A value that is set but
malformed (an unknown transport or log level, a non-boolean `SOLIDSERVER_SSL_VERIFY`,
a non-numeric or out-of-range `SOLIDSERVER_HTTP_PORT`, a non-absolute or over-long
`SOLIDSERVER_SOCKET`) is rejected at startup with an error on stderr, so a typo
cannot be silently ignored.

## Usage

### Stdio Mode (Standard)

Designed for use with Claude Desktop, Cursor, and other local MCP clients.

```json
{
  "mcpServers": {
    "solidserver": {
      "command": "/path/to/solidserver-mcp",
      "env": {
        "SOLIDSERVER_HOST": "sds.example.com",
        "SOLIDSERVER_TOKEN_ID": "yourtokenid",
        "SOLIDSERVER_TOKEN_SECRET": "yourtokensecret"
      }
    }
  }
}
```

### HTTP Mode

For remote deployment or shared contexts.

```bash
export SOLIDSERVER_TRANSPORT=http
export SOLIDSERVER_HOST=sds.example.com
export SOLIDSERVER_TOKEN_ID=yourtokenid
export SOLIDSERVER_TOKEN_SECRET=yourtokensecret
./solidserver-mcp
```

### Unix Socket Mode

For local clients that prefer a filesystem socket over a TCP port. The socket
path defaults to `$XDG_RUNTIME_DIR/solidserver-mcp.sock` (or `/tmp` when that is
unset); override it with an absolute `SOLIDSERVER_SOCKET`.

```bash
export SOLIDSERVER_TRANSPORT=unix
export SOLIDSERVER_SOCKET=/run/solidserver-mcp.sock
export SOLIDSERVER_HOST=sds.example.com
export SOLIDSERVER_TOKEN_ID=yourtokenid
export SOLIDSERVER_TOKEN_SECRET=yourtokensecret
./solidserver-mcp
```

## Development

Requires Go 1.27.

- **Check** (fmt + vet + lint + test): `task check`
- **Build**: `task go:build`
- **Lint**: `task go:lint`
- **Format**: `task go:fmt`
- **Tidy**: `task go:tidy`
- **Container image**: `task image:build` (override tool: `CONTAINER_TOOL=docker task image:build`)

## License

Apache-2.0
