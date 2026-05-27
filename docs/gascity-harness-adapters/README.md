# Gas City Harness Adapters

Tracked source for the **Gas City harness-wrapper adapters** that let real AI
harnesses (OpenCode today; Codex and Claude Code next) run as valid Gas City
session identities and return results through Gas City mail.

This package is the *adapter mold* referenced by Beads `home-7jk3` (keystone),
`home-piis` (Codex), and `home-5v4k` (Claude Code). It exists because the live
Gas City city root at `/home/perttu/gascity` is `.gitignore`'d (its top-level
`*` guardrail ignores `bin/`, `agents/`, and the harness workdirs), so the
runtime adapter files are not committed there. This tracked package + its
install script are the durable source of truth and the documented sync path
back into the live city.

## Why this lives in `chrote/docs/`

- CHROTE is the only one of the three trees (`/home/perttu`, `/home/perttu/gascity`,
  `/home/perttu/chrote`) with a git remote and a clean tracked working tree, so
  it is the durable, pushable home. The home repo ignores the `gascity/` dir
  outright, and the gascity repo's own top-level `*` guardrail ignores its
  `bin/`/`agents/`/workdirs, so neither can track these files.
- `docs/adr/0001-chrote-3-gas-city-substrate.md` (in this repo) records the
  CHROTE 3.0 decision: Gas City is the orchestration substrate; "future
  harnesses follow one substrate pattern instead of one-off CHROTE wrappers."
  Packaging the adapter mold here is that pattern, and it sits beside its own
  decision record and `docs/chrote-gascity-framing.md`.
- Not `integrations/`: CHROTE's `.gitignore` ignores `/integrations/` (comment:
  "Nested repo clones (reference only, not part of chrote)"). Even the existing
  `integrations/clawdbot/` is untracked. Putting the package there would silently
  fail the "tracked source" requirement, so it lives under the tracked `docs/`
  tree instead.
- Not `src/`: this is not Go code or a CHROTE command surface. CHROTE's
  `AGENTS.md` forbids mirroring the `gc` command tree. The package ships source
  files + a self-contained install script only; it never wraps or re-exposes
  `gc`. That makes it a tracked durable artifact, which `docs/` already holds
  (ADRs, plans).

## Layout

```
docs/gascity-harness-adapters/
  README.md                     this file
  bin/install-adapter           install / dry-run / verify a single adapter into a city root
  adapters/
    opencode-smoke/
      adapter.toml              machine-readable adapter manifest + safety contract
      bin/opencode-gc-agent     the Gas City start_command wrapper (one-shot opencode run)
      agent/agent.toml          the Gas City agent definition (-> agents/opencode-smoke/agent.toml)
      workdir/.opencode/agent/gas-city-smoke.md   the OpenCode agent prompt (tool default-deny)
  ingress/
    hermes/                     Hermes -> Gas City INGRESS adapter (different shape; see below)
  docs/
    gascity-opencode-parity.md  preserved smoke evidence (gc-52763 / gc-52766)
    gascity-hermes-ingress.md   ingress bridge verification evidence (home-qnzi)
```

> **Two adapter shapes live here.** `adapters/` is the **harness-wrapper mold**
> (wrap a CLI as a long-lived Gas City-owned *session identity* that returns
> mail; driven by `install-adapter`). `ingress/hermes/` is the **ingress mold**
> (a sanitized request envelope → native `gc` primitives → an artifact receipt;
> no gc agent, no session, not touched by `install-adapter`). They are
> intentionally different; see `ingress/hermes/README.md`.

Adapter source layout maps to the live city root as declared by each adapter's
`adapter.toml` `[[file]]` entries:

| source (in this package)                        | live city dest (`/home/perttu/gascity/...`)         |
|-------------------------------------------------|-----------------------------------------------------|
| `adapters/opencode-smoke/bin/opencode-gc-agent` | `bin/opencode-gc-agent`                             |
| `adapters/opencode-smoke/agent/agent.toml`      | `agents/opencode-smoke/agent.toml`                  |
| `adapters/opencode-smoke/workdir/.opencode/agent/gas-city-smoke.md` | `opencode-smoke/.opencode/agent/gas-city-smoke.md` |

Runtime-only state is deliberately **not** packaged and never copied:
`.gc/` (supervisor state, `controller.token`, sockets), `node_modules/` under
the harness workdir, and any `*.done` run markers.

## Install / sync into the live city

```bash
PKG=/home/perttu/chrote/docs/gascity-harness-adapters

# List available adapters.
"$PKG/bin/install-adapter" list

# Preview what would change in the live city (no writes).
"$PKG/bin/install-adapter" opencode-smoke --dry-run

# Recreate/sync the adapter into the live Gas City city root
# (default city: $GC_CITY, else /home/perttu/gascity).
"$PKG/bin/install-adapter" opencode-smoke

# Confirm a city matches the tracked source (non-zero exit on drift).
"$PKG/bin/install-adapter" opencode-smoke --verify

# Then reload Gas City so it picks up the agent:
gc --city /home/perttu/gascity reload
gc config explain --agent opencode-smoke
```

`--city DIR` targets any city root, including a disposable temp dir for testing.

The install step runs the adapter's `[safety]` contract first and refuses to
write anything if any invariant is violated (see Safety contract below).

## The adapter mold (contract for new harnesses)

A harness adapter is a directory under `adapters/<name>/` containing:

1. **`adapter.toml`** — identity + file map + safety contract:
   - `[adapter] name` — the Gas City agent alias and `agents/<name>/` dir name.
   - `[adapter] harness` — the underlying CLI (`opencode`, `codex`, `claude`, ...).
   - `[adapter] lifecycle` — `one-shot` (wrapper runs the CLI once and exits) or
     `interactive` (wrapper `exec`s a long-lived session). OpenCode is `one-shot`;
     the existing `pi-gc-agent` in the live city is the `interactive` shape.
   - `[[file]]` blocks — each `src` (relative to the adapter dir) → `dest`
     (relative to the city root) with an octal `mode`.
   - `[safety]` — the invariants below, including `tool_deny_mechanism`
     (`config` | `sandbox-read-only`).

2. **A wrapper** (`bin/<harness>-gc-agent`) that, at minimum:
   - normalizes `PATH` and prints only minimal diagnostics (no env dumps, no secrets);
   - resolves the city root from `GC_CITY`/`GC_DIR`;
   - **requires an immutable `GC_SESSION_ID`** and refuses to send mail without
     it (identity must come from Gas City, never from `GC_ALIAS` alone);
   - drives the harness in a contained, tool-restricted mode (no dangerous-skip
     / broad-permission flags);
   - returns its result via `gc --city "$city" mail send human --from "$GC_SESSION_ID" ...`.

3. **A Gas City agent definition** (`agent/agent.toml`) using the minimal shape:
   ```toml
   description = "Contained smoke-test <harness> harness agent."
   work_dir = "."
   start_command = "./bin/<harness>-gc-agent"
   prompt_mode = "none"
   nudge = "ready"
   max_active_sessions = 1
   min_active_sessions = 0
   ```

4. **A deny posture** that the harness actually enforces, declared via
   `[safety] tool_deny_mechanism` (see the two valid forms below). The harness
   must not be able to edit, run shell, hit the web, or spawn sub-tasks during a
   smoke. Where the harness supports it, also ship a human-readable
   tool/permission config under `workdir/...` as belt-and-braces.

### Tool-deny: two valid enforced forms

Different harnesses enforce deny differently. The adapter must **declare which
mechanism it relies on** in `[safety] tool_deny_mechanism`, and `install-adapter`
validates the declared mechanism *structurally* (a bare occurrence of the word
"deny" never satisfies the check):

- **`config`** — the harness has a per-tool permission config. There must be a
  structural `<tool>: deny` block in a `workdir/` file (e.g. OpenCode's
  `gas-city-smoke.md` front matter denies `bash`/`edit`/`webfetch`/`task`/…),
  or an explicit read-only `--tools <allowlist>` in the wrapper (e.g. Pi's
  `--tools read,grep,find,ls`). Validated by matching `^\s*<tool>\s*:\s*deny\s*$`
  in `workdir/` or `--tools <allowlist>` in `bin/`.
- **`sandbox-read-only`** — the harness has no per-tool deny config and no
  `--tools` flag; deny is enforced by a read-only sandbox. The **wrapper** must
  actually invoke it (e.g. Codex's `codex exec --sandbox read-only`). Validated
  by matching `--sandbox read-only` (or `--sandbox-read-only` / `--sandbox=read-only`)
  in `bin/`. Any prose deny instructions (e.g. an `AGENTS.md`) are belt-and-braces
  layered on top, **not** the enforcement mechanism.

> **The mechanism must be real code, not a comment.** `install-adapter`
> evaluates the `--tools`, `--sandbox read-only`, and `GC_SESSION_ID` checks
> only against the *executable* lines of `bin/` files: full-line `#` comments
> and inline ` #…` trailing comments are stripped before matching. An adapter
> cannot pass the gate with a prose comment that merely mentions a flag — the
> flag has to be in the actual invocation. (Forbidden-substring checks are the
> opposite: they match everywhere, comments included, because a dangerous flag
> must appear *nowhere* in the committed files.)

### Safety contract (enforced by `install-adapter`)

These mirror the `home-4xv.5` real-harness boundary. The install script asserts
them against the source before writing and **fails loud** otherwise:

- `requires_immutable_session_id = true` → the wrapper must gate on `GC_SESSION_ID`.
- `tool_default_deny = true` → requires a declared `tool_deny_mechanism`
  (`config` | `sandbox-read-only`) whose enforcement is structurally present
  (see above). An undeclared or unknown mechanism is a violation.
- `no_dangerous_skip_flags = true` → declarative assertion of intent.
- `forbidden_substrings = [...]` → none of these strings may appear in any
  committed adapter file (e.g. `--dangerously-skip-permissions`, `--yolo`,
  `controller.token`).

### Adding Codex / Claude Code

Copy `adapters/opencode-smoke/` to `adapters/<name>/`, then:

1. Set `[adapter] name`, `harness`, `lifecycle`, `description`.
2. Replace the wrapper with the harness-specific invocation, keeping the
   `GC_SESSION_ID` gate, the contained/tool-restricted launch, and the
   `gc mail send ... --from "$GC_SESSION_ID"` return.
3. Repoint `[[file]]` `src`/`dest` at the new files.
4. Keep `[safety]` satisfied (Codex is paid/credentialed with a dangerous-bypass
   history — do **not** relax these defaults). Declare `tool_deny_mechanism`
   honestly: `config` if the harness has a per-tool deny config or `--tools`
   allowlist, `sandbox-read-only` if deny is enforced by a read-only sandbox the
   wrapper invokes. The install gate validates the declared mechanism is really
   present.
5. `install-adapter <name> --dry-run`, then install, `gc reload`, run one
   disposable smoke, record the gc ids in `docs/`.

## Safety-boundary enforcement check

`bin/check-safety-boundary` is the repeatable test that the `home-4xv.5`
real-harness boundary actually **holds** for every packaged adapter (CHROTE 3.0
ready-criterion 7). It is hermetic — reads tracked source, writes only under a
private `mktemp` dir, never starts a supervisor, never runs `gc init`, never
touches the live city.

```bash
PKG=/home/perttu/chrote/docs/gascity-harness-adapters
"$PKG/bin/check-safety-boundary"            # exit 0 = boundary holds; 1 = regressed
"$PKG/bin/check-safety-boundary" --verbose  # per-check PASS/FAIL
```

It asserts, per adapter, that there are no dangerous/bypass/yolo flags, that
tool default-deny is enforced via the *declared + validated* mechanism, that the
wrapper requires an immutable `GC_SESSION_ID` before mail (exit 64 otherwise),
and that env/credentials are scrubbed; plus adversarial negatives that must all
be refused (junk "deny", comment-only `--tools`, dangerous-flag wrapper,
missing session gate, comment-only `--sandbox read-only`). The `gc init` mayor
dangerous-flag finding (`home-d0lv`) is covered at the adapter boundary and
tracked out-of-scope. See `docs/gascity-safety-boundary-enforcement.md` for the
full assertion list, the recorded all-pass result, and fails-loud evidence. The
ready gate (`home-5na1`) runs this script.

## Verification evidence

See `docs/gascity-opencode-parity.md` for the OpenCode parity smoke
(session `gc-52763` → mail `gc-52766`) and the install/verify evidence proving
this package recreates the live config.
