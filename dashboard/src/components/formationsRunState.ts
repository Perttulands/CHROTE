import type { RunEvent, RunStatusProjection, RunStatusResult } from './formationsTypes'

export const runEventTypes = [
  'run_started',
  'run_resumed',
  'node_waiting',
  'node_started',
  'slot_dispatch',
  'slot_result',
  'node_output',
  'gate_evaluating',
  'gate_verdict',
  'human_input_requested',
  'human_verdict_recorded',
  'verification_verdict',
  'escalation_raised',
  'error',
  'run_blocked',
  'run_canceled',
  'run_failed',
  'run_succeeded',
]

export function upsertRunEvent(events: RunEvent[], next: RunEvent): RunEvent[] {
  if (!next.runId || !next.seq) return events
  const existing = events.findIndex(event => event.seq === next.seq && event.runId === next.runId)
  const merged = existing >= 0
    ? events.map((event, index) => index === existing ? next : event)
    : [...events, next]
  return merged.sort((a, b) => a.seq - b.seq)
}

export function statusFromRunEvent(event: RunEvent): string {
  switch (event.type) {
    case 'run_started':
    case 'run_resumed':
      return 'running'
    case 'run_blocked':
      return 'blocked'
    case 'run_canceled':
      return 'canceled'
    case 'run_failed':
      return 'failed'
    case 'run_succeeded':
      return 'succeeded'
    default:
      return ''
  }
}

export function runEventResumeAllowed(event: RunEvent, fallback: boolean): boolean {
  if (event.type === 'run_blocked') return event.data?.resumeAllowed === true
  if (event.type === 'run_started' || event.type === 'run_resumed') return false
  if (event.type === 'run_canceled' || event.type === 'run_failed' || event.type === 'run_succeeded') return false
  return fallback
}

export function runEventText(event: RunEvent): string {
  const data = event.data || {}
  if (typeof data.text === 'string') return data.text
  if (typeof data.reason === 'string') return data.reason
  if (typeof data.prompt === 'string') return data.prompt
  if (typeof data.error === 'string') return data.error
  return ''
}

export function runStatusFromResponse(data: RunStatusProjection | RunStatusResult): RunStatusProjection {
  const nested = (data as RunStatusResult).status
  return typeof nested === 'object' && nested !== null ? nested : data as RunStatusProjection
}

export function runEventReportRef(event: RunEvent): string {
  const reportRef = event.data?.reportRef
  return typeof reportRef === 'string' ? reportRef : ''
}

export function activeRunStorageKey(slug: string): string {
  return `chrote-formations-active-run-${slug}`
}

export type NodeRunState = '' | 'running' | 'done' | 'blocked' | 'waiting' | 'failed'

/** Project honest per-node run state from the ledger events (mirrors the engine vocabulary). */
export function projectNodeStates(events: RunEvent[], activeRun: RunStatusProjection | null): Map<string, NodeRunState> {
  const map = new Map<string, NodeRunState>()
  for (const event of events) {
    const nodeId = event.nodeId || event.gateId
    if (!nodeId) continue
    switch (event.type) {
      case 'node_started':
      case 'slot_dispatch':
      case 'gate_evaluating':
        map.set(nodeId, 'running')
        break
      case 'node_waiting':
        map.set(nodeId, 'waiting')
        break
      case 'node_output': {
        const status = typeof event.data?.status === 'string' ? event.data.status : 'done'
        map.set(nodeId, status === 'blocked' ? 'blocked' : 'done')
        break
      }
      case 'gate_verdict': {
        const verdict = typeof event.data?.verdict === 'string' ? event.data.verdict : ''
        map.set(nodeId, verdict === 'fail' ? 'failed' : 'done')
        break
      }
      case 'run_blocked':
        map.set(nodeId, 'blocked')
        break
      case 'run_failed':
        map.set(nodeId, 'failed')
        break
      default:
        break
    }
  }
  if (activeRun && !activeRun.final && activeRun.status === 'running') {
    // leave node states as projected
  }
  return map
}

/**
 * Project the concrete per-output-port payloads produced by the run.
 *
 * The only routing-truth payload is `node_output.outputs[portId]` (FORMATIONS.md). The free-form
 * `node_output.text` is a display summary and is deliberately NOT consumed here — it must never be
 * surfaced as a port payload. A port absent from the map genuinely has no produced output yet.
 * Re-emitted ports keep their latest payload (events are processed in seq order).
 */
export function projectNodeOutputs(events: RunEvent[]): Map<string, Map<string, string>> {
  const map = new Map<string, Map<string, string>>()
  for (const event of events) {
    if (event.type !== 'node_output' || !event.nodeId) continue
    const outputs = event.data?.outputs
    if (typeof outputs !== 'object' || outputs === null) continue
    let ports = map.get(event.nodeId)
    if (!ports) {
      ports = new Map<string, string>()
      map.set(event.nodeId, ports)
    }
    for (const [portId, payload] of Object.entries(outputs as Record<string, unknown>)) {
      if (typeof payload === 'string') ports.set(portId, payload)
    }
  }
  return map
}

export function openHumanGateId(events: RunEvent[]): string {
  let openGateId = ''
  for (const event of [...events].sort((a, b) => a.seq - b.seq)) {
    if (event.type === 'human_input_requested' && event.gateId) {
      openGateId = event.gateId
      continue
    }
    if ((event.type === 'human_verdict_recorded' || event.type === 'gate_verdict') && event.gateId === openGateId) {
      openGateId = ''
    }
    if (event.type === 'run_succeeded' || event.type === 'run_failed' || event.type === 'run_canceled') {
      openGateId = ''
    }
  }
  return openGateId
}
