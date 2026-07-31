# Private Beads sidecar contract

CHROTE keeps operational issue state private while making it durable and
revision-addressable. The checkout-local `.beads/` database is authoritative for
live work; it is intentionally excluded from this public source repository.
Owner and maintainer installations replicate that state to a separately
authorized private Git sidecar.

## Required transport properties

A compliant sidecar must:

1. use a private remote distinct from the public CHROTE source origin;
2. carry native Dolt history (`refs/dolt/data`) as the exact recovery path;
3. carry a full `bd export --all` artifact for inspection and migration;
4. pair each exported tracker state with the CHROTE source commit, branch,
   tracked-worktree state, Dolt commit, checksum, and record counts;
5. synchronize periodically so tracker-only changes leave the machine;
6. synchronize successfully before a CHROTE source push proceeds;
7. fail non-zero on export, native push, Git push, or verification errors; and
8. prove restore in a disposable workspace without overwriting an existing
   `.beads/` database.

The portable JSONL export is not a substitute for native Dolt replication.
Current `bd` versions may normalize fields during JSONL import, so exact recovery
must use the private Dolt remote and verify the resulting export against the
sidecar artifact.

## Boundaries

- Sidecar URLs, credentials, checkout paths, user-service units, and schedules
  are operator configuration. Do not commit them here.
- Public contributors must not be assumed to have sidecar access.
- Never configure the public CHROTE Git remote as a Beads Dolt destination.
- Never commit raw tracker exports, memories, interactions, or workspace
  metadata to this repository.
- Before native push, inspect `bd dolt remote list --json` and confirm the target
  is the authorized private sidecar.

A fresh owner checkout restores by creating local workspace identity/configuration
and running `bd bootstrap` against the private Dolt remote. Keep that procedure
in the private sidecar so it travels with the data and cannot leak operator
topology into public documentation.
