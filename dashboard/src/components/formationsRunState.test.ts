import { describe, expect, it } from 'vitest'
import {
  activeRunStorageKey,
  projectNodeOutputs,
  runEventReportRef,
  runEventResumeAllowed,
  runEventText,
  runStatusFromResponse,
  statusFromRunEvent,
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

  it('extracts run text, report references, and resume affordance from events', () => {
    expect(runEventText({ runId: 'run_1', seq: 1, type: 'run_blocked', data: { reason: 'needs human' } })).toBe('needs human')
    expect(runEventText({ runId: 'run_1', seq: 2, type: 'node_output', data: { text: 'report body' } })).toBe('report body')
    expect(runEventReportRef({ runId: 'run_1', seq: 2, type: 'node_output', data: { reportRef: 'reports/fmn_work.md' } })).toBe('reports/fmn_work.md')
    expect(runEventResumeAllowed({ runId: 'run_1', seq: 3, type: 'run_blocked', data: { resumeAllowed: true } }, false)).toBe(true)
    expect(runEventResumeAllowed({ runId: 'run_1', seq: 4, type: 'run_failed' }, true)).toBe(false)
  })

  it('projects the per-output-port payloads from the routing contract, not the display summary', () => {
    // The only routing-truth payload is node_output.outputs[portId]; node_output.text is a
    // display-only summary and must never be surfaced as a port payload (FORMATIONS.md).
    const events: RunEvent[] = [
      { runId: 'run_1', seq: 1, type: 'run_started' },
      {
        runId: 'run_1',
        seq: 2,
        type: 'node_output',
        nodeId: 'fmn_work',
        data: {
          text: 'human summary that must not leak into ports',
          outputs: { port_work_out: 'the real produced payload' },
        },
      },
    ]

    const outputs = projectNodeOutputs(events)

    expect(outputs.get('fmn_work')?.get('port_work_out')).toBe('the real produced payload')
    // The display summary is not a port payload.
    expect(outputs.get('fmn_work')?.get('port_work_out')).not.toBe('human summary that must not leak into ports')
    // Ports that never produced an output are absent (the card keeps its honest idle state).
    expect(outputs.get('fmn_work')?.get('port_unproduced')).toBeUndefined()
    expect(outputs.get('fmn_other')).toBeUndefined()
  })

  it('keeps the latest output payload when a port is re-emitted', () => {
    const events: RunEvent[] = [
      { runId: 'run_1', seq: 1, type: 'node_output', nodeId: 'fmn_work', data: { outputs: { port_out: 'first' } } },
      { runId: 'run_1', seq: 2, type: 'node_output', nodeId: 'fmn_work', data: { outputs: { port_out: 'second' } } },
    ]
    expect(projectNodeOutputs(events).get('fmn_work')?.get('port_out')).toBe('second')
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
