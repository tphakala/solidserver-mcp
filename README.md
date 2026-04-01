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

  <br>

  <!-- Code Quality -->
  <a href="https://golang.org">
    <img src="https://img.shields.io/badge/Built%20with-Go-teal?style=flat-square&logo=go">
  </a>
  <a href="https://goreportcard.com/report/github.com/tphakala/solidserver-mcp">
    <img src="https://goreportcard.com/badge/github.com/tphakala/solidserver-mcp?style=flat-square">
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

## Features

- **IPAM Tools**:
  - `solidserver_ip_create`: Allocate a new IP address (next free or specific).
  - `solidserver_ip_delete`: Release an IP address.
  - `solidserver_ip_find_free`: Find available free IP addresses in a subnet.
  - `solidserver_ip_list`: List and filter IP addresses in a space.
- **Subnet Tools**:
  - `solidserver_subnet_list`: List and filter subnets in a space.
  - `solidserver_subnet_info`: Get detailed information for a specific subnet.
  - `solidserver_subnet_create`: Create a new subnet within a space.
  - `solidserver_subnet_delete`: Delete a specific subnet from a space.
  - `solidserver_space_list`: List available IPAM spaces.
- **DNS Tools**:
  - `solidserver_dns_record_create`: Create A, AAAA, CNAME, and other records.
  - `solidserver_dns_record_delete`: Delete DNS records.
  - `solidserver_dns_record_list`: List and filter DNS resource records.
  - `solidserver_dns_zone_list`: List DNS zones.
- **VLAN Tools**:
  - `solidserver_vlan_domain_list`: List VLAN domains.
  - `solidserver_vlan_list`: List and filter VLANs.
  - `solidserver_vlan_create`: Create a new VLAN.
  - `solidserver_vlan_delete`: Delete a specific VLAN.
- **DHCP Tools**:
  - `solidserver_dhcp_server_list`: List DHCP servers.
  - `solidserver_dhcp_scope_list`: List DHCP scopes.
  - `solidserver_dhcp_range_list`: List DHCP ranges.
  - `solidserver_dhcp_lease_list`: List DHCP leases.
  - `solidserver_dhcp_static_add`: Add a static DHCP reservation.
  - `solidserver_dhcp_static_delete`: Delete a static DHCP reservation.

## Configuration

The server is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SOLIDSERVER_HOST` | Hostname or IP of the SolidServer | Required |
| `SOLIDSERVER_TOKEN_ID` | API Token ID | Required |
| `SOLIDSERVER_TOKEN_SECRET` | API Token Secret | Required |
| `SOLIDSERVER_SSL_VERIFY` | Verify SSL certificate | `true` |
| `MCP_TRANSPORT` | Transport mode (`stdio` or `http`) | `stdio` |
| `MCP_HTTP_HOST` | Host/IP to bind HTTP server | `localhost` |
| `MCP_HTTP_PORT` | Port for HTTP transport | `8080` |
| `LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |

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
export MCP_TRANSPORT=http
export SOLIDSERVER_HOST=sds.example.com
export SOLIDSERVER_TOKEN_ID=yourtokenid
export SOLIDSERVER_TOKEN_SECRET=yourtokensecret
./solidserver-mcp
```

## Development

Requires Go 1.26.

- **Build**: `task build`
- **Lint**: `task lint`
- **Tidy**: `task tidy`
- **Docker/Podman Build**: `task docker-build`

## License

Apache-2.0
