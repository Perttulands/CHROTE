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
  sessions: GasCitySession[]
  upstreamErrors?: GasCityUpstreamError[]
}

export interface GasCitySession {
  source?: 'gascity'
  city: string
  id: string
  name?: string
  title?: string
  alias?: string
  template?: string
  status?: string
  state?: string
  attachTarget?: string
  createdAt?: string
  lastActive?: string
  running: boolean
  attached: boolean
}

export interface GasCityUpstreamError {
  route: string
  message: string
}

export interface CreateGasCitySessionRequest {
  name: string
  template: string
  title?: string
}

export interface GasCityCreatedSession {
  source: 'gascity'
  schemaVersion?: string
  id: string
  name: string
  sessionName: string
  alias?: string
  title?: string
  template: string
  transport: string
  workDir: string
  deferredStart: boolean
  attached: boolean
  attachTarget: string
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

export function getGasCityObserver(init?: RequestInit) {
  return request<GasCityObserver>('/api/gascity/observer', init)
}

export function createGasCitySession(input: CreateGasCitySessionRequest, init?: RequestInit) {
  return request<GasCityCreatedSession>('/api/gascity/sessions', {
    ...init,
    method: 'POST',
    body: JSON.stringify(input),
  })
}
