# Contributing to CHROTE

CHROTE is private-infrastructure software with terminal-grade reach. Small,
reviewable, verified changes beat ambitious mush.

## Before changing code

1. Read [`AGENTS.md`](AGENTS.md) for repository working rules.
2. Read [`docs/source-truth-index.md`](docs/source-truth-index.md) and the spec
   that owns the behavior you plan to change.
3. Inspect nearby callers, state owners, tests, and existing UI patterns.
4. Define done before editing. If behavior changes, start with a failing test.

Do not mix unrelated cleanup, host-local deployment configuration, or private
operator data into a product change.

## Development prerequisites

- Go 1.26.5+
- Node.js 20.19+ or 22.12+
- npm
- Python 3 for documentation and recovery-tool checks
- tmux and ttyd only for approved live terminal integration work

## Setup

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE

cd dashboard
npm ci
cd ../src
go mod download
```

## Stable local gates

Run the narrow test first while developing, then the relevant broader gates.
Before opening a pull request, reproduce the repository contract:

```bash
# Documentation and source-truth contract
python3 scripts/doc-lint.py

# Host neutrality: no deployment specifics in tracked files (all file types, not just Markdown)
python3 scripts/host-neutrality.py

# Dashboard
cd dashboard
npm run lint
npm run test:unit -- --coverage
npm audit --audit-level=moderate
npm test -- --project=chromium
cd ..

# Build the exact dashboard embedded by Go
./scripts/build-embedded-dashboard.sh
diff -qr dashboard/dist src/internal/dashboard/dist

# Go
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
go build -o /tmp/chrote-server-contributor ./cmd/server
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 -mode=binary /tmp/chrote-server-contributor
```

If you changed workload-recovery tooling, also run its Python test suite from the
repository root:

```bash
python3 -m unittest discover -s scripts/tmux-recovery -p 'test_*.py'
```

Live browser/terminal tests are separate because they operate an actual CHROTE
backend and tmux substrate. Run them only against an approved disposable or
operator-controlled instance.

## Documentation rules

- `PRD.md` owns the current product and roadmap boundary.
- `FORMATIONS.md`, `ARCHON.md`, `DATA-MODEL.md`, and `DESIGN-SYSTEM.md` own their
  declared contracts.
- Public docs describe generic supported behavior, not one maintainer's service
  names, home paths, sockets, ports, or rollback layout.
- Archive material is context, not authority.
- README prose should sound like CHROTE, not generated launch copy.
- Public screenshots must contain no terminal transcripts, credentials, private
  paths, personal usernames, content belonging to a second local account, or
  sensitive issue/session names.

## Pull requests

1. Branch from current `main`.
2. Keep the diff focused.
3. Add tests that prove changed behavior.
4. Run the relevant gates after the final edit.
5. Check `git diff --check` and `git status`.
6. Describe the operator outcome, important boundaries, and exact verification.
7. Use the protected pull-request flow; do not force-push public `main`.

Never weaken or skip a failing test to make CI green. Fix the behavior, fix an
incorrect test with evidence, or report the blocker plainly.

## Reporting bugs

Include:

- expected and actual behavior;
- CHROTE commit/version and browser;
- minimal reproduction steps;
- relevant logs with secrets, private paths, terminal contents, and identities
  removed;
- screenshots only when sanitized.

Use the private security-advisory path for vulnerabilities rather than posting
exploit details publicly.
