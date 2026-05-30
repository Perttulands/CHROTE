# Gas City Real-Harness Safety Boundary — Enforcement Test

Asserts CHROTE 3.0 **ready-criterion 7** ("the safety boundary holds") for the
packaged harness adapters. Beads: `home-td6q` (this enforcement test),
`home-4xv.5` / ADR-0002 (the boundary it asserts), `home-7jk3` (the adapter mold
+ install gate it hardens), `home-5na1` (the ready gate it feeds).

`home-4xv.5` defined the real-harness safety boundary as a decision doc.
Ready-criterion 7 requires it to **hold**, not just be written. This is the
executable counterpart: a repeatable check that fails loudly if any adapter — or
the install gate — regresses away from default-deny / no-credential-leak.

## What runs

```bash
PKG=/home/perttu/chrote/docs/gas-city-research/adapter-research/harness-adapters
"$PKG/bin/check-safety-boundary"            # pass/fail summary, exit 0/1
"$PKG/bin/check-safety-boundary" --verbose  # per-check PASS/FAIL lines
```

The check is **hermetic**: it reads tracked source and writes only under a
private `mktemp` dir (removed on exit). It NEVER starts a Gas City supervisor,
NEVER runs `gc init`, and NEVER writes to the live `/home/perttu/gascity` city.
The install gate it exercises (`bin/install-adapter`) reads source files only;
`--dry-run` writes nothing, and the adversarial fixtures are pointed at throwaway
cities that are asserted empty. This satisfies the `home-td6q` / `home-d0lv`
constraint to run only against disposable/no-register state.

It auto-discovers every adapter under `adapters/`, so it covers all current
adapters and any future one with no edit.

## Assertions

Per packaged adapter (`opencode-smoke`, `codex-smoke`, `claude-code`):

| id | asserts (maps to home-4xv.5 boundary) |
|----|----------------------------------------|
| `P0-install`    | the install safety gate accepts the legitimate adapter |
| `P1-forbidden`  | injecting `--dangerously-bypass-approvals-and-sandbox` into the wrapper makes the gate **refuse** (0 files written) — the forbidden-substring check is real |
| `P1b-declared`  | the adapter declares `--dangerously-bypass` and `--yolo` in `forbidden_substrings` |
| `P2-deny`       | tool default-deny via the **declared + validated** mechanism: stripping the real enforcement (config `--tools` allowlist / structural `<tool>: deny`, or `--sandbox read-only`) makes the gate **refuse** — proving P0 passed on the real mechanism, not incidentally |
| `P3-session-gate` | running the real wrapper with `GC_SESSION_ID` unset exits **64** and sends nothing (immutable session-id required before mail) |
| `P4-scrub`      | the wrapper normalizes `PATH`, dumps no environment, and `controller.token` is forbidden (env/credential scrubbing) |

Adversarial negatives — each **must be refused, nothing installed**:

| id | fixture |
|----|---------|
| `neg-a/junk-deny`      | a file containing the word "deny" but no real deny mechanism |
| `neg-b/comment-tools`  | a `--tools` allowlist present **only in a comment** (no real flag) |
| `neg-c/dangerous-flag` | a wrapper carrying `--dangerously-skip-permissions` |
| `neg-d/no-session-gate`| a wrapper whose `GC_SESSION_ID` appears **only in a comment** (no real gate) |
| `neg-e/comment-sandbox`| `--sandbox read-only` present **only in a comment** (sandbox analog of neg-b) |

(`neg-a`–`neg-d` are the four required by `home-td6q`. `neg-e` additionally
guards the `sandbox-read-only` branch's comment-hardening so all three hardened
checks have a negative.)

`home-d0lv` coverage (`d0lv/bypass-covered`): `gc init --provider codex` can
materialize a mayor session carrying `--dangerously-bypass-approvals-and-sandbox`.
That is `gc init` / mayor provisioning, **out of adapter scope** — this package
never calls `gc init`. At the adapter boundary the check asserts every adapter
forbids that bypass family (the literal `--dangerously-bypass` prefix, matched by
the gate's `grep -F`), so an adapter can never reintroduce it. The `gc init` path
itself is tracked and resolved under `home-d0lv`: scaffold disposable cities from
an inert `--file` config (`start_command = "true"`, `[beads] provider = "file"`),
never `--provider codex` — see
`chrote-3.0-gascity/docs/gas-city-research/architecture/substrate-map.md`, "Safe Disposable City
Proof Path (home-d0lv)".

## Result (2026-05-27)

`bin/check-safety-boundary` exits **0** with all checks green:

```
adapters tested: 3 (claude-code codex-smoke opencode-smoke)
checks: 24  passed: 24  failed: 0
BOUNDARY HOLDS: all 24 assertions passed across 3 adapters + 5 adversarial negatives.
```

The three live adapters also pass read-only install verification against the
live city (`install-adapter <name> --verify --city /home/perttu/gascity` →
`verify OK` for each), confirming the tracked source still matches the live
runtime and nothing was mutated by this work.

### Fails-loud evidence (the test has teeth)

Each guard was confirmed to fail the check when regressed (Beads home-td6q
verification), exit 1 with a clean `FAIL` line and no crash:

- un-hardening the install gate's config `--tools` check → `neg-b` FAILs.
- un-hardening the `GC_SESSION_ID` check → `neg-d` FAILs.
- un-hardening the `--sandbox read-only` check → `neg-e` FAILs.
- a dangerous flag added to a real adapter wrapper → that adapter's `P0` FAILs.
- neutralizing a real wrapper's `exit 64` session gate → that adapter's `P3` FAILs.

## Install-gate hardening recorded here (home-7jk3)

This work also hardened `bin/install-adapter` so the structural checks are real
code, not prose. The config `--tools` check, the `--sandbox read-only` check, and
the `GC_SESSION_ID` gate check now match **only on non-comment lines** of `bin/`
files (a new `grep_code` helper strips full-line and inline shell comments before
matching). A previously-open hole — the config `--tools` check matched anywhere in
`bin/`, including a prose comment that named the flag with no real invocation —
is closed and regression-guarded by `neg-b`. Forbidden-substring checks
deliberately still match everywhere (comments included): a dangerous flag must
appear **nowhere** in the committed files. See `home-7jk3` notes and the README
"Tool-deny: two valid enforced forms" section.

## Feeding the ready gate (home-5na1)

`home-5na1` ("CHROTE 3.0 ready gate") lists "the safety-boundary enforcement
check (home-td6q) passes" as an acceptance item. The ready gate runs:

```bash
/home/perttu/chrote/docs/gas-city-research/adapter-research/harness-adapters/bin/check-safety-boundary
```

and treats a non-zero exit as a ready-bar failure for criterion 7.
