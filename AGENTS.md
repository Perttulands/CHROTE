# CHROTE

CHROTE is a browser-based agentic IDE for one trusted operator. It makes tmux-hosted terminal sessions easy to run, arrange, observe, and steer from any device on the operator's private network.

## Project map

- `dashboard/src/` owns the React user interface and device-local workspace presentation.
- `src/cmd/server/` assembles and starts the Go server.
- `src/internal/api/` exposes host resources and component APIs to the browser.
- `src/internal/proxy/` owns terminal transport and ttyd lifecycle.
- `src/internal/dashboard/` embeds the built dashboard served by the Go binary.
- `scripts/` owns the canonical build, validation, installation, and source-contract entrypoints.
- `docs/` contains durable product, architecture, user, and maintainer documentation.

Read `PRD.md` for the durable product contract and `SECURITY.md` for the trust and access boundary. Use Beads, never Markdown, for roadmap, status, dependencies, or outstanding work.

## Golden invariants

tmux owns live sessions. CHROTE, its tests, development tools, deployments, browser disconnects, and service restarts must never kill or disrupt tmux sessions. Session termination is an explicit operator action.

Broad access within configured roots is intentional. Rely on Unix permissions and report access failures plainly; do not narrow ownership, modes, ACLs, or configured roots as speculative hardening.

Tracked source and documentation stay host-neutral. Keep real deployment paths, ports, sockets, service identities, credentials, transcripts, and recovery procedures in private operator configuration.

Use `./scripts/build-embedded-dashboard.sh` for the embedded dashboard. Do not hand-copy build output; verify it with `python3 scripts/check-embedded-dashboard.py`.

## Work state

Run `bd` from this repository root so work resolves to CHROTE's `.beads` store and `chrote-` prefix. Use the shared Beads skills for drafting, review, execution, and discovered work.

Execute only the active Bead. Record unrelated findings as linked Beads instead of fixing them in place.

## Git discipline

Keep `main` current, clean, tested, and deployable. Do ordinary work directly there and commit verified increments promptly. Use a branch or worktree only when isolation materially helps; merge verified work back to `main` immediately, then remove the temporary state. Build and deploy from `main`.

Stage only files owned by the active Bead. Preserve unrelated work already present in the tree.

## Validation

Run the gates relevant to the changed area:

```bash
# Embedded product build
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
python3 scripts/check-embedded-dashboard.py

# Go server
cd src
GOTOOLCHAIN=go1.26.6 go vet ./...
GOTOOLCHAIN=go1.26.6 go test ./...
GOTOOLCHAIN=go1.26.6 go test -race ./...

# Dashboard
cd dashboard
npm run lint
npm run test:unit -- --coverage
npm test

# Documentation and patch hygiene
python3 scripts/doc-lint.py
git diff --check
```

Installer changes also run both modes of `scripts/test-public-install.sh`. Runtime deployment is separate from repository verification and requires the operator-approved local target.
