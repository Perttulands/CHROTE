---
name: operate-chrote
description: Run, install, upgrade, restart, diagnose, health-check, deploy, or roll back an explicitly selected CHROTE runtime.
---

# Operate CHROTE

Repository verification and runtime operation are separate. Read `AGENTS.md`, then use `docs/installation.md` for installer-managed deployments and `docs/troubleshooting.md` for their failure boundaries. Existing deployments may use private operator configuration instead.

## Resolve the target

Before changing runtime state, establish the exact endpoint, working tree, service, tmux sockets, and approved procedure. Get missing values from operator configuration or the user. Do not infer a deployment from repository defaults, a familiar port, or a running process.

Capture enough before-state evidence to compare after the action:

- source commit and dirty state;
- runtime identity and health;
- relevant tmux server and session identities;
- the route or behavior the Bead intends to change.

## Verify the candidate

Run the source gates required by `AGENTS.md`. Build embedded assets only through the owning script and prove that source and bundle agree. Installer changes use disposable installer tests, never a live operated target as a fixture.

## Perform the operation

Use the procedure owned by the resolved target. Touch only that target. Preserve every pre-existing tmux server and session across installs, restarts, failures, tests, and rollback.

Use `install.sh` only for the installer-managed setup it documents. Roll back tracked changes with Git and rebuild from the intended source. Do not substitute an old binary and call the repository rolled back.

## Prove the result

Repeat the before-state observations and verify all of them:

- the intended runtime is healthy and runs the intended source;
- the changed route or behavior works at the approved endpoint;
- every pre-existing tmux server and session remains present;
- no unrelated runtime was restarted or changed.

A passing repository test, a successful service action, and working user behavior are different evidence. Report each accurately. Stop on an unexplained identity mismatch rather than probing guessed targets.
