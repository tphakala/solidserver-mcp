# SolidServer MCP Server

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
  - `solidserver_space_list`: List available IPAM spaces.
- **DNS Tools**:
  - `solidserver_dns_record_create`: Create A, AAAA, CNAME, and other records.
  - `solidserver_dns_record_delete`: Delete DNS records.
  - `solidserver_dns_record_list`: List and filter DNS resource records.
  - `solidserver_dns_zone_list`: List DNS zones.

## Configuration

The server is configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SOLIDSERVER_HOST` | Hostname or IP of the SolidServer | Required |
| `SOLIDSERVER_USERNAME` | API Username | Required |
| `SOLIDSERVER_PASSWORD` | API Password | Required |
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
        "SOLIDSERVER_USERNAME": "admin",
        "SOLIDSERVER_PASSWORD": "yourpassword"
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

