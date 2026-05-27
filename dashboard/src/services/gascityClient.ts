export interface ApiErrorPayload {
  code: string
  message: string
}

interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: ApiErrorPayload
}

export class GasCityApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'GasCityApiError'
    this.code = code
    this.status = status
  }
}

export interface GasCityObserver {
  status: string
  checkedAt: string
  error?: string
  health: GasCityHealth
  cities: GasCityCity[]
  sessions: GasCitySession[]
  mail: GasCityMailCounts
  work: GasCityWorkCounts
  formulas: GasCityFormula[]
  molecules: GasCityWorkItem[]
  wisps: GasCityWorkItem[]
  convoys: GasCityWorkItem[]
  recentEvents: GasCityEvent[]
  upstreamErrors?: GasCityUpstreamError[]
}

export interface GasCityHealth {
  status: string
  version?: string
  buildId?: string
  uptimeSeconds?: number
  citiesTotal: number
  citiesRunning: number
  startupReady: boolean
  startupPhase?: string
}

export interface GasCityCity {
  name: string
  path?: string
  running: boolean
  status?: string
  error?: string
}

export interface GasCitySession {
  city: string
  id: string
  title?: string
  alias?: string
  template?: string
  state?: string
  provider?: string
  sessionName?: string
  createdAt?: string
  lastActive?: string
  running: boolean
  attached: boolean
}

export interface GasCityMailCounts {
  total: number
  unread: number
}

export interface GasCityWorkCounts {
  open: number
  ready: number
  inProgress: number
  routed: number
  molecules: number
  wisps: number
  convoys: number
}

export interface GasCityFormula {
  city: string
  name: string
  description?: string
  version?: string
  runCount: number
}

export interface GasCityWorkItem {
  city: string
  id: string
  title?: string
  status?: string
  issueType?: string
  ref?: string
  routedTo?: string
  createdAt?: string
}

export interface GasCityEvent {
  city?: string
  seq: number
  type: string
  time?: string
  actor?: string
  subject?: string
}

export interface GasCityUpstreamError {
  route: string
  message: string
}

export interface GasCityMailList {
  recipient: string
  limit: number
  messages: GasCityMailMessage[]
}

export interface GasCityMailMessage {
  id: string
  from?: string
  recipient: string
  subject?: string
  body: string
  bodyTruncated: boolean
  status?: string
  issueType?: string
  read: boolean
  fromSessionId?: string
  createdAt?: string
  updatedAt?: string
}

export interface GasCityPiPoemRequest {
  topic: string
}

export interface GasCityPiPoemResponse {
  nonce: string
  subject: string
  target: string
  targetAlias?: string
  targetTemplate?: string
  targetSessionId?: string
  recipient: string
  output: string
}

export interface GasCityTranscript {
  source: string
  stale: boolean
  sessionId: string
  alias?: string
  template?: string
  state?: string
  city?: string
  lines: number
  lineCount: number
  capturedAt?: string
  transcript: string
  truncated: boolean
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = init
    ? await fetch(path, {
        ...init,
        headers: {
          ...(init.body ? { 'Content-Type': 'application/json' } : {}),
          ...init.headers,
        },
      })
    : await fetch(path)

  let envelope: ApiEnvelope<T>
  try {
    envelope = await response.json()
  } catch {
    throw new GasCityApiError('GASCITY_INVALID_RESPONSE', 'Gas City returned invalid JSON', response.status)
  }

  if (!response.ok || !envelope.success) {
    const code = envelope.error?.code || 'GASCITY_REQUEST_FAILED'
    const message = envelope.error?.message || `Gas City request failed with status ${response.status}`
    throw new GasCityApiError(code, message, response.status)
  }

  return envelope.data as T
}

export function getGasCityObserver() {
  return request<GasCityObserver>('/api/gascity/observer')
}

export function getGasCityMail(recipient = 'human', limit = 20) {
  const query = new URLSearchParams({ recipient, limit: String(limit) })
  return request<GasCityMailList>(`/api/gascity/mail?${query.toString()}`)
}

export function sendGasCityPiPoem(body: GasCityPiPoemRequest) {
  return request<GasCityPiPoemResponse>('/api/gascity/requests/pi-poem', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function getGasCityTranscript(sessionId: string, lines = 120) {
  const query = new URLSearchParams({ lines: String(lines) })
  return request<GasCityTranscript>(`/api/gascity/sessions/${encodeURIComponent(sessionId)}/transcript?${query.toString()}`)
}
