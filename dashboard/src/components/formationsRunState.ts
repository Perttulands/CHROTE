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

function evidenceString(data: Record<string, unknown> | undefined, key: string): string {
  const value = data?.[key]
  return typeof value === 'string' ? value : ''
}

export interface NodeEvidenceDispatch {
  seq: number
  slotId: string
  agentId: string
  harness: string
  phase: string
  promptSha256: string
  sessionRef: string
}

export interface NodeEvidenceAttempt {
  attempt: number
  startedSeq: number | null
  reason: string
  dispatches: NodeEvidenceDispatch[]
}

export interface NodeEvidenceOutputPort {
  port: string
  value: string
  reportRef: string
  ref: string
  artifactRef: string
}

export interface NodeEvidenceOutput {
  seq: number
  status: string
  text: string
  reportRef: string
  ports: NodeEvidenceOutputPort[]
}

export interface NodeEvidenceGateVerdict {
  seq: number
  verdict: string
  reason: string
  routePort: string
  perKind: [string, string][]
}

export interface NodeEvidence {
  nodeId: string
  state: NodeRunState
  attempts: NodeEvidenceAttempt[]
  output: NodeEvidenceOutput | null
  gateVerdict: NodeEvidenceGateVerdict | null
  eventCount: number
}

/**
 * Project a single node's inspectable evidence straight from the run ledger the
 * API already returns: dispatch attempts (node_started + slot_dispatch, grouped
 * by attempt), its latest output (inline value + reportRef + per-port payloads),
 * and any gate verdict with its per-kind evidence. Reads only what the backend
 * emits — no new endpoint, no new projection.
 */
export function projectNodeEvidence(events: RunEvent[], nodeId: string): NodeEvidence {
  const sorted = [...events].sort((a, b) => a.seq - b.seq)
  const attempts = new Map<number, NodeEvidenceAttempt>()
  const order: number[] = []
  const ensureAttempt = (attempt: number): NodeEvidenceAttempt => {
    const key = attempt || 1
    let entry = attempts.get(key)
    if (!entry) {
      entry = { attempt: key, startedSeq: null, reason: '', dispatches: [] }
      attempts.set(key, entry)
      order.push(key)
    }
    return entry
  }
  let output: NodeEvidenceOutput | null = null
  let gateVerdict: NodeEvidenceGateVerdict | null = null
  let eventCount = 0
  for (const event of sorted) {
    if ((event.nodeId || event.gateId) !== nodeId) continue
    eventCount++
    switch (event.type) {
      case 'node_started': {
        const entry = ensureAttempt(event.attempt || 1)
        entry.startedSeq = event.seq
        entry.reason = evidenceString(event.data, 'reason')
        break
      }
      case 'slot_dispatch': {
        const entry = ensureAttempt(event.attempt || 1)
        entry.dispatches.push({
          seq: event.seq,
          slotId: evidenceString(event.data, 'slotId'),
          agentId: evidenceString(event.data, 'agentId'),
          harness: evidenceString(event.data, 'harness'),
          phase: evidenceString(event.data, 'phase'),
          promptSha256: evidenceString(event.data, 'promptSha256'),
          sessionRef: evidenceString(event.data, 'sessionRef'),
        })
        break
      }
      case 'node_output': {
        const rawOutputs = event.data?.outputs
        const ports: NodeEvidenceOutputPort[] = []
        if (rawOutputs && typeof rawOutputs === 'object') {
          for (const [port, payload] of Object.entries(rawOutputs as Record<string, unknown>)) {
            const fields = (payload && typeof payload === 'object' ? payload : {}) as Record<string, unknown>
            ports.push({
              port,
              value: typeof fields.text === 'string' ? fields.text : '',
              reportRef: typeof fields.reportRef === 'string' ? fields.reportRef : '',
              ref: typeof fields.ref === 'string' ? fields.ref : '',
              artifactRef: typeof fields.artifactRef === 'string' ? fields.artifactRef : '',
            })
          }
        }
        ports.sort((a, b) => a.port.localeCompare(b.port))
        output = {
          seq: event.seq,
          status: evidenceString(event.data, 'status') || 'done',
          text: evidenceString(event.data, 'text'),
          reportRef: runEventReportRef(event),
          ports,
        }
        break
      }
      case 'gate_verdict': {
        const rawPerKind = event.data?.perKind
        const perKind: [string, string][] = rawPerKind && typeof rawPerKind === 'object'
          ? Object.entries(rawPerKind as Record<string, unknown>).map(([kind, result]) => [
              kind,
              typeof result === 'string' ? result : String(result),
            ])
          : []
        gateVerdict = {
          seq: event.seq,
          verdict: evidenceString(event.data, 'verdict'),
          reason: evidenceString(event.data, 'reason'),
          routePort: evidenceString(event.data, 'routePort'),
          perKind,
        }
        break
      }
      default:
        break
    }
  }
  return {
    nodeId,
    state: projectNodeStates(sorted, null).get(nodeId) || '',
    attempts: order.map(key => attempts.get(key) as NodeEvidenceAttempt),
    output,
    gateVerdict,
    eventCount,
  }
}

export interface PeekTarget {
  sessionName: string
  sessionRef: string
}

/**
 * Decide whether a node's live agent tmux session is attachable right now.
 * Peek is honest: the node must be running AND its latest dispatch must name a
 * tmux session that is still present in the live terminal registry. Formations
 * sessions are ephemeral — spawned per step and torn down after — so "ran once"
 * is never enough; the session name has to still be live to attach. Non-tmux
 * refs (e.g. lab runs, "lab:...") have no attachable tmux session and are
 * refused. Returns the session to attach, or null when peek is not available.
 */
export function peekTargetForEvidence(
  evidence: NodeEvidence | null,
  liveSessionNames: ReadonlySet<string>,
): PeekTarget | null {
  if (!evidence || evidence.state !== 'running') return null
  const tmuxPrefix = 'tmux:'
  for (let a = evidence.attempts.length - 1; a >= 0; a--) {
    const dispatches = evidence.attempts[a].dispatches
    for (let d = dispatches.length - 1; d >= 0; d--) {
      const sessionRef = dispatches[d].sessionRef
      if (!sessionRef.startsWith(tmuxPrefix)) continue
      const sessionName = sessionRef.slice(tmuxPrefix.length)
      if (!sessionName) continue
      // The latest dispatched session is the only one that can still be live;
      // if it is gone from the registry, the step has been torn down — do not
      // fall back to an older attempt's (already dead) session.
      return liveSessionNames.has(sessionName) ? { sessionName, sessionRef } : null
    }
  }
  return null
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
