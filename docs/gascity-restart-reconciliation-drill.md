# Gas City Restart Reconciliation Drill

Date: 2026-05-27
Bead: `home-ukyt`
Nonce: `C5R-20260527-012907`

## Scope

This drill tested the live `/home/perttu/gascity` sidecar with disposable
workflow and mail state. It did not edit Gas City runtime files by hand, expose
the supervisor, touch CHROTE tmux sessions, or use paid harnesses for new work.

The restart was announced before execution. The command used was:

```bash
gc restart /home/perttu/gascity
```

## Before Restart

Read-only baseline:

```bash
gc status
gc session list --state all
gc mail count human
gc events --seq
sha256sum /home/perttu/gascity/.gc/beads.json
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux -L gascity ls
tmux -S /run/user/1000/chrote-tmux/tmux-1000/default ls | wc -l
```

Observed state:

- supervisor PID: `3703159`
- Gas City sessions: 4 active, 0 suspended
- active Gas City session IDs: `gc-51923`, `gc-4171`, `gc-4169`, `gc-4168`
- Gas City tmux sessions: `planner`, `reviewer-a`, `reviewer-b`,
  `s-gc-51923`
- CHROTE tmux session count on the chrote socket: `25`
- human mail count: `6 total`, `2 unread`
- event cursor: `105074`
- pre-work store hash:
  `e126cc76a9f6ab58e21e1e0173f86ef0251446df91900b72fc31fc4f31fb5150`

Transcript baseline:

- `gc session peek planner --lines 20` recovered recent tmux output.
- `gc session logs planner --tail 20` failed with:
  `session "gc-4168" has no session_key and workdir fallback is ambiguous`.

## Disposable State Created

Formula:

```bash
gc formula cook plan-review-synthesis \
  --title "C5 restart drill C5R-20260527-012907" \
  --meta chrote.restart_drill=C5R-20260527-012907
```

Created:

- root molecule: `gc-52557`
- steps: `gc-52559`, `gc-52562`, `gc-52563`, `gc-52564`

Mail:

```bash
gc mail send human \
  -s "C5 restart drill C5R-20260527-012907" \
  -m "Disposable restart reconciliation mail for C5R-20260527-012907"
```

Created:

- message: `gc-52560`

Pre-restart after disposable work:

- human mail count: `7 total`, `3 unread`
- event cursor: `105081`
- store hash:
  `02bee5ce192349d268f6cd745d940129ac6f74d55ba0af61b79d8fdebd8def21`

## Restart

Command:

```bash
gc restart /home/perttu/gascity
```

Observed output summary:

- unregistered city `gascity`
- triggered reconciliation
- stopped city
- supervisor process reported running
- registered city `gascity`
- installed or refreshed user systemd service
- adopted sessions
- city started under supervisor

No CHROTE tmux sessions were restarted by this command.

## After Restart

Read-only comparison:

```bash
gc status
gc session list --state all
gc mail count human
gc mail inbox human
jq -c '.beads[] | select(.id=="gc-52557" or .id=="gc-52559" or .id=="gc-52562" or .id=="gc-52563" or .id=="gc-52564" or .id=="gc-52560") | {id,title,status,issue_type,assignee,from,description,labels,metadata}' /home/perttu/gascity/.gc/beads.json
gc events --seq
TMUX_TMPDIR=/run/user/1000/chrote-tmux tmux -L gascity ls
tmux -S /run/user/1000/chrote-tmux/tmux-1000/default ls | wc -l
gc session peek planner --lines 30
gc session logs planner --tail 20
gc doctor --verbose
```

Observed state:

- supervisor PID changed to `3826452`
- Gas City sessions: 4 active, 0 suspended
- session IDs preserved: `gc-51923`, `gc-4171`, `gc-4169`, `gc-4168`
- Gas City tmux sessions recreated with fresh creation timestamps:
  `planner`, `reviewer-a`, `reviewer-b`, `s-gc-51923`
- CHROTE tmux session count remained `25`
- human mail count remained `7 total`, `3 unread`
- disposable mail `gc-52560` remained unread in `gc mail inbox human`
- disposable formula root/steps remained present and open:
  `gc-52557`, `gc-52559`, `gc-52562`, `gc-52563`, `gc-52564`
- event cursor advanced to `105096`
- post-comparison store hash:
  `9b80ae7861cde6b274ce0d098f89d75bc37bc75d985e6c14eba90c9b6ff8db0e`
- `gc doctor --verbose` passed 46 checks

Transcript comparison:

- `gc session peek planner --lines 30` recovered the restarted mock-agent
  output from tmux.
- `gc session logs planner --tail 20` still failed with the same
  no-`session_key` / ambiguous workdir limitation.

## Recommendation

Gas City restart reconciliation is acceptable for the current CHROTE 3.0
central-workflow pilot if CHROTE treats `gc session peek` or a CHROTE-side
adapter as the transcript recovery path for tmux-backed sessions.

The drill did not silently lose workflow beads, mail, session IDs, or operator
visibility. It did recreate the underlying tmux windows, so raw tmux creation
timestamps and transient pane scrollback are not durable evidence. The durable
state that mattered for the pilot was still available through Gas City ids and
the file-backed bead store after restart.

Do not rely on `gc session logs` for tmux/mock transcript recovery until the
known `session_key` limitation is fixed upstream or bypassed by CHROTE.
