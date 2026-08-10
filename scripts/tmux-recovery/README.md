# CHROTE tmux recovery operator tools

These tools are the operator-side companion for the workload-aware recovery
descriptor contract in `docs/adr/0001-workload-aware-session-recovery.md`.

The Python clients are intentionally thin:

- `snapshot.py` writes an immutable timestamped manifest and posts only
  `session_bank` recovery plans to CHROTE after owner-side evidence collection.
- `restore.py` never launches tmux or agents itself. It calls CHROTE's
  `/api/tmux/session-bank/{name}/recover` route for Session Bank owners, then
  recollects owner-side evidence and runs verification.
- `verify.py` checks the accepted manifest against observed Go-compatible
  descriptors, session-level pane verification records, managed status probes,
  and optional derived helper HTTP endpoints.
- `collector.py` emits bounded owner-side evidence for `owner_probe.py`: tmux
  pane topology/status, `/proc` process-tree argv/cwd/start metadata, and
  optional Hermes state-db candidate metadata. It does not read environments,
  transcript contents, or shell history.

Managed records are read-only manifest entries. A managed record uses
`managerKind: "systemd-user"`, a `managerRef` unit name, `restartAllowed:
false`, and a typed `statusProbe`. Restore health-checks those records; it does
not POST them to Session Bank and does not start or restart the unit.

Every managed record these tools write describes an externally owned,
non-restarting, read-only unit. ADR-0015 retires CHROTE's former per-session
supervision path; these tools do not make an external manager CHROTE-owned.

Manifest files use schema version `chrote.tmux-recovery.manifest.v1`; the JSON
schema lives next to this README as `recovery-manifest.schema.json`.
Descriptors contain only CHROTE/Go recovery fields. Operator-only pane status
and loopback helper probes live under a session `verification` array keyed by
captured pane target provenance. During restore verification, tmux
session/window/pane ids and raw indexes are not recovery identity; panes are
matched by normalized logical window order and pane order, then checked for
topology, workload identity, pane health, and helper probes.

Owner-side collection must run as the target Unix user. The tools check that
the current effective user matches `--unix-user`; they do not call privilege
escalation helpers.

For Session Bank collection, owner refs are derived per pane as
`<unixUser>/<sessionName>` from `tmux list-panes -a`; do not apply one owner ref
across a multi-session socket. External-manager collection requires an explicit
`--session-name` filter plus `--owner-ref`, or a typed manifest-only managed
record.

`--owner-kind` accepts `session_bank` and `external_manager`. No tool here can
produce the retired `persistent_agent` owner. Manifests written before that
change still load: an accepted manifest is an immutable artifact pinned to an
exact `schemaVersion`, so the value stays valid on read.

Snapshot normal path:

```bash
chrote-tmux-recovery-snapshot \
  --api-url http://127.0.0.1:8095 \
  --socket /run/user/2001/tmux/chrote \
  --unix-user alice \
  --owner-home /home/alice \
  --owner-kind session_bank \
  --owner-may-restart \
  --managed-records /path/to/managed-records.json \
  --output-dir /path/to/accepted-manifests
```

Snapshot validates the manifest in memory, posts every Session Bank plan, and
only then writes a new mode-0600 timestamped accepted manifest. If any CHROTE
API post fails, no accepted manifest is written. An explicit accepted baseline
preserves only validated baseline session records whose `(unixUser,
sessionName)` key is absent from current observations; current records replace
same-key baseline records. Arbitrary top-level/session metadata is rejected.

`--api-url` must be a sanitized origin. Loopback HTTP is allowed for local
operators; non-loopback origins must use HTTPS. Userinfo, query strings,
fragments, and non-root paths are rejected and never persisted into manifests.

Restore normal path:

```bash
chrote-tmux-recovery-restore \
  --api-url http://127.0.0.1:8095 \
  --manifest /path/to/accepted-manifest.json \
  --socket /run/user/2001/tmux/chrote \
  --unix-user alice \
  --owner-home /home/alice \
  --owner-kind session_bank \
  --owner-may-restart \
  --readiness-seconds 30 \
  --stability-seconds 30
```

Restore delegates Session Bank owners to CHROTE recovery APIs and health-checks
managed `systemd-user` owners. `restore.py` itself never starts tmux, launches
agents, starts or restarts units, or reads unit environments. Units that these
tools see as managed records stay read-only. `--topology-only` is explicit and
reports limited topology/cwd/dead verification rather than workload identity
success.

`restore` and `verify` first poll readiness for up to 30 seconds at a bounded
0.5-second interval, requiring exact topology, workload identity, pane health,
helper endpoint probes, and managed owner status to pass together. After
readiness succeeds, the separate 30-second stability interval takes an
independent second sample. Tests may pass `--readiness-seconds 0` and
`--stability-seconds 0`; operators should not use stale observed evidence to
certify recovery.

Fixture/offline modes remain available for tests and reviews:

```bash
chrote-tmux-recovery-snapshot --api-url http://127.0.0.1:8095 --input sessions.json --output-dir /tmp/manifests
chrote-tmux-recovery-restore --api-url http://127.0.0.1:8095 --manifest accepted.json --observed observed.json --allow-test-observed --readiness-seconds 0 --stability-seconds 0
chrote-tmux-recovery-verify --manifest accepted.json --observed observed.json --allow-test-observed --readiness-seconds 0 --stability-seconds 0
```

Disposable integration smoke:

```bash
python3 scripts/tmux-recovery/smoke_disposable.py
```

The smoke builds the current CHROTE server into a unique `/tmp` root, starts it
on random loopback ports with temporary recovery state, creates a unique tmux
socket plus fake owner home, runs real
snapshot/restore/verify CLI calls, and prints a concise JSON result. It may add a
unique current-user transient `systemd-run --user` sleep unit when user systemd
is available; otherwise the managed-owner leg is reported as an explicit skip.
It does not use configured live service ports, production data roots, existing
tmux sockets, sudo, or existing systemd units.

Install into an explicit prefix:

```bash
bash scripts/tmux-recovery/install.sh --prefix /tmp/chrote-recovery-tools
```

The install copies files only. It does not mutate CHROTE runtime state, tmux,
systemd units, or operator shell startup files.
