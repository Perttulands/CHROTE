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
# Inside the CHROTE workspace
cd /path/to/chrote

# Build the dashboard
cd dashboard
npm install
npm run build
cp -r dist ../src/internal/dashboard/

# Build the server
cd ../src
go build -o ../chrote-server ./cmd/server

# Restart the user service to pick up changes
systemctl --user restart chrote.service
```

### Running Tests

```bash
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go test ./...
go vet ./...
go test -race ./...
go test -cover ./...

cd ../dashboard
npm ci
npm run lint
npm run build
npm run test:unit -- --coverage
npm audit --audit-level=moderate
npm test
```

## Code Style

- Go: Follow standard Go conventions (`gofmt`, `go vet`)
- TypeScript/React: `npm run lint`, `npm run build`, and `npm run test:unit -- --coverage` are the current static/unit gates.
- Commit messages: Use clear, descriptive messages

## Pull Request Process

1. Ensure the backend gates pass: `cd src && test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))" && go vet ./... && go test ./... && go test -race ./...`
2. Ensure the dashboard gates pass: `cd dashboard && npm run lint && npm run build && npm run test:unit -- --coverage && npm audit --audit-level=moderate && npm test`
3. Run backend coverage when changing backend behavior: `cd src && go test -cover ./...`
4. Update documentation if needed
5. Submit a pull request with a clear description of changes

## Reporting Issues

- Use GitHub Issues for bugs and feature requests
- Include steps to reproduce for bugs
- Check existing issues before creating duplicates

## Security

For security vulnerabilities, please see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
