# ADR-0005: Mission Boards

## Status
Accepted

## Context
Formations boards were a generic, multi-mission graph store: a board could hold
zero or many missions, and a run had to be told which mission to start. That left
the board with no stable identity — "the board" was a container, not a unit of
work — and forced every run, gallery summary, and API call to carry a mission
selector even when there was only one obvious mission.

Execution had a parallel problem. Beyond the deterministic lab executor, the only
way to reach real runtime sessions was a prod-smoke mode
(`CHROTE_FORMATIONS_TMUX_PROD_SMOKE`) that lifted the safety boundary so the
executor could target the **live CHROTE cockpit tmux socket**. Running mission
agents on the same socket humans use as their cockpit is dangerous: it blurs
ownership, risks colliding with operator sessions, and makes the golden rule (do
not disrupt running shells or tmux sessions) depend on the executor behaving.

The main alternatives were:
- keep generic multi-mission boards and keep passing a mission selector
  everywhere, accepting that a board has no single identity;
- keep prod-smoke against the cockpit socket as the runtime execution path;
- make the board's identity its single mission and give runtime execution its own
  dedicated socket isolated from the cockpit.

## Decision
A board is a **Mission Board**: it has exactly one mission, and that mission is
the board's identity. The board and its mission are created together in one atomic
write (`Store.CreateMissionBoard`, `POST /api/formations/boards`,
`archon board new <slug> --title <t> --goal <g>`); there is never an empty board
on disk. Adding a second mission is refused, and `ValidateBoard` records a
`mission_count` error when a board does not have exactly one mission, surfaced by
`archon board validate` (non-zero exit) and `doctor files`. Running a board runs
its single mission; no mission selector is required.

Runtime execution moves to a dedicated formations tmux socket
(`CHROTE_FORMATIONS_TMUX_DEDICATED=1` with `CHROTE_FORMATIONS_TMUX_SOCKET`,
sessions prefixed `mission-`), isolated from the cockpit socket. The executor
always refuses the cockpit socket. Prod-smoke against the live cockpit socket is
retired; the execution ladder is now lab (synthetic) → dedicated formations
socket (runtime). The CHROTE surface is the **Missions** tab — a Mission Board
gallery with a session side-panel bound to the dedicated socket — not a generic
board list.

## Consequences
The board becomes the unit of work a human points at: one board, one mission, one
goal, renderable as a gallery card with goal plus latest run and runnable without
a selector. The one-mission invariant is enforced on write and on validation
rather than assumed, so a malformed board fails loud instead of running
ambiguously. Mission agents can no longer touch the cockpit socket by
construction, so the golden rule no longer depends on prod-smoke discipline.

The trade-off is that existing multi-mission boards are not migrated: there is no
automatic split into one-mission boards, so any such board (the demo boards) is
burned and must be recreated as Mission Boards. The empty-board constructor
(`Store.CreateBoard`/`BoardCreateRequest`) is removed as dead and contradictory,
and reaching runtime sessions now requires provisioning a dedicated socket rather
than flipping a prod-smoke flag against the live cockpit.
