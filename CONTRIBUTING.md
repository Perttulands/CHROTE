# Contributing to CHROTE

Thank you for your interest in contributing to CHROTE!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/CHROTE.git`
3. Create a branch: `git checkout -b feature/your-feature-name`

## Development Setup

### Quick Start (Recommended)

The easiest way to get a development environment:

```powershell
# From PowerShell in the CHROTE directory
.\Chrote-Toggle.ps1 -Setup
```

This sets up everything automatically. See [README.md](README.md) for details.

### Manual Setup / Prerequisites

If setting up manually:
- Go 1.23+
- Node.js 20+
- WSL2 (for Windows) or Linux
- tmux
- ttyd

### Building from Source

```bash
# Inside WSL as chrote user
cd /code

# Build the dashboard
cd dashboard
npm install
npm run build
cp -r dist ../src/internal/dashboard/

# Build the server
cd ../src
go build -o ../chrote-server ./cmd/server

# Restart the service to pick up changes
sudo systemctl restart chrote-server
```

### Running Tests

```bash
cd src
go test ./...
```

## Code Style

- Go: Follow standard Go conventions (`gofmt`, `go vet`)
- TypeScript/React: ESLint with the project's configuration
- Commit messages: Use clear, descriptive messages

## Pull Request Process

1. Ensure tests pass: `go test ./...`
2. Update documentation if needed
3. Submit a pull request with a clear description of changes

## Reporting Issues

- Use GitHub Issues for bugs and feature requests
- Include steps to reproduce for bugs
- Check existing issues before creating duplicates

## Security

For security vulnerabilities, please see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
