# Direct bv prototype

One disposable experiment for `chrote-z9i1`: try the installed `bv` in CHROTE's
existing terminal hosting, with a host workspace and one project viewer.

From the repository root, set the current host API, guarded tmux executable,
absolute tmux socket and a private disposable state directory:

```bash
python3 dashboard/src/components/BeadsView/prototype-bv.py launch \
  --api-url "$CHROTE_API_URL" --socket "$CHROTE_TMUX_SOCKET" \
  --tmux "$CHROTE_TMUX_COMMAND" --state "$CHROTE_BV_PROTOTYPE_STATE"
```

Open `prototype-bv-host` or `prototype-bv-project` from CHROTE's Sessions sidecar.
The first tmux window is the actual viewer; the second shows source paths,
read times, counts, errors and refresh state. `Ctrl+b`, then `1`, opens sources;
`Ctrl+b`, then `0`, returns. `/` searches; `?` opens native viewer help. Host
workspace mode supports its native repository picker. The tmux status bar keeps
the snapshot/failure distinction visible while browsing.

The launcher discovers owning stores through `/api/workspaces`, verifies each
with `bd where`, and exports current regular issues through the installed `bd`
wrapper, at concurrency two. Source stores, settings and instructions are never
used as the viewer's writable directory. Viewer cwd is disposable `/tmp` state
(excluded from CHROTE's workspace discovery). Exports and workspace state stay
under the caller's private state directory; optional viewer cwd state may also
land in that disposable `/tmp` directory. No deployment is needed.

The host's sources window checks small source fingerprints every two seconds,
and reads only changed stores. Failed sources are retried after sixty seconds;
last successful snapshots remain visibly marked when a source fails. This
fingerprint loop is a prototype feed, not a production cache. Newly discovered
stores require relaunch. A manual full read is available with:

```bash
python3 dashboard/src/components/BeadsView/prototype-bv.py refresh \
  --state "$CHROTE_BV_PROTOTYPE_STATE"
```

The installed viewer's native host workspace mode does **not** live-reload or
support `Ctrl+r`; its loaded timestamp remains explicit. Press `q`, then Enter,
to reopen the newest snapshots. The project JSONL viewer does live-reload.
This difference is evidence for judging the proposed live-work experience.
Viewer edits operate on disposable snapshots and do not write back to live
work. Native issue prefixes are retained; duplicate prefixes across stores get
a path-key namespace, with original records retained in their source snapshots.

Judge loaded navigation separately from reading authoritative source stores.
Keep the prototype sessions available until the operator has tried them; remove
only these exact sessions and their disposable state when retiring the trial.
