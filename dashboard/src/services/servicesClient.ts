export interface ApiErrorPayload {
  code: string
  message: string
}

interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: ApiErrorPayload
}

export class ServicesApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'ServicesApiError'
    this.code = code
    this.status = status
  }
}

export interface ServiceStatus {
  id: 'tts' | 'context' | string
  name: string
  status: string
  message?: string
  configured: boolean
  tokenConfigured?: boolean
  capabilities: string[]
}

export interface ServicesCatalog {
  services: ServiceStatus[]
}

export interface TTSHealth {
  status?: string
  ok?: boolean
  messages?: number
  clients?: number
}

export interface TTSMessage {
  id: string
  text: string
  source?: string
  backend?: string
  voice?: string
  status: 'queued' | 'generating' | 'ready' | 'error' | string
  audioUrl?: string
  duration?: number
  error?: string
  createdAt?: string
  generatedAt?: string
}

export interface TTSMessagesResponse {
  messages: TTSMessage[]
}

export interface TTSEnqueueRequest {
  text: string
  source: string
  backend: string
  voice: string
}

export interface TTSEnqueueResponse {
  id: string
  status: string
}

export interface ContextDocSummary {
  path: string
  size?: number
  modified?: string
  title?: string
}

export interface ContextDocsResponse {
  docs: ContextDocSummary[]
}

export interface ContextDoc {
  path: string
  meta?: Record<string, unknown>
  content: string
  modified?: string
}

export interface ContextHistoryEntry {
  hash: string
  date?: string
  subject?: string
  message?: string
}

export interface ContextHistoryResponse {
  path: string
  history: ContextHistoryEntry[]
}

export interface ContextAskSource {
  path: string
  title?: string
  line?: number
  snippet?: string
}

export interface ContextAskResponse {
  answer: string
  sources: ContextAskSource[]
}

export interface ContextGrant {
  id: string
  name?: string
  purpose?: string
  client_type?: string
  status?: string
  scopes?: string[]
  constraints?: Record<string, unknown>
  egress?: Record<string, unknown>
  limits?: Record<string, unknown>
  expires_at?: string | null
  token_fingerprint?: string
  created_at?: string
  updated_at?: string
  revoked_at?: string | null
}

export interface ContextGrantsResponse {
  grants: ContextGrant[]
}

export type ContextGrantCreateRequest = Record<string, unknown>

export interface ContextGrantTokenResponse {
  token?: string
  grant: ContextGrant
}

export interface ContextGrantPreviewRequest {
  grant_id?: string
  grant?: ContextGrant | Record<string, unknown>
  question?: string
  filters?: Record<string, unknown>
  limits?: Record<string, unknown>
  egress?: Record<string, unknown>
  include_baseline?: boolean
}

export interface ContextPreviewChunk {
  canonical_path?: string
  path?: string
  document_id?: string
  title?: string
  sensitivity?: string
  lifecycle?: string
  snippet?: string
}

export interface ContextPreviewDenied {
  canonical_path?: string
  path?: string
  reason?: string
}

export interface ContextEgressPlan {
  allowed?: boolean
  reason?: string
  provider?: string
  model?: string
  total_prompt_chars?: number
}

export interface ContextGrantPreviewResponse {
  preview: {
    chunks?: ContextPreviewChunk[]
    denied?: ContextPreviewDenied[]
    egress_plan?: ContextEgressPlan
  }
}

export interface ContextIngestionItem {
  path: string
  id?: string
  title?: string
  domain?: string
  kind?: string
  sensitivity?: string
  lifecycle?: string
  provenance?: string
  review_status?: string
  prompt_injection_risk?: string
  source_type?: string
  source_url?: string
  proposed_targets?: string[]
}

export interface ContextIngestionQueueResponse {
  items: ContextIngestionItem[]
}

export interface ContextIngestionReviewResponse {
  item: ContextIngestionItem
}

export interface ContextAuditEvent {
  id?: string
  type: string
  timestamp?: string
  actor?: string
  principal_role?: string
  grant_id?: string
  operation?: string
  details?: Record<string, unknown>
}

export interface ContextAuditResponse {
  events: ContextAuditEvent[]
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })

  let envelope: ApiEnvelope<T>
  try {
    envelope = await response.json()
  } catch {
    throw new ServicesApiError('SERVICE_INVALID_RESPONSE', 'Service returned invalid JSON', response.status)
  }

  if (!response.ok || !envelope.success) {
    const code = envelope.error?.code || 'SERVICE_REQUEST_FAILED'
    const message = envelope.error?.message || `Service request failed with status ${response.status}`
    throw new ServicesApiError(code, message, response.status)
  }

  return envelope.data as T
}

function encodeContextPath(path: string): string {
  return encodeURIComponent(path)
}

export function listServices() {
  return request<ServicesCatalog>('/api/services')
}

export function getTTSHealth() {
  return request<TTSHealth>('/api/services/tts/health')
}

export function getTTSMessages() {
  return request<TTSMessagesResponse>('/api/services/tts/messages')
}

export function enqueueTTS(body: TTSEnqueueRequest) {
  return request<TTSEnqueueResponse>('/api/services/tts/enqueue', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function getContextDocs() {
  return request<ContextDocsResponse>('/api/services/context/docs')
}

export function readContextDoc(path: string) {
  return request<ContextDoc>(`/api/services/context/docs/${encodeContextPath(path)}`)
}

export function saveContextDoc(path: string, content: string) {
  return request<{ ok: boolean; path: string }>(`/api/services/context/docs/${encodeContextPath(path)}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  })
}

export function getContextHistory(path: string) {
  return request<ContextHistoryResponse>(`/api/services/context/history/${encodeContextPath(path)}`)
}

export function askContext(question: string) {
  return request<ContextAskResponse>('/api/services/context/ask', {
    method: 'POST',
    body: JSON.stringify({ question }),
  })
}

export function getContextGrants() {
  return request<ContextGrantsResponse>('/api/services/context/grants')
}

export function createContextGrant(body: ContextGrantCreateRequest) {
  return request<ContextGrantTokenResponse>('/api/services/context/grants', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function revokeContextGrant(id: string) {
  return request<{ grant: ContextGrant }>(`/api/services/context/grants/${encodeURIComponent(id)}/revoke`, {
    method: 'POST',
  })
}

export function rotateContextGrant(id: string) {
  return request<ContextGrantTokenResponse>(`/api/services/context/grants/${encodeURIComponent(id)}/rotate`, {
    method: 'POST',
  })
}

export function previewContextGrant(body: ContextGrantPreviewRequest) {
  return request<ContextGrantPreviewResponse>('/api/services/context/grants/preview', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function getContextIngestionQueue() {
  return request<ContextIngestionQueueResponse>('/api/services/context/ingestion/queue')
}

export function approveContextIngestionCandidate(path: string) {
  return request<ContextIngestionReviewResponse>(`/api/services/context/ingestion/candidates/${encodeContextPath(path)}/approve`, {
    method: 'POST',
  })
}

export function rejectContextIngestionCandidate(path: string, reason?: string) {
  return request<ContextIngestionReviewResponse>(`/api/services/context/ingestion/candidates/${encodeContextPath(path)}/reject`, {
    method: 'POST',
    body: JSON.stringify(reason ? { reason } : {}),
  })
}

export function getContextAudit(limit = 25) {
  return request<ContextAuditResponse>(`/api/services/context/audit?limit=${encodeURIComponent(String(limit))}`)
}
