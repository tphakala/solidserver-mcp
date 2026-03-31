# SolidServer MCP Project Plan

This document outlines the architecture and detailed implementation plan for the **SolidServer MCP (Model Context Protocol)** server. It aims to integrate EfficientIP SolidServer IPAM/DNS functionalities into AI contexts using the `github.com/efficientip-labs/solidserver-go-client/sdsclient` Go SDK.

## 1. Project Overview

**Goal:** Create a robust, production-ready Go 1.26 MCP server that securely exposes EfficientIP SolidServer functionalities (IPAM, DNS management) to AI agents.
**Inspiration:** Based heavily on the architecture of `autotask-mcp-go`.

## 2. Directory Structure & Architecture

```text
solidserver-mcp/
├── main.go               # Application entry point; sets up logging and handles signals.
├── config.go             # Configuration definition and env var parsing.
├── server.go             # MCP server initialization, tool registration, and HTTP/Stdio transport handlers.
├── go.mod                # Go module definition (Go 1.26)
├── go.sum                # Go module checksums
├── .golangci.yaml        # Linter configuration (strict)
├── rules/                # Custom ruleguard rules for golangci-lint
├── Dockerfile            # Containerization (multi-stage build)
├── README.md             # Documentation
├── services/
│   └── client.go         # Wrapper/helper for solidserver-go-client initialization and error handling.
└── tools/
    ├── tools.go          # Central tool registration loop (RegisterAll).
    ├── ipam_tools.go     # Handlers for IP address management tools.
    ├── subnet_tools.go   # Handlers for Subnet/Space management tools.
    └── dns_tools.go      # Handlers for DNS management tools.
```

## 3. Detailed Implementation Steps

### Phase 1: Foundation, Configuration & Build System
1. **Initialize Go Module:** 
   - `go mod init github.com/tphakala/solidserver-mcp` (Targeting Go 1.26)
   - Add dependencies: `github.com/modelcontextprotocol/go-sdk/mcp`, `github.com/efficientip-labs/solidserver-go-client/sdsclient`.
2. **Setup Build System:**
   - Create a `Taskfile.yml` to define build, test, and linting tasks (using `go-task`).
3. **Setup Linting:** 
   - Ensure the `.golangci.yaml` and `rules/` directory are actively used during development, integrated into the Taskfile.
4. **Define Configuration (`config.go`):**
   - Create a `Config` struct:
     ```go
     type Config struct {
         Host       string // SOLIDSERVER_HOST
         Username   string // SOLIDSERVER_USERNAME
         Password   string // SOLIDSERVER_PASSWORD
         SSLVerify  bool   // SOLIDSERVER_SSL_VERIFY (default: true)
         Transport  string // MCP_TRANSPORT (stdio or http)
         HTTPPort   int    // MCP_HTTP_PORT
         LogLevel   string // LOG_LEVEL (debug, info, warn, error)
     }
     ```
   - Implement `LoadConfig()` to parse these from environment variables.

### Phase 2: Client Setup & Server Core
1. **Initialize SDK Client (`services/client.go`):**
   - Implement a factory function `NewSolidServerClient(cfg Config) (*solidserver.Client, error)` that initializes the `solidserver.NewClient()` using the provided credentials.
   - Setup a wrapper to handle standard EfficientIP API errors and map them to human-readable strings for the LLM.
2. **Setup MCP Server Core (`server.go`):**
   - Define `buildServer(client *solidserver.Client) *mcp.Server`.
   - Setup the `mcp.ServerOptions` with clear instructions describing what the SolidServer MCP does.
   - Implement `run(ctx, cfg, logger)` to route to either `runStdio()` or `runHTTP()`.
   - **runStdio():** Attach the server to stdin/stdout (disabling standard logging to stdout to preserve the JSON-RPC channel).
   - **runHTTP():** Setup SSE (Server-Sent Events) via `mcp.NewStreamableHTTPHandler` for remote connection scenarios.

### Phase 3: Tool Implementation (`tools/`)
All tools will use the `github.com/modelcontextprotocol/go-sdk/mcp` structure. `tools.go` will contain a `RegisterAll(s *mcp.Server, client *solidserver.Client)` function.

#### IPAM Tools (`tools/ipam_tools.go`)
- **`solidserver_ip_create`**: 
  - **Params:** `space` (string), `subnet` (string), `name` (string, optional), `mac` (string, optional).
  - **Action:** Requests a new IP address allocation from a specified subnet.
- **`solidserver_ip_delete`**: 
  - **Params:** `ip_address` (string), `space` (string).
  - **Action:** Releases/deletes a specified IP address.
- **`solidserver_ip_find_free`**:
  - **Params:** `space` (string), `subnet` (string), `limit` (int).
  - **Action:** Returns a list of available free IP addresses in a subnet without allocating them.

#### Subnet Tools (`tools/subnet_tools.go`)
- **`solidserver_subnet_list`**:
  - **Params:** `space` (string, optional), `keyword` (string, optional).
  - **Action:** Searches for subnets matching the keyword within a space.
- **`solidserver_subnet_info`**:
  - **Params:** `name` or `subnet_address` (string).
  - **Action:** Returns detailed utilization and configuration info for a specific subnet.

#### DNS Tools (`tools/dns_tools.go`)
- **`solidserver_dns_record_create`**:
  - **Params:** `name` (string), `type` (A, AAAA, CNAME), `value` (string), `ttl` (int).
  - **Action:** Creates a new DNS record.
- **`solidserver_dns_record_delete`**:
  - **Params:** `name` (string), `type` (string).
  - **Action:** Deletes a specific DNS record.
- **`solidserver_dns_record_search`**:
  - **Params:** `name` (string).
  - **Action:** Looks up existing DNS records matching the name.

### Phase 4: Error Handling & Logging
- **Strict Logging Isolation:** Ensure all `slog` output goes to `os.Stderr` when the server is in `stdio` mode to prevent breaking the MCP JSON-RPC protocol.
- **Graceful Shutdown:** Capture `SIGINT` and `SIGTERM` in `main.go` using `signal.NotifyContext`, passing the cancellable context down to the server handlers.

### Phase 5: Containerization & Documentation
- **`Dockerfile` / `Containerfile`:**
  - Create a Podman-compatible multi-stage build.
  ```dockerfile
  FROM golang:1.26 AS builder
  # Build process...
  FROM alpine:latest
  # Copy binary and set entrypoint
  ```
- **`.env.example`:** Provide a template for users.
- **`README.md`:** Document setup, configuration, and integration steps for common MCP clients (Claude Desktop, Cursor, etc.).

## 4. Execution Plan
1. `go mod init` & `go mod tidy`
2. Implement `config.go` & `main.go`
3. Implement `server.go` (transport & lifecycle)
4. Implement `services/client.go`
5. Iteratively build `tools/ipam_tools.go`, `tools/subnet_tools.go`, `tools/dns_tools.go`
6. Finalize `Dockerfile` & `README.md`
