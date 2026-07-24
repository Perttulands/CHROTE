import { describe, expect, it } from 'vitest'
import { projectNodeEvidence, peekTargetForEvidence } from './formationsRunState'
import type { RunEvent } from './formationsTypes'

/* RED: grab-the-wheel peek gate (chrote-a7k).
   peekTargetForEvidence is the honest gate that decides whether a node's live
   agent tmux session is attachable RIGHT NOW: the node must be running AND the
   latest dispatch's tmux sessionRef must name a session that is currently live
   in the terminal registry. Ephemeral sessions (spawned per-step, torn down
   after) mean "ran once" is never enough. */

const NODE = 'fmn_work'

function evidence(events: RunEvent[]) {
  return projectNodeEvidence(events, NODE)
}

describe('peekTargetForEvidence', () => {
  it('offers the live tmux session while the node is running and the session is live', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { slotId: 'slot_lead', sessionRef: 'tmux:mission-run1-lead-ab12' } },
    ])
    expect(peekTargetForEvidence(ev, new Set(['mission-run1-lead-ab12']))).toEqual({
      sessionName: 'mission-run1-lead-ab12',
      sessionRef: 'tmux:mission-run1-lead-ab12',
    })
  })

  it('withholds peek when the session is not (yet/no longer) in the live registry', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { sessionRef: 'tmux:mission-run1-lead-ab12' } },
    ])
    expect(peekTargetForEvidence(ev, new Set())).toBeNull()
  })

  it('withholds peek once the node has finished even if a name collision stays live', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { sessionRef: 'tmux:mission-run1-lead-ab12' } },
      { runId: 'run_1', seq: 3, type: 'node_output', nodeId: NODE, data: { status: 'done' } },
    ])
    expect(peekTargetForEvidence(ev, new Set(['mission-run1-lead-ab12']))).toBeNull()
  })

  it('picks the latest attempt session when a node was re-dispatched', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { sessionRef: 'tmux:mission-old' } },
      { runId: 'run_1', seq: 3, type: 'node_started', nodeId: NODE, attempt: 2 },
      { runId: 'run_1', seq: 4, type: 'slot_dispatch', nodeId: NODE, attempt: 2, data: { sessionRef: 'tmux:mission-new' } },
    ])
    expect(peekTargetForEvidence(ev, new Set(['mission-old', 'mission-new']))?.sessionName).toBe('mission-new')
  })

  it('does not peek a running node that has no dispatched session yet', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
    ])
    expect(peekTargetForEvidence(ev, new Set(['mission-run1-lead-ab12']))).toBeNull()
  })

  it('refuses non-tmux sessionRefs (lab runs have no attachable tmux session)', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { sessionRef: 'lab:scout' } },
    ])
    expect(peekTargetForEvidence(ev, new Set(['scout', 'lab:scout']))).toBeNull()
  })

  it('treats a blank sessionRef as not attachable', () => {
    const ev = evidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: NODE, attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: NODE, attempt: 1, data: { sessionRef: '' } },
    ])
    expect(peekTargetForEvidence(ev, new Set(['']))).toBeNull()
  })
})
