---
name: chrote-build-and-env
description: Use when bootstrapping CHROTE dependencies, building the embedded dashboard and server, reproducing CI, or diagnosing build and embedded-asset drift. Do not use for live installation or service restarts.
---

# CHROTE build and environment

Verified: 2026-08-30

Keep this skill procedural. `AGENTS.md`, `CONTRIBUTING.md`, and
`.github/workflows/ci.yml` own the current commands, toolchain pins, and quality
policy. Read them before relying on this summary.

## Preconditions

- Work from the repository root.
- Inspect `git status --short`; preserve unrelated and untracked work.
- Read `src/go.mod`, `dashboard/package.json`, and the CI workflow for current
  runtime and dependency requirements.
- Treat repository verification and runtime deployment as separate tasks.

## Build the repository

1. Install the locked dashboard dependencies:

   ```bash
   npm ci --prefix dashboard
   ```

2. Build the dashboard and refresh the Go embed through the owning helper:

   ```bash
   ./scripts/build-embedded-dashboard.sh
   ```

   Never copy either `dist` tree by hand. The helper owns the embed and its
   source fingerprint.

3. Run the Go checks with the toolchain prefix currently specified by
   `AGENTS.md`:

   ```bash
   cd src
   GOTOOLCHAIN=go1.26.6 go vet ./...
   GOTOOLCHAIN=go1.26.6 go test ./...
   GOTOOLCHAIN=go1.26.6 go test -race ./...
   cd ..
   ```

4. Run the dashboard checks:

   ```bash
   cd dashboard
   npm run lint
   npm run test:unit -- --coverage
   npm test
   cd ..
   ```

5. Run repository contracts relevant to the change:

   ```bash
   python3 scripts/doc-lint.py
   python3 scripts/host-neutrality.py
   python3 scripts/check-embedded-dashboard.py
   git diff --check
   ```

Use `CONTRIBUTING.md` and CI when the task requires the full built-server,
formatting, dead-code, or dependency-scan contracts. Do not invent a parallel
gate or duplicate a repository script.

## Boundaries

- Do not restart a process, touch a live endpoint, or infer an operated target
  while validating source.
- Do not run live browser tests without an explicitly approved target.
- If the default browser-test port is occupied, select an unused port through
  `CHROTE_PLAYWRIGHT_PORT`; do not kill the listener.
- Rollback is `git revert` followed by a rebuild, never a binary swap.

## Done when

- The checks required by the changed surface pass without hidden skips.
- The embedded dashboard check reports that source and bundle agree.
- `git diff --check` is clean and unrelated work remains untouched.
- The handoff lists every command, result, warning, and skipped check.
