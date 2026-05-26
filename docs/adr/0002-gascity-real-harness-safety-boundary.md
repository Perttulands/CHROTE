# ADR-0002: Gas City Real-Harness Safety Boundary

## Status

Proposed

## Context

ADR-0001 makes Gas City the CHROTE 3.0 orchestration substrate and keeps
CHROTE as the authenticated cockpit and policy layer. That direction is not
enough by itself for paid or credentialed harnesses.

The local Gas City spike has useful evidence, but it is mostly mock-agent
evidence. The mock agents are bash stand-ins that read stdin, write simple logs,
and have no credential store, spending authority, approval UI, or private
harness transcript state. The Pi smoke showed a real harness can be launched
with strict flags, but mail reaction was still partial and OpenCode was not
available in that shell. The `gc mail` poem smoke showed that a CHROTE-launched
Codex process can call Gas City mail, but it could only send as the default or
human identity. It did not prove Gas City owned the process, identity, approval
boundary, transcript, or rollback path.

The safety decision must therefore distinguish two modes:

- Mock-agent mode: acceptable for Gas City workflow, mail, formula, observer,
  and UI experiments because the processes are harmless shell stand-ins.
- Real-harness mode: default-deny until one adapter proves credential, file,
  approval, transcript, process ownership, and rollback behavior under Gas City
  identity.

## Decision

Integrate Codex as the first real harness, but only through one narrow
`codex-smoke` adapter boundary.

Codex is first because it is available in the local Gas City pilot evidence, it
is a central CHROTE target, and the prior poem smoke exposed the exact failure
ADR-0001 says must be solved: a CHROTE-launched Codex process is not enough
unless Gas City owns the identity and session. Pi remains useful evidence for
read-only wrapper design, but its mail/notify path is still partial. Claude is
not logged in in the recorded pilot evidence, and OpenCode was not on PATH.

The adapter is a request boundary, not a raw command boundary. Gas City may
create durable work, mail, nudges, events, and a named session. CHROTE may expose
bounded controls through its authenticated surface. The adapter must enforce the
real-harness rules below before any prompt reaches Codex.

## Adapter Boundary

### Credentials

The adapter may use only the harness-native credential already available to the
target user/session, or a dedicated narrow credential explicitly created for the
smoke. It may check whether expected commands and non-secret environment names
exist.

The adapter must not read, print, copy, persist, or pass through owner tokens,
broad Context Citadel tokens, OpenAI keys, shell history, SSH material, `.env`
contents, or full environment dumps. Any future credential injection must use an
allowlist by variable name and a dedicated principal.

### Environment Scrubbing

The adapter may set a minimal PATH, Gas City session metadata, a dedicated
session directory, and explicit non-secret feature flags.

The adapter must start from a scrubbed environment. It must not inherit broad
ambient variables by default, and it must not log command lines or diagnostics
that can include secret values. Startup diagnostics are limited to command path,
working directory, session id, policy mode, and credential-presence booleans.

### Filesystem Scope

The adapter may run in a dedicated smoke workspace under the Gas City or CHROTE
3.0 workstream and may read only explicitly allowed project paths needed for the
test. The first Codex smoke starts read-only.

The adapter must not treat `/home/perttu`, `/srv`, `~/.ssh`, `~/.config`,
Context Citadel data, Beads internals, or CHROTE runtime config as generally
available workspace. File mutation requires an explicit bead-backed request,
owned path scope, and CHROTE/user approval for that session.

### Approvals

The adapter may submit bounded prompts that ask for analysis, status, or a
trivial read-only response.

The adapter must require explicit human approval before writes, deletion,
package installation, network expansion, service changes, tmux control, process
kill/restart, credential use beyond the harness-native login, or commands that
would spend non-trivial money. Approval must be attached to the Gas City work or
Bead context, not hidden in an in-process memory flag.

### Dangerous Mode

The adapter must not enable dangerous, bypass, YOLO, auto-approve, or permission
prompt skipping by default. The existing Gas City managed-session setting that
skips dangerous-mode prompts is not acceptable for real harness sessions.

Dangerous mode may be tested only in a disposable workspace, for one named
session, with explicit Perttu approval, visible labeling, no broad credentials,
and a rollback note before launch.

### Transcript Retention

The adapter may retain bounded operational evidence: Gas City session id, target
alias, prompt summary, approval references, mail ids, event ids, exit state, and
redacted excerpts needed to debug routing.

The adapter must not copy full private harness transcripts into Beads, Context
Citadel, CHROTE docs, or Gas City mail by default. Raw transcript files stay in
the harness/session store unless an explicit retention policy says otherwise.
Any retained excerpt must be screened for credentials and private unrelated
content.

### Process Ownership

Gas City must own the real harness process it routes to. A valid real-harness
success requires a configured Gas City agent, a named Gas City session, a stable
Gas City identity, and mail/submit paths that address that identity.

CHROTE must not count a random terminal, arbitrary tmux pane, or
CHROTE-launched subprocess as a valid Gas City harness identity. Native harness
operation outside Gas City remains allowed for humans, but it is not evidence
for this adapter.

### Wrong-Session Targeting

The adapter may target a session only after resolving an immutable Gas City
session id and confirming the expected configured agent name.

The adapter must fail closed on ambiguous aliases, stale sessions, missing
identity, mismatched agent config, or attempts to target raw tmux session names.
Mutation commands must include both the resolved session id and the expected
agent alias in the audit trail.

### Kill And Restart Behavior

The adapter may request graceful stop of the one smoke session it created, after
recording the reason and preserving bounded transcript evidence.

The adapter must not kill, restart, drain, or reparent existing CHROTE, tmux,
Gas City, or harness sessions. Automatic restart for a real paid/credentialed
harness is disabled until a separate restart/reconciliation drill proves that
pending work, mail, transcript pointers, and approvals survive safely.

### Rollback

Rollback is configuration-first:

- disable the `codex-smoke` Gas City agent;
- hide or disable CHROTE mutating controls for real harnesses;
- stop creating new real-harness sessions;
- gracefully close only sessions created by the adapter and approved for
  closure;
- keep Beads as work truth and Context Citadel as context truth;
- return CHROTE operation to the CHROTE 2.0 baseline or read-only Gas City
  observer mode.

Rollback must not require deleting Beads, editing Context Citadel history,
rewriting Gas City runtime files by hand, or exposing the raw supervisor.

## Non-Goals

- No broad multi-harness rollout.
- No dangerous mode by default.
- No owner-token, broad local-agent-token, SSH, shell-history, or full-env
  leakage.
- No raw Gas City supervisor exposure through CHROTE, Tailscale, public web, or
  unauthenticated local proxies.
- No change to Beads as durable work truth or Context Citadel as durable context
  truth.
- No claim that mock-agent, Pi smoke, or poem smoke results prove real-harness
  safety.

## Verification Before Connecting A Paid/Credentialed Harness

A real Codex harness is not considered connected until all gates below pass and
the results are recorded:

1. Perttu accepts this ADR or an updated replacement.
2. `codex-smoke` has a dedicated wrapper/config with scrubbed environment,
   read-only/default-deny mode, no dangerous prompt skipping, and no full env
   dump.
3. `gc config explain --agent codex-smoke` and `gc doctor --verbose` pass.
4. The first run uses a disposable or explicitly scoped workspace and proves
   `gc session peek <session>` shows the real Codex harness process, not only a
   shell wrapper.
5. Direct `gc session submit` returns a trivial response without file mutation.
6. Gas City mail stores an addressed message for the Codex identity, and any
   injection/reaction bridge is explicit rather than assumed from storage alone.
7. Wrong-session checks fail closed for ambiguous alias, stale session id, and
   raw tmux-name targeting.
8. A write attempt, delete attempt, package install, network-expanding command,
   service change, tmux control, and process kill/restart each either fail or
   require visible human approval.
9. Transcript retention produces only bounded, redacted operational evidence;
   token-pattern scans over changed files and retained excerpts find no leaked
   credential values.
10. Rollback is drilled by disabling `codex-smoke`, proving no new real-harness
    session can be launched through CHROTE/Gas City controls, and leaving the
    read-only observer/mock-agent path intact.

## Consequences

What gets safer:

- Gas City can keep proving workflow, mail, and observer value with mock agents
  while real harnesses stay default-deny.
- The first real integration has a small blast radius and a named rollback path.
- CHROTE remains the policy and authentication boundary instead of exposing raw
  supervisor mutation.
- Future harnesses can reuse a proven adapter pattern instead of inventing
  one-off wrappers.

What gets harder:

- The first Codex integration must do more than "launch a command"; it must
  prove identity, approvals, transcript handling, and wrong-session safety.
- Existing Gas City prompt-skipping behavior has to be separated from real
  harness sessions.
- Mail storage, mail injection, and harness reaction must be tested separately.
- Some convenient CHROTE/tmux shortcuts are deliberately not counted as success.

What remains risky:

- Restart/reconciliation for real harnesses is still unproven.
- Transcript recovery is still an adapter concern.
- Cost and credential behavior depend on the exact harness mode selected at
  implementation time.
- This ADR is only Proposed; connecting Codex for real requires acceptance plus
  the verification gates above.
