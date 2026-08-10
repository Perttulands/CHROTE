# ADR-0001: Workload-Aware Session Recovery Descriptors

## Status

Accepted. Partially superseded 2026-08-09 by
[ADR-0015](0015-access-first-non-interference.md): CHROTE no longer owns
continuous supervision and the restart-capable CHROTE-installed external-manager
cell is retired. The descriptor model, the exactly-one-owner rule, and the
unresolved-rather-than-guessed discipline recorded here remain in force for
operator-triggered recovery. ADR-0014 was the intermediate supervision decision
and is now superseded in full.

## Context
CHROTE currently sees tmux sessions, windows, panes, process names, and recent
text. That topology is useful transport evidence, but it is not workload
identity. A pane named `codex-alpha` might contain a resumable Codex transcript,
an intentional shell, a Python static server, a process supervised by another
manager, or evidence too ambiguous to recover safely.

Session Bank one-shot recovery and the now-retired Persistent Agent continuous
supervision path needed a shared vocabulary before either subsystem changed
behavior. Without typed descriptors, recovery would either guess from
names/transcripts or persist raw commands that can drift into unsafe restart
behavior.

## Decision
Introduce workload recovery descriptors as the shared recovery model. A
descriptor is pane-aware and records the tmux session/window/pane topology,
including tmux `windowLayout`, that produced the evidence. Topology remains
evidence only. The workload identity is the typed descriptor mode and fields.

Descriptor modes are:

- `topology`: CHROTE can preserve or present the tmux shape, but there is no
  workload-specific restart contract. Intentional shells use this mode.
- `agent`: a known agent workload with an allowlisted agent kind, native session
  id, and optional Hermes profile.
- `command`: a known typed command workload. The initial command type is only
  `python-http-server`, with loopback bind, numeric port, and an owner-home
  bounded directory.
- `managed`: an external manager owns the workload and CHROTE must not create a
  competing restart owner.
- `unresolved`: evidence exists, but it is unknown, unsafe, missing, or
  ambiguous.

Every descriptor has exactly one recovery owner:

- Session Bank owns one-shot recovery for banked sessions.
- CHROTE owns no continuous supervision path. (The former Persistent Agents and
  CHROTE-installed systemd owner were retired by ADR-0015.)
- External managers own their sessions; CHROTE may observe them but must not
  reconstruct them as Session Bank work.

The owner records kind, reference, and whether that owner is allowed to restart.
Mode and owner combinations are strict: agent, command, and topology descriptors
require a restarting Session Bank owner; managed descriptors require a
non-restarting external manager; unresolved descriptors cannot permit restart.
Descriptors carry evidence source and confidence so later recovery code can
fail loudly instead of turning weak evidence into a restart command.

Accepted immutable manifests written by the retired implementation may still
contain a `persistent_agent` owner and remain parseable for read compatibility.
No current path produces that owner, and reading it does not reinstate a live
continuous-supervision capability.

Canonical argv is derived from typed fields only; shell command strings are only
a rendered view of that argv and must quote unsafe tokens. Codex and Claude use
UUID resume ids. Hermes uses a validated profile, a native session id, the
owner-home-bounded managed-venv Python path, and module `hermes_cli.main`.
Python HTTP server recovery uses only the typed bind, port, and directory
fields.

## Rejected Alternatives
- **Newest-transcript guessing:** rejected. A recent transcript is evidence, not
  ownership. Choosing the newest candidate can resume the wrong workload.
- **Raw environment capture:** rejected. Environments can contain credentials and
  host-specific noise. Descriptors must not persist raw environment maps.
- **Arbitrary command persistence:** rejected. Stored command strings cannot
  override canonical output. Commands must be typed and allowlisted.
- **Duplicate recovery owners:** rejected. A session cannot be owned by both
  Session Bank and an external manager.

## Consequences
This creates a schema/probe foundation for operator-triggered Session Bank
reconstruction and external-manager observation. Current Session Bank JSON
remains readable, and existing response fields stay intact.

Future recovery work must first derive or read a descriptor, validate exactly one
owner, and then let only that owner perform its recovery action. If evidence is
ambiguous, unsupported, or unsafe, the descriptor must be `unresolved` rather
than guessed.

The trade-off is that some recoverable-looking panes will initially remain
unresolved until a typed workload is added. That is intentional: adding a new
command kind requires schema, sanitizer, probe, and tests rather than raw shell
command storage.

## Enforcement
- Go descriptor tests in `src/internal/api/recovery_descriptor_test.go` prove
  canonical Codex, Claude, Hermes, Python HTTP server, topology, managed, and
  unresolved descriptors.
- The same Go tests reject malformed ids/profiles, unsafe paths, unsafe binds,
  invalid ports, unsafe window layouts, wrong mode/owner combinations, duplicate
  candidates, conflicting owners, raw unresolved argv reasons, and stored
  resume-command overrides.
- The Go tests prove canonical argv for typed recovery and shell quoting for
  path tokens containing whitespace, semicolon, backtick, `$()`, and a single
  quote while preserving simple-path command output.
- The Go fixture parity test reads every owner-probe fixture `want` descriptor
  and validates it through the Go canonicalizer, including unresolved
  `conflicting_evidence` emitted for stale transcript conflicts.
- `TestSessionBankEntryLegacyJSONRoundTripKeepsCurrentFields` protects legacy
  Session Bank JSON readability and current field preservation.
- Python probe tests in `scripts/tmux-recovery/test_owner_probe.py` classify
  fixture evidence for Codex, Claude, Hermes, Python HTTP server, shell
  topology, ambiguity, and unknown processes.
- The probe fixtures assert that ambiguity is unresolved rather than newest-wins
  and that outputs do not contain raw `argv` or `env` keys.
- The Hermes probe fixtures use the production resume shape:
  `<ownerHome>/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main
  --profile <profile> [--resume <id>]`.
