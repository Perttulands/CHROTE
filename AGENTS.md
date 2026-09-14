# CHROTE

CHROTE is a browser-based agentic IDE for one trusted operator. It makes tmux-hosted terminal sessions easy to run, arrange, observe, and steer from any device on the operator's private network.

## Project map

- `dashboard/src/` owns the React user interface and device-local workspace presentation.
- `src/cmd/server/` assembles and starts the Go server.
- `src/internal/api/` exposes host resources and component APIs to the browser.
- `src/internal/proxy/` owns terminal transport and the pseudo-terminals it attaches on.
- `src/internal/dashboard/` embeds the built dashboard served by the Go binary.
- `scripts/` owns the canonical build, validation, installation, and source-contract entrypoints.
- `docs/` contains durable product, architecture, user, and maintainer documentation.

Read `VISION.md` for product intent, `PRD.md` for the durable product contract, `ARCHITECTURE.md` for system ownership and boundaries, and `SECURITY.md` for the trust boundary. Use Beads, never Markdown, for roadmap, status, dependencies, or outstanding work.

## Golden invariants

tmux owns live sessions. CHROTE, its tests, development tools, deployments, browser disconnects, and service restarts must never implicitly or accidentally terminate or disrupt existing tmux sessions. Exact operator-authorized deletion and exact cleanup of test-owned or failed-creation-owned sessions are allowed.

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

`docs/TEST_STRATEGY.md` states the rule a test must satisfy to earn its place, which layer owns which behaviour, and how these gates are split across CI; read it before adding or deleting a test.

Run the gates relevant to the changed area:

```bash
# Embedded product build
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
python3 scripts/check-embedded-dashboard.py

# Go server
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
GOTOOLCHAIN=go1.26.6 go vet ./...
GOTOOLCHAIN=go1.26.6 go test -race ./...

# Dashboard
cd dashboard
npm run lint
npm run test:unit
npm test

# Integrated and source contracts
cd ..
./scripts/test-built-server-contract.sh
python3 scripts/doc-lint.py
python3 scripts/host-neutrality.py
git diff --check
```

CI validates pushes and pull requests targeting `main` or `master`. Documentation
changes on the explicit allowlist run document and host-neutrality checks; other
changes run the five product jobs, including installer smoke against the stamped
binary. Manual `workflow_dispatch` always runs the full product checks.

Read `CONTRIBUTING.md` when reproducing CI or collecting exact-commit evidence.
A documentation-only success does not prove the built product. Before deployment,
require full-product success for the candidate commit. Runtime deployment is
separate and requires the operator-approved local target. When changing the
installer, run its smoke in both source and binary modes locally.
