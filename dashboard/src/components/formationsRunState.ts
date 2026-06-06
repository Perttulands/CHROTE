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
