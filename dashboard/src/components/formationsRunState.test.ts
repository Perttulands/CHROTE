import { describe, expect, it } from 'vitest'
import {
  activeRunStorageKey,
  projectNodeEvidence,
  runEventReportRef,
  runEventResumeAllowed,
  runEventText,
  runStatusFromResponse,
  statusFromRunEvent,
  projectNodeStates,
  upsertRunEvent,
} from './formationsRunState'
import type { RunEvent, RunStatusProjection } from './formationsTypes'

describe('formations run-state helpers', () => {
  it('deduplicates run events by run and sequence while keeping timeline order', () => {
    const first: RunEvent = { runId: 'run_1', seq: 2, type: 'node_started', nodeId: 'fmn_work' }
    const second: RunEvent = { runId: 'run_1', seq: 1, type: 'run_started' }
    const replacement: RunEvent = { runId: 'run_1', seq: 2, type: 'node_output', nodeId: 'fmn_work' }

    const events = upsertRunEvent(upsertRunEvent(upsertRunEvent([], first), second), replacement)

    expect(events.map(event => `${event.seq}:${event.type}`)).toEqual([
      '1:run_started',
      '2:node_output',
    ])
    expect(upsertRunEvent(events, { runId: '', seq: 3, type: 'node_started' })).toBe(events)
  })

  it('maps ledger events to status labels without turning in-flight blocks into final success', () => {
    expect(statusFromRunEvent({ runId: 'run_1', seq: 1, type: 'run_started' })).toBe('running')
    expect(statusFromRunEvent({ runId: 'run_1', seq: 2, type: 'run_blocked' })).toBe('blocked')
    expect(statusFromRunEvent({ runId: 'run_1', seq: 3, type: 'run_succeeded' })).toBe('succeeded')
    expect(statusFromRunEvent({ runId: 'run_1', seq: 4, type: 'node_output' })).toBe('')
  })

  it('keeps historical inline-verification verdicts non-authorizing in node projection', () => {
    const states = projectNodeStates([
      { runId: 'run_1', seq: 1, type: 'node_output', nodeId: 'fmn_work', data: { status: 'done' } },
      { runId: 'run_1', seq: 2, type: 'verification_verdict', nodeId: 'fmn_work', data: { verdict: 'fail' } },
    ], null)

    expect(states.get('fmn_work')).toBe('done')
  })

  it('extracts run text, report references, and resume affordance from events', () => {
    expect(runEventText({ runId: 'run_1', seq: 1, type: 'run_blocked', data: { reason: 'needs human' } })).toBe('needs human')
    expect(runEventText({ runId: 'run_1', seq: 2, type: 'node_output', data: { text: 'report body' } })).toBe('report body')
    expect(runEventReportRef({ runId: 'run_1', seq: 2, type: 'node_output', data: { reportRef: 'reports/fmn_work.md' } })).toBe('reports/fmn_work.md')
    expect(runEventResumeAllowed({ runId: 'run_1', seq: 3, type: 'run_blocked', data: { resumeAllowed: true } }, false)).toBe(true)
    expect(runEventResumeAllowed({ runId: 'run_1', seq: 4, type: 'run_failed' }, true)).toBe(false)
  })

  it('projects a formation node dispatch attempts, inline output, and reportRef from the ledger', () => {
    const evidence = projectNodeEvidence([
      { runId: 'run_1', seq: 1, type: 'run_started' },
      { runId: 'run_1', seq: 2, type: 'node_started', nodeId: 'fmn_work', attempt: 1, data: { reason: 'single-formation' } },
      { runId: 'run_1', seq: 3, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 1, data: { slotId: 'slot_lead', agentId: 'mason', harness: 'codex', phase: 'work', promptSha256: 'abcdef0123456789' } },
      { runId: 'run_1', seq: 4, type: 'node_output', nodeId: 'fmn_work', data: { status: 'done', text: 'the work output', reportRef: 'reports/fmn_work.md', outputs: { port_work_out: { text: 'the work output', reportRef: 'reports/fmn_work.md', ref: 'ledger://run_1/edge' } } } },
    ], 'fmn_work')

    expect(evidence.state).toBe('done')
    expect(evidence.attempts).toHaveLength(1)
    expect(evidence.attempts[0]).toMatchObject({ attempt: 1, startedSeq: 2, reason: 'single-formation' })
    expect(evidence.attempts[0].dispatches).toEqual([
      { seq: 3, slotId: 'slot_lead', agentId: 'mason', harness: 'codex', phase: 'work', promptSha256: 'abcdef0123456789', sessionRef: '' },
    ])
    expect(evidence.output).toMatchObject({ status: 'done', text: 'the work output', reportRef: 'reports/fmn_work.md' })
    expect(evidence.output?.ports).toEqual([
      { port: 'port_work_out', value: 'the work output', reportRef: 'reports/fmn_work.md', ref: 'ledger://run_1/edge', artifactRef: '' },
    ])
    expect(evidence.gateVerdict).toBeNull()
  })

  it('groups repeated dispatch attempts and captures the gate verdict evidence for a gate node', () => {
    const evidence = projectNodeEvidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: 'fmn_work', attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 1, data: { slotId: 'slot_lead', agentId: 'mason' } },
      { runId: 'run_1', seq: 3, type: 'node_started', nodeId: 'fmn_work', attempt: 2 },
      { runId: 'run_1', seq: 4, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 2, data: { slotId: 'slot_lead', agentId: 'mason' } },
      { runId: 'run_1', seq: 5, type: 'gate_verdict', nodeId: 'gate_review', gateId: 'gate_review', data: { verdict: 'fail', reason: 'needs another pass', routePort: 'fail', perKind: { code: 'fail', human: 'pass' } } },
    ], 'gate_review')

    expect(evidence.attempts).toHaveLength(0)
    expect(evidence.gateVerdict).toMatchObject({ verdict: 'fail', reason: 'needs another pass', routePort: 'fail' })
    expect(evidence.gateVerdict?.perKind).toEqual([['code', 'fail'], ['human', 'pass']])
    expect(evidence.state).toBe('failed')

    const workEvidence = projectNodeEvidence([
      { runId: 'run_1', seq: 1, type: 'node_started', nodeId: 'fmn_work', attempt: 1 },
      { runId: 'run_1', seq: 2, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 1, data: { slotId: 'slot_lead' } },
      { runId: 'run_1', seq: 3, type: 'node_started', nodeId: 'fmn_work', attempt: 2 },
      { runId: 'run_1', seq: 4, type: 'slot_dispatch', nodeId: 'fmn_work', attempt: 2, data: { slotId: 'slot_lead' } },
    ], 'fmn_work')
    expect(workEvidence.attempts.map(attempt => attempt.attempt)).toEqual([1, 2])
    expect(workEvidence.output).toBeNull()
  })

  it('normalizes run status envelopes and active run storage keys', () => {
    const flat: RunStatusProjection = {
      runId: 'run_flat',
      status: 'running',
      final: false,
      boardSlug: 'session-search',
      missionId: 'mis_showcase',
      eventCount: 1,
    }
    const nested = { status: { ...flat, runId: 'run_nested', eventCount: 2 } }

    expect(runStatusFromResponse(flat).runId).toBe('run_flat')
    expect(runStatusFromResponse(nested).runId).toBe('run_nested')
    expect(activeRunStorageKey('session-search')).toBe('chrote-formations-active-run-session-search')
  })
})
