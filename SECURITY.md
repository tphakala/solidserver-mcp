# Security Policy

## Reporting a vulnerability

Please report security issues privately, not in a public issue or pull request.

Use GitHub's private vulnerability reporting: go to the [Security tab](https://github.com/tphakala/solidserver-mcp/security) and click **Report a vulnerability**. That opens a private advisory visible only to you and the maintainer.

Please include enough detail to reproduce: the version or commit, your transport and configuration (redact any tokens, hostnames, or IPs), and the impact you observed. A minimal proof of concept helps a lot.

You can expect an acknowledgement within a few days. Once a fix is ready, it ships in a patch release and the advisory is published with credit to you, unless you prefer to stay anonymous.

## Supported versions

This is a small, single-maintainer project. Security fixes land on the latest release line only; please upgrade to the newest `v1.x` release before reporting.

| Version | Supported |
|---------|-----------|
| Latest `v1.x` release | Yes |
| Older releases | No |

## Scope and hardening notes

A few things worth knowing when assessing or deploying this server:

- **Credentials.** The server authenticates to the SOLIDserver appliance with an API token supplied through `SOLIDSERVER_TOKEN_ID` / `SOLIDSERVER_TOKEN_SECRET`, or from files via the `*_FILE` variants (prefer the file form for Docker or Kubernetes secret mounts). Tokens are never written to logs; set `SOLIDSERVER_LOG_REDACT_PII=true` to also redact IP and MAC addresses from log output.
- **Untrusted appliance data.** Free-text fields returned by the appliance (record comments, object names, descriptions, error messages) are writable by anyone who can edit those objects, so they are a prompt-injection surface. Text the model reads is wrapped in an explicit `<untrusted-data>` envelope. The typed structured content returned alongside it is not fenced (fencing would make it unparseable), so a client that feeds structured output to a model should treat it as untrusted too.
- **Blast-radius controls.** For least privilege, scope the appliance API token to only what the deployment needs, run with `SOLIDSERVER_READ_ONLY=true` when writes are not required, and fence production objects with `SOLIDSERVER_PROTECTED_SPACES` / `SOLIDSERVER_PROTECTED_ZONES` / `SOLIDSERVER_PROTECTED_SUBNETS`.
- **Transport binding.** The HTTP transport binds to `127.0.0.1` by default. Only widen `SOLIDSERVER_HTTP_HOST` (for example to `0.0.0.0`) behind your own authentication and network controls; the server does not add an auth layer of its own. The unix socket transport keeps access to local filesystem permissions.
- **TLS.** Appliance TLS verification is on by default (`SOLIDSERVER_SSL_VERIFY=true`). Disable it only for a lab appliance with a self-signed certificate, never in production.
