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

CHROTE is designed for **local or trusted network use only**. By default:

- The server binds to `0.0.0.0` (all interfaces)
- No built-in authentication is provided
- Use a reverse proxy with authentication (Tailscale, nginx + auth, etc.) for production

### Recommended Security Practices

1. **Use Tailscale or similar** - CHROTE works well behind Tailscale for secure remote access
2. **Bind to localhost** - Set `CHROTE_HOST=127.0.0.1` if only local access is needed
3. **Restrict allowed roots** - Use `CHROTE_ROOTS` to limit filesystem access
4. **Run as non-root user** - The systemd service runs as your user, not root

### Environment Variables

Sensitive configuration should use environment variables, not config files:

- `CHROTE_ROOTS` - Comma-separated list of allowed filesystem roots
- `CHROTE_PORT` - Server port (default: 8080)
- `CHROTE_HOST` - Bind address (default: 0.0.0.0)

### Path Traversal Protection

The server validates all file paths against configured allowed roots. Attempts to access paths outside allowed roots are blocked and logged.

## Known Limitations

- No built-in HTTPS (use a reverse proxy)
- No built-in authentication (rely on network-level security)
- Terminal sessions are not isolated (tmux shared sessions)
