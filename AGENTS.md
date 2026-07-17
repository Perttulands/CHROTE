# Agent instructions

Use these rules for work in the CHROTE repository. Keep public instructions host-neutral; deployment-specific paths, ports, sockets, service identities, and credentials belong in untracked operator configuration.

## Product boundary

CHROTE is a private browser cockpit for host-owned work:

- durable tmux sessions and browser terminal windows;
- files under configured roots;
- Beads through modern `bd`;
- harness-neutral agent observability;
- recovery, scheduling, server status, and optional local adapters.

CHROTE is not a hosted service, an IDE, or an OS sandbox. Do not assume Gastown, Ralph, or any single agent harness is installed.

Formations and Archon are experimental and unreleased. Their active specs define development contracts, not a supported release promise.

## Source truth

Read [`docs/source-truth-index.md`](docs/source-truth-index.md) before changing product or architecture claims.

- [`PRD.md`](PRD.md) owns the current product and roadmap boundary.
- [`SECURITY.md`](SECURITY.md) owns the public trust boundary.
- [`FORMATIONS.md`](FORMATIONS.md), [`ARCHON.md`](ARCHON.md), and [`DATA-MODEL.md`](DATA-MODEL.md) own experimental orchestration contracts.
- Plans and archives are context, not current authority.

## Work state

Use `bd` for durable task state in the owning workspace:

```bash
bd prime
bd ready --json
bd show <id>
bd update <id> --claim --json
```

Create discovered work as linked Beads rather than burying it in prose or unrelated changes.

## Before editing

- Read nearby code, callers, tests, and the active source-truth document.
- Define done before changing files.
- Preserve unrelated dirty work.
- Do not kill, rename, or restart tmux sessions unless the task explicitly requires it.
- Do not assume a service name, port, socket, or deployment lane from tracked files. Discover the approved target from local operator configuration before runtime actions.
- Never commit private topology, credentials, terminal transcripts, or operator-specific recovery procedures.

## Build and verify

Use the repository scripts instead of hand-copying embedded assets:

```bash
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh

cd src
GOTOOLCHAIN=go1.26.5 go vet ./...
GOTOOLCHAIN=go1.26.5 go test ./...
GOTOOLCHAIN=go1.26.5 go test -race ./...

cd ../dashboard
npm run lint
npm run test:unit -- --coverage
npm test

cd ..
python3 scripts/doc-lint.py
```

Set `CHROTE_PLAYWRIGHT_PORT` when the default Vite test port is already owned by another process. Do not kill an unrelated listener merely to run tests.

For installer changes, run both disposable modes:

```bash
./scripts/test-public-install.sh /path/to/chrote-server
./scripts/test-public-install.sh
```

Before completion, run `git diff --check`, compare `dashboard/dist` with `src/internal/dashboard/dist`, inspect every touched repository, and report warnings or skipped checks plainly.

## Runtime actions

Runtime deployment is separate from repository verification. Before restarting or installing anything:

1. identify the exact operator-approved service and endpoint;
2. inspect current status and health;
3. preserve tmux sessions and unrelated work;
4. restart only the intended service;
5. verify the live endpoint after activation.

If the runtime target is not discoverable from local, untracked operator context, ask instead of guessing.
