# Security Policy

## Supported versions

CHROTE v2 is alpha. Security fixes target `main` and the newest v2 alpha release.
The v1 line and older snapshots are preserved for history but are not supported.

| Version | Supported |
| --- | --- |
| `main` | Yes |
| newest `2.0.0-alpha.*` | Yes, until superseded |
| `1.x` and older | No |

Before the next tagged alpha, release artifacts must be rebuilt with the patched
Go baseline declared by `src/go.mod` and pass both source and binary
`govulncheck` gates. Do not treat an older downloadable binary as equivalent to
current `main`.

## Trust model

CHROTE is private infrastructure for a single human operator, and that operator
boundary can span several Unix identities at once. CHROTE has
no built-in application login, and nothing in the application authenticates a
request.
Retired settings such as `API_AUTH_TOKEN` are ignored: the server logs a startup
warning if one is still set, and a value in an environment file provides no
protection.

Anyone who can reach the dashboard holds, at once:

- a terminal for **every** Unix user listed in `CHROTE_TERMINAL_USERS` — that is
  arbitrary command execution as each of those users, not just one;
- the file APIs across everything under `CHROTE_ROOTS` with the service
  identity's Unix permissions — deployments may configure broad roots, up to `/`;
- Beads data, local service proxies, schedules, recovery actions, and any
  experimental Formations surface included in that build.

The control is the network perimeter, not application authentication and not
same-user isolation. Default runtime values are loopback-only:

- `HOST=127.0.0.1`
- `PORT=8094`
- `TTYD_PORT=7683`

Use a private access layer such as Tailscale for remote access. Do not bind
CHROTE directly to an untrusted LAN or the public internet. Treat exposing the
dashboard as exposing the shells of every configured terminal user.

## CORS is not authentication

`CORS_ORIGINS` controls which browser origins receive cross-origin API headers.
If unset, CHROTE emits no CORS headers.

That protects browser API use across origins. It does not identify a user, stop
direct network clients, or replace the private-network trust boundary.

## Terminal boundary

CHROTE and ttyd can attach to tmux sessions available to their Unix identity and
to any explicitly configured socket mapping. A deployment commonly runs the
server as a dedicated service account while fronting other Unix users' sessions
through deliberate socket grants, so the reachable surface is the union of every
configured user's sessions, not the service account's alone.

- A terminal is arbitrary command execution as that Unix user.
- Cross-user socket access requires deliberate filesystem and tmux ACL setup.
- CHROTE must not guess socket ownership or silently widen access.
- Experimental Formations executor access does not permit creating or killing unrelated tmux
  sessions.
- Browser/device disconnects must not kill durable tmux work.

Treat exposing CHROTE as exposing a shell.

## Filesystem boundary

`CHROTE_ROOTS` constrains CHROTE's file APIs. `CHROTE_WORKDIR` controls the
default working directory for new sessions.

- Keep configured roots as narrow as practical. Narrow roots are advice, not a
  guarantee: a deployment that sets `CHROTE_ROOTS=/` grants the file APIs
  everything the service identity can read or write.
- Symlinks are resolved before access and mutation authorization.
- A broad root permits every operation the CHROTE API exposes within the Unix
  permissions of the service identity.
- File roots do **not** sandbox tmux agents; those processes retain their Unix
  user's filesystem permissions.
- Experimental Formations artifact hydration and script-gate working directories have their
  own documented root checks.

## Services, schedules, and executors

Optional service URLs and tokens are server-side runtime configuration. Browser
clients call CHROTE-owned proxy routes and must never receive service bearer
tokens.

Scheduled tasks, session locking, recovery actions, and experimental Formations
executors cross from observation into host mutation. Their contracts must:

- require explicit configuration and operator intent;
- use argument vectors instead of implicit shell parsing where possible;
- constrain working directories and executable boundaries;
- cap and redact recorded output;
- fail loud in durable history or ledgers;
- refuse unsafe promotion from lab/isolated environments to live host state.

Resuming a locked agent is a case of the second rule rather than an exception to
it. tmux receives one fixed launcher command containing no config data. That
launcher reads typed config separately and invokes the agent with an argument
vector; no rendered resume command is typed into a pane or parsed as shell text,
and startup does not depend on pane contents. Around that: a session name is
validated before it is used to build a unit name, so a name cannot introduce a
second unit, a flag, or a metacharacter; any grant to control units is scoped to
the verbs and the single unit-name pattern CHROTE owns, never to arbitrary unit
names. Cross-user calls use non-interactive sudo and one absolute, root-owned
helper path installed from the reviewed source; its sudoers grant cannot invoke
another program. Startup exercises each configured account's real user manager,
and an unproven account has no lock action. Per-agent configuration stays in a
CHROTE-owned directory with a
target-readable ACL, while launcher receipts stay in a separate target-owned
directory readable by CHROTE's service group. Both are mode 0640; receipt reads
reject ownership, mode, regular-file, and symlink-boundary violations. Receipts
are derived from the pane's observed process rather than desired config, bind to
the current systemd invocation and monotonic start time, and identify the live
PID by its process start ticks. Publication failure prevents unit readiness.

## Secrets

Never commit:

- bearer tokens or API keys;
- private service environment files;
- tmux socket grants or sudoers fragments for a specific host;
- recovery manifests containing private command lines or paths;
- terminal transcripts;
- private screenshot content.

Use owner-controlled runtime environment files with restrictive permissions.
`.env.example` documents variable names only.

## Dependencies and releases

A release is not complete merely because a binary exists.

- CI must use the Go version declared by `src/go.mod`.
- `govulncheck ./...` must report no reachable source vulnerabilities.
- the exact release binary must pass `govulncheck -mode=binary`;
- build provenance must identify the intended clean commit and patched Go
  version;
- dashboard dependency audit, tests, and embedded-asset parity must pass.

## Reporting a vulnerability

Use GitHub's private security-advisory flow for the repository when available.
If private reporting is unavailable, contact the maintainer directly rather than
publishing exploit details in a public issue.

Include the affected commit/version, deployment boundary, reproduction steps,
impact, and any evidence that avoids exposing real credentials, terminal
contents, or private paths.
