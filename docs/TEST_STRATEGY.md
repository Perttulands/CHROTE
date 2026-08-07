# CHROTE test strategy

CHROTE combines a Go server, an embedded React dashboard, tmux/ttyd integration, host filesystem access, Beads, and optional local adapters. The test strategy follows those boundaries instead of pretending one giant end-to-end suite can prove everything.

## Supported toolchain

- Go 1.26.5
- Node.js 20.19+ or 22.12+
- npm
- Chromium through Playwright
- `govulncheck` v1.6.0

CI and release workflows must use the same Go baseline declared by `src/go.mod`.

## Quality contract

A change is green only when the relevant layers pass after the final edit:

1. documentation and release-truth checks;
2. dashboard build, lint, unit tests, dependency audit, and browser tests;
3. Go formatting, vet, tests, race tests, and coverage;
4. source and exact-binary vulnerability scans;
5. embedded-dashboard parity;
6. disposable installer smoke when packaging, versioning, installation, service, or release behavior changes.

Warnings are evidence. Do not suppress React lifecycle warnings, Go diagnostics, npm audit findings, or browser errors merely to make output quiet.

## Canonical local matrix

### Documentation

```bash
python3 scripts/doc-lint.py
python3 scripts/host-neutrality.py
git diff --check
```

`scripts/doc-lint.py` enforces stable public facts: source-truth frontmatter and routing, host-neutral product docs, shipped versus experimental view labels, version/toolchain parity, and required README media.

`scripts/host-neutrality.py` is the wider net over the same contract: it checks every tracked file rather than only Markdown, because real account names and socket paths reached shipped scripts and Go tests while a Markdown-only lint reported PASS.

Local links and images must resolve. Plans and archives are historical context and are not allowed to stand in for active product truth.

### Dashboard build and embedded assets

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
diff -qr dashboard/dist src/internal/dashboard/dist
```

The Go binary serves `src/internal/dashboard/dist` through `go:embed`. A clean `dashboard/dist` is not enough if the embedded copy is stale.

### Dashboard static, unit, and dependency checks

```bash
cd dashboard
npm run lint
npm run test:unit -- --coverage
npm audit --audit-level=moderate
```

Unit tests own component behavior, state normalization, API clients, routing, recovery helpers, and interaction contracts that do not need a real browser process.

### Deterministic browser tests

```bash
cd dashboard
npm test
```

The default Playwright suite starts a mocked Vite server. It must mock backend and terminal requests and must not depend on a live CHROTE instance.

If the default Vite port is already occupied, use another port rather than killing an unrelated process:

```bash
CHROTE_PLAYWRIGHT_PORT=5279 npm test
```

The default suite covers desktop and mobile layouts, terminal workspace persistence, Sessions/Files sidecars, Peek and location-chip behavior, drag/drop, Files, Beads, settings, destructive-action confirmation, and error states.

Experimental Formations browser specs are separate:

```bash
npm run test:formations
```

Their existence does not promote Formations into the supported release surface.

### Go checks

```bash
cd src
gofmt -w <changed-go-files>
go vet ./...
go test ./...
go test -race ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Go unit and package tests own API contracts, path authorization, tmux command construction, terminal proxy lifecycle, operator-triggered recovery, schedules, and experimental orchestration internals. Terminal lifecycle tests distinguish non-interference with external tmux work from the explicitly transient CHROTE-owned ttyd attach processes.

### Vulnerability and release-binary checks

```bash
cd src
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

version="$(tr -d '\r\n' < ../VERSION)"
go build \
  -trimpath \
  -ldflags "-X main.Version=$version" \
  -o /tmp/chrote-server \
  ./cmd/server
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 \
  -mode=binary /tmp/chrote-server
```

Source scanning and binary scanning prove different things. Releases require both.

### Operator tooling tests

```bash
python3 -m unittest discover -s scripts/tmux-recovery -p 'test_*.py'
```

These tests cover the operator-side recovery clients: manifest validation, owner rules, and the snapshot/restore/verify CLIs. They do not establish continuous supervision or host-reboot recovery.

### Disposable installer smoke

```bash
./scripts/test-public-install.sh /tmp/chrote-server
./scripts/test-public-install.sh
```

The first mode tests an exact prebuilt binary. The second builds from the checkout. Both run under temporary HOME/XDG roots and random loopback ports.

The smoke proves:

- managed files and the server user unit are written under the selected prefix;
- workspace paths with spaces and `%` survive environment and systemd quoting;
- `/api/health` reports the expected version;
- an isolated tmux session is discovered;
- the ttyd proxy responds;
- normal uninstall preserves workspace, state, and private overrides;
- explicit purge removes state and private overrides without deleting the workspace.

It validates each generated unit with `systemd-analyze`. It deliberately does not start or replace a real user service.

## Live backend integration

Live integration tests are operator-run because they need an explicitly approved CHROTE backend, tmux socket, and terminal proxy.

```bash
cd dashboard
CHROTE_TEST_URL=http://127.0.0.1:<approved-port> npm run test:live
```

Before a live run:

1. identify the exact approved endpoint and service;
2. inspect current health and existing tmux state;
3. create only isolated test-owned sessions and files;
4. avoid destructive or bulk-recovery actions;
5. clean up test-owned state;
6. verify the original service and tmux sessions remain healthy.

Never hard-code private hostnames, user homes, service identities, sockets, or deployment lanes into the tracked test strategy.

## Manual product smoke

A manual smoke should verify the user-visible contract rather than every control:

1. load the dashboard from an approved loopback/private endpoint;
2. confirm three terminal workspaces and one-to-four-window layouts;
3. attach a test-owned tmux session and reconnect the browser;
4. confirm Sessions-row **Peek** does not reassign it;
5. confirm the location chip navigates to its attached window;
6. confirm Sessions open/pin/width/filter/group-collapse remains global across terminal switches, including a consistent pinned rail and width on every tab while any workspace's Files panel is open; confirm Files state remains per workspace and narrow layouts overlay at `768px` and below;
7. inspect one configured file root and one configured Beads workspace;
8. verify optional integrations degrade clearly when unavailable;
9. inspect Server and recovery status without triggering destructive actions.

Formations is excluded from the supported product smoke until its release gate passes.

## CI and release ownership

The pull-request CI workflow runs the public quality contract on clean GitHub runners. The release workflow repeats release-critical checks, derives the binary version from the tag, scans both target binaries, runs the installer smoke against the release-smoke binary, and publishes only after those checks pass.

A local pass is evidence, not permission to bypass protected CI. A tag is not a release until its workflow is green and the published artifacts match the intended commit and version.
