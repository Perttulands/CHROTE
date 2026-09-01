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

- Go 1.26.6+
- Node.js 20.19+ or 22.12+
- npm
- Python 3 for documentation checks
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

Run the narrow test first while developing. Before opening a pull request,
reproduce the single CI quality job:

```bash
# Documentation and source-truth contract
python3 scripts/doc-lint.py

# Host neutrality: no deployment specifics in tracked files (all file types, not just Markdown)
python3 scripts/host-neutrality.py

# Build the assets and server CI tests exercise
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

# Dashboard unit, lint, and mocked browser journeys
cd dashboard
npm run test:unit
npm run lint
npm test
cd ..

# Go format, vet, and one race pass
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test -race ./...
go build -trimpath -o ../chrote-server-ci ./cmd/server
cd ..

# Source and built-server contracts
python3 scripts/check-embedded-dashboard.py
CHROTE_SERVER_BINARY="$PWD/chrote-server-ci" ./scripts/test-built-server-contract.sh
```

Live browser/terminal tests are separate because they operate an actual CHROTE
backend and tmux substrate. Run them only against an approved disposable or
operator-controlled instance.

`govulncheck` and `npm audit --audit-level=moderate` run on the weekly CI
schedule rather than every push.

## Documentation rules

- `VISION.md` owns product intent, `PRD.md` owns durable requirements, and
  `ARCHITECTURE.md` owns system structure and state ownership.
- `DESIGN-SYSTEM.md` owns dashboard visual and interaction contracts.
- Public docs describe generic supported behavior, not one maintainer's service
  names, home paths, sockets, ports, or rollback layout.
- `docs/legacy-ideas.md` is non-current context, never roadmap authority.
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
