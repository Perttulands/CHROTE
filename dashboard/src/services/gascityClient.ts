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
