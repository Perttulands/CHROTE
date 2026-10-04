# CHROTE

CHROTE is a browser-based agentic IDE for one trusted operator. It makes tmux-hosted terminal sessions easy to run, arrange, observe, and steer from devices on the operator's private network.

## Product boundaries

tmux owns live sessions. Product changes, tests, deployments, browser disconnects and service restarts must preserve existing sessions. Exact operator-authorized deletion and cleanup of test-owned or failed-creation-owned sessions are the exceptions.

Broad access within configured roots is intentional. Unix permissions define access; report failures plainly. Tracked source and documentation stay host-neutral. Real deployment paths, ports, sockets, service identities and private operator material belong in operator configuration.

For product intent use `VISION.md`; for supported behavior use `PRD.md`; for component boundaries use `ARCHITECTURE.md`; for changes to trust or access use `SECURITY.md`.

## Source and build entrypoints

- `dashboard/src/` owns React UI and device-local presentation.
- `src/internal/api/` exposes host resources; `src/internal/proxy/` owns terminal transport and attached pseudo-terminals.
- `src/cmd/server/` assembles the server; `src/internal/dashboard/` embeds the dashboard it serves.
- `scripts/` owns build, installation and validation entrypoints.

Generate embedded assets with `./scripts/build-embedded-dashboard.sh`; `python3 scripts/check-embedded-dashboard.py` checks their consistency. Directly copying build output bypasses that contract.

This repository owns the `chrote-` Beads store. Host deployment configuration belongs to its operator workspace.

## Verification and delivery

For test changes, consult `docs/TEST_STRATEGY.md` for behavior ownership and CI routing. `CONTRIBUTING.md` gives commands and environments for Go, dashboard, built-server and installer checks. Select checks for the changed behavior.

Documentation changes use `python3 scripts/doc-lint.py`, `python3 scripts/host-neutrality.py` and `git diff --check` from the repository root. The CI documentation allowlist runs these without proving the built product; manual `workflow_dispatch` runs all product jobs.

Deployment to the established internal target requires full-product success for the candidate commit. For installer changes, the source-mode and binary-mode smoke checks exercise distinct installation paths.
