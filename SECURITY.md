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
- The `/srv` proving lane runs from `/srv/chrote` with data under `/srv/data/chrote`, system unit `chrote-srv.service`, server port `8095`, and terminal proxy port `7686`.
- The legacy rollback lane runs from `/home/perttu/chrote`, user unit `chrote.service`, server port `8094`, and terminal proxy port `7683`.
- `API_AUTH_TOKEN` protects `/api/*` and `/terminal/*`; the dashboard exchanges the token for a time-limited Secure, HttpOnly, SameSite session cookie.
- Use a reverse proxy, Tailscale, or equivalent network controls before binding beyond localhost.

### Recommended Security Practices

1. **Keep localhost as the default** - Leave `HOST=127.0.0.1` unless the host network is explicitly trusted.
2. **Use Tailscale HTTPS for remote access** - Never expose CHROTE with Funnel or raw HTTP.
3. **Separate read and mutation roots** - `CHROTE_ROOTS=/` can provide broad browsing, while `CHROTE_WRITE_ROOTS` limits create, overwrite, rename, and delete.
4. **Run as a non-root user** - The systemd service should run as the owning Unix user, not root.
5. **Treat terminal access as host access** - tmux sessions are not a sandbox.

### Environment Variables

Sensitive configuration should use host-owned environment files or service environment, not browser state:

- `HOST` - Server bind address (default: `127.0.0.1`).
- `PORT` - Server port. The `/srv` proving lane sets `8095`; the legacy rollback lane uses `8094`.
- `TTYD_PORT` - Terminal proxy port. The `/srv` proving lane sets `7686`; the legacy rollback lane uses `7683`.
- `API_AUTH_TOKEN` - Owner token for API/terminal access and secure browser-session login; `/api/health` remains public.
- `CORS_ORIGINS` - Optional comma-separated browser origins for cross-origin API access.
- `CHROTE_ROOTS` - Comma-separated list of allowed filesystem roots.
- `CHROTE_WRITE_ROOTS` - Comma-separated roots where file mutations are allowed. Defaults to `CHROTE_ROOTS` for backward compatibility.
- `CHROTE_FILE_DENY_PATHS` - Additional sensitive roots blocked from browsing and mutation. Built-in credential and pseudo-filesystem exclusions always apply.
- `CHROTE_MAX_UPLOAD_BYTES` - Maximum file upload/write request size; defaults to 64 MiB.
- `CHROTE_WORKDIR` - Default working directory for launched sessions.

### Path Traversal Protection

The server resolves symlinks before revalidating paths, blocks sensitive credential and pseudo-filesystem locations, separates read roots from write roots, and bounds uploads. Attempts to access paths outside policy are rejected.

## Known Limitations

- No built-in HTTPS termination (use tailnet-only Tailscale Serve or another trusted reverse proxy)
- Terminal sessions are not isolated (tmux shared sessions)
