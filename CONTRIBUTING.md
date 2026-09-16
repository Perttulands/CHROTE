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
- tmux for the disposable built-server and installation checks

## Setup

```bash
git clone https://github.com/Perttulands/CHROTE.git
cd CHROTE

cd dashboard
npm ci
cd ../src
go mod download
cd ..
```

## Stable local gates

Run the narrow test first while developing. CI checks pushes to `main` and
`master`, and pull requests targeting either branch. Documentation-only changes
run document and host-neutrality checks. See the routing rules in
[`docs/TEST_STRATEGY.md`](docs/TEST_STRATEGY.md#ci).

Run these checks from the repository root for documentation changes:

```bash
python3 scripts/doc-lint.py
python3 scripts/host-neutrality.py
git diff --check
```

To reproduce all five product jobs locally, start at the repository root of a
clean, committed checkout. This recipe stops at the first failure. The installer
smoke checks that the running binary reports that checkout's full commit hash.

```bash
set -e
python3 scripts/doc-lint.py
python3 scripts/host-neutrality.py

# Build the same stamped assets and server as CI
npm ci --prefix dashboard
./scripts/build-embedded-dashboard.sh
./scripts/build-server.sh "$PWD/chrote-server-ci"
python3 scripts/check-embedded-dashboard.py

# Dashboard unit, lint, and mocked browser journeys
cd dashboard
npx playwright install --with-deps chromium
npm run test:unit
npm run lint
npm test
cd ..

# Go format, vet, and race checks
cd src
test -z "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))"
go vet ./...
go test -race ./...
cd ..

# Test the built and installed product on disposable sockets
CHROTE_SERVER_BINARY="$PWD/chrote-server-ci" ./scripts/test-built-server-contract.sh
CHROTE_EXPECTED_BUILD_COMMIT="$(git rev-parse HEAD)" \
  ./scripts/test-public-install.sh "$PWD/chrote-server-ci"
git diff --check
```

Use a free `CHROTE_PLAYWRIGHT_PORT` if another process owns the default test
port. The built-server and installer checks create and clean up their own tmux
sessions on disposable sockets. Live browser tests target an actual CHROTE
backend and need an approved instance.

For hosted full-product evidence on a documentation commit, use the CI workflow's
**Run workflow** action or `gh workflow run ci.yml --ref <branch-or-tag>`.
Manual dispatch always runs the full product gate. Confirm the resulting run's
commit matches the candidate and that `CI result` reports full-product success.
A PR run verifies GitHub's proposed merge commit; verify the resulting main
commit separately before deployment. Record the tested commit and workflow URL.
CI verifies the repository and installed product; host deployment is separate.

Run `govulncheck` and `npm audit --audit-level=moderate` when dependencies change.
CI has no scheduled dependency-scan job.

Change dashboard dependencies with npm 11, as `npx npm@11 install <pkg>` or
`npx npm@11 audit fix`. npm 10.9.7 fails to re-resolve this lockfile and exits
with `Cannot read properties of null (reading edgesOut)`. Installing the
lockfile unchanged with `npm ci` works on either line.

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

1. Use current `main`; branch when review or isolation needs it.
2. Keep the diff focused.
3. Add tests that prove changed behavior.
4. Run the relevant gates after the final edit.
5. Check `git diff --check` and `git status`.
6. Describe the operator outcome, important boundaries, and exact verification.
7. For a pull request, wait for its CI result. Direct work on `main` still needs
   successful validation of the exact committed revision before deployment.
   Do not force-push public `main`.

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
