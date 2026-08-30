---
name: chrote-run-and-operate
description: Use when an explicitly authorized task requires running, installing, upgrading, restarting, health-checking, or rolling back CHROTE. Do not use repository defaults to guess an existing deployment target.
---

# Run and operate CHROTE

Verified: 2026-08-30

Runtime work is separate from source verification. `AGENTS.md` owns the runtime
contract; `docs/installation.md` covers a fresh installer-managed setup, and
`docs/troubleshooting.md` covers that setup's failure boundaries. Existing
deployments may differ and are identified only by approved operator context.

## Preconditions

- Confirm that the task explicitly authorizes the requested runtime mutation.
- Resolve the exact target, endpoint, working tree, and procedure from untracked
  operator configuration. If any value is missing or ambiguous, stop and ask.
- Inspect source and runtime state before changing either one.
- Preserve tmux sessions, unrelated processes, and unrelated worktrees.
- Never print private configuration or include it in tracked output.

## Capture the before state

Record enough evidence to compare after the action:

- source commit and dirty state;
- resolved runtime target and its current health;
- running executable or build identity when observable;
- relevant tmux session names and counts;
- the exact user-facing route or API behavior the task intends to change.

Do not treat a healthy repository checkout as proof of the deployed runtime,
and do not treat process existence as proof of route behavior.

## Verify the candidate

1. Run the source gates required by `AGENTS.md` for the changed surface.
2. Build embedded assets only with `scripts/build-embedded-dashboard.sh`.
3. Confirm `scripts/check-embedded-dashboard.py` passes before building or
   installing a server.
4. For installer changes, run both disposable public-install modes documented
   in `AGENTS.md`. Do not use a real operated target as an installer fixture.

## Perform the authorized action

- Use the procedure named by current operator configuration.
- Use `install.sh` only for the fresh or installer-managed workflow documented
  in `docs/installation.md`; it is not evidence about another deployment.
- Touch only the resolved target. Do not stop, rename, or recreate tmux work.
- Do not change ownership, modes, or ACLs as an operational shortcut.
- Keep the action reversible. Rollback is `git revert` plus a rebuild, never a
  saved-binary swap.

## Prove the after state

Re-run the same observations captured before the action and verify:

- the intended target is healthy;
- the running build corresponds to the intended source;
- the changed route or API behavior works at the approved endpoint;
- pre-existing tmux sessions remain present;
- no unrelated target was restarted or modified.

Repository tests, a successful restart, and an HTTP response are separate
pieces of evidence. Report each one accurately; do not collapse them into a
single success claim.

## If it fails

- Stop after the first unexplained target or identity mismatch and report the
  exact evidence. Do not probe guessed services or ports.
- If health regresses, use the approved rollback procedure and repeat the same
  before/after checks. Do not restart adjacent infrastructure.

## Done when

- Source, runtime identity, health, route behavior, and preservation evidence
  agree with the requested outcome.
- Every warning, skipped gate, and unavailable proof is stated explicitly.
- The handoff distinguishes repository state from deployed runtime state.
