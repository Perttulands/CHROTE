# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately:

1. **Do NOT** create a public GitHub issue
2. Email the maintainer or use GitHub's private vulnerability reporting feature
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will respond within 48 hours and work with you to understand and address the issue.

## Security Considerations

### Network Exposure

CHROTE is designed for **local or explicitly trusted network use only**. By default:

- The server binds to `127.0.0.1`.
- The server port is `8094`.
- The terminal proxy port is `7683`.
- `/api/*` authentication is optional through `API_AUTH_TOKEN`; if unset, rely on localhost/Tailscale/reverse-proxy boundaries.
- Use a reverse proxy, Tailscale, or equivalent network controls before binding beyond localhost.

### Recommended Security Practices

1. **Keep localhost as the default** - Leave `HOST=127.0.0.1` unless the host network is explicitly trusted.
2. **Use Tailscale or similar for remote access** - CHROTE works well behind private-network controls.
3. **Restrict allowed roots** - Use `CHROTE_ROOTS` to limit filesystem access.
4. **Run as a non-root user** - The systemd service should run as the owning Unix user, not root.
5. **Treat terminal access as host access** - tmux sessions are not a sandbox.

### Environment Variables

Sensitive configuration should use host-owned environment files or service environment, not browser state:

- `HOST` - Server bind address (default: `127.0.0.1`).
- `PORT` - Server port (default: `8094`).
- `TTYD_PORT` - Terminal proxy port (default: `7683`).
- `API_AUTH_TOKEN` - Optional bearer token for `/api/*` routes except `/api/health`.
- `CORS_ORIGINS` - Optional comma-separated browser origins for cross-origin API access.
- `CHROTE_ROOTS` - Comma-separated list of allowed filesystem roots.
- `CHROTE_WORKDIR` - Default working directory for launched sessions.

### Path Traversal Protection

The server validates all file paths against configured allowed roots. Attempts to access paths outside allowed roots are blocked and logged.

## Known Limitations

- No built-in HTTPS (use a reverse proxy)
- No built-in authentication (rely on network-level security)
- Terminal sessions are not isolated (tmux shared sessions)
