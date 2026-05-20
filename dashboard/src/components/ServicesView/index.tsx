import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ServicesApiError,
  approveContextIngestionCandidate,
  askContext,
  createContextGrant,
  enqueueTTS,
  getContextAudit,
  getContextDocs,
  getContextHistory,
  getContextGrants,
  getContextIngestionQueue,
  getTTSHealth,
  getTTSMessages,
  listServices,
  previewContextGrant,
  readContextDoc,
  rejectContextIngestionCandidate,
  revokeContextGrant,
  rotateContextGrant,
  saveContextDoc,
  type ContextAskResponse,
  type ContextAuditEvent,
  type ContextDoc,
  type ContextDocSummary,
  type ContextHistoryEntry,
  type ContextGrant,
  type ContextGrantPreviewResponse,
  type ContextIngestionItem,
  type ServiceStatus,
  type TTSHealth,
  type TTSMessage,
} from '../../services/servicesClient'

function errorMessage(error: unknown): string {
  if (error instanceof ServicesApiError) return error.message
  if (error instanceof Error) return error.message
  return 'Service request failed'
}

function statusLabel(service?: ServiceStatus): string {
  if (!service) return 'unknown'
  return service.status
}

function serviceById(services: ServiceStatus[], id: string) {
  return services.find((service) => service.id === id)
}

function formatDate(raw?: string) {
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleString()
}

function parseList(value: string) {
  return value.split(',').map((item) => item.trim()).filter(Boolean)
}

function grantLabel(grant: ContextGrant) {
  return grant.name || grant.id
}

function stringList(value: unknown) {
  if (!Array.isArray(value)) return ''
  return value.filter((item): item is string => typeof item === 'string').join(', ')
}

function grantDomainsLabel(grant: ContextGrant) {
  return stringList(grant.constraints?.domains)
}

function TTSConsole({ service }: { service?: ServiceStatus }) {
  const [health, setHealth] = useState<TTSHealth | null>(null)
  const [messages, setMessages] = useState<TTSMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [text, setText] = useState('')
  const [source, setSource] = useState('Codex')
  const [backend, setBackend] = useState('edge')
  const [voice, setVoice] = useState('en-US-ChristopherNeural')
  const [enqueueStatus, setEnqueueStatus] = useState('')
  const [liveMode, setLiveMode] = useState('polling')
  const [feedFailed, setFeedFailed] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [nextHealth, nextMessages] = await Promise.all([
        getTTSHealth(),
        getTTSMessages(),
      ])
      setHealth(nextHealth)
      setMessages(nextMessages.messages || [])
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    const eventSourceCtor = window.EventSource
    if (eventSourceCtor && !feedFailed) {
      const feed = new eventSourceCtor('/api/services/tts/feed')
      const onUpdate = () => {
        refresh()
      }
      feed.addEventListener('new', onUpdate)
      feed.addEventListener('update', onUpdate)
      feed.onerror = () => {
        setFeedFailed(true)
        setLiveMode('polling')
        feed.close()
      }
      setLiveMode('sse')
      return () => feed.close()
    }

    setLiveMode('polling')
    const interval = window.setInterval(refresh, 10000)
    return () => window.clearInterval(interval)
  }, [refresh, feedFailed])

  const handleEnqueue = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!text.trim()) return

    setEnqueueStatus('')
    setError('')
    try {
      const result = await enqueueTTS({
        text: text.trim(),
        source: source.trim() || 'Codex',
        backend,
        voice: voice.trim(),
      })
      setEnqueueStatus(`${result.id} ${result.status}`)
      setText('')
      await refresh()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const handleBackendChange = (nextBackend: string) => {
    setBackend(nextBackend)
    if (nextBackend === 'edge') setVoice('en-US-ChristopherNeural')
    if (nextBackend === 'kokoro') setVoice('am_onyx')
    if (nextBackend === 'orpheus') setVoice('troy')
  }

  return (
    <section className="services-panel" aria-label="TTS Gateway">
      <div className="services-panel-header">
        <div>
          <h2>TTS Gateway</h2>
          <p>status: {health?.status || (health?.ok ? 'ok' : statusLabel(service))}</p>
        </div>
        <div className="services-panel-actions">
          <span className={`services-live-badge ${liveMode === 'sse' ? 'is-live' : ''}`}>
            {liveMode === 'sse' ? 'LIVE' : 'POLL'}
          </span>
          <button type="button" className="services-button secondary" onClick={refresh} disabled={loading}>
            Refresh
          </button>
        </div>
      </div>

      {error && <div className="services-error" role="alert">{error}</div>}

      <form className="tts-enqueue-form" onSubmit={handleEnqueue}>
        <label className="services-field services-field-wide">
          <span>TTS text</span>
          <textarea
            aria-label="TTS text"
            value={text}
            onChange={(event) => setText(event.target.value)}
            rows={4}
          />
        </label>
        <label className="services-field">
          <span>TTS source</span>
          <input
            aria-label="TTS source"
            value={source}
            onChange={(event) => setSource(event.target.value)}
          />
        </label>
        <label className="services-field">
          <span>TTS backend</span>
          <select
            aria-label="TTS backend"
            value={backend}
            onChange={(event) => handleBackendChange(event.target.value)}
          >
            <option value="kokoro">kokoro</option>
            <option value="edge">edge</option>
            <option value="orpheus">orpheus</option>
          </select>
        </label>
        <label className="services-field">
          <span>TTS voice</span>
          <input
            aria-label="TTS voice"
            value={voice}
            onChange={(event) => setVoice(event.target.value)}
          />
        </label>
        <div className="services-form-actions">
          <button type="submit" className="services-button" disabled={!text.trim()}>
            Enqueue
          </button>
          {enqueueStatus && <span className="services-inline-status">{enqueueStatus}</span>}
        </div>
      </form>

      <div className="services-subheader">
        <h3>Queue</h3>
        <span>{messages.length} messages</span>
      </div>
      <div className="tts-message-list" aria-live="polite">
        {loading && messages.length === 0 ? (
          <div className="services-empty">Loading messages...</div>
        ) : messages.length === 0 ? (
          <div className="services-empty">No TTS messages yet.</div>
        ) : (
          messages.map((message) => (
            <article key={message.id} className={`tts-message tts-message-${message.status}`}>
              <div className="tts-message-main">
                <div className="tts-message-text">{message.text}</div>
                <div className="tts-message-meta">
                  <span>{message.status}</span>
                  {message.source && <span>{message.source}</span>}
                  {message.backend && <span>{message.backend}</span>}
                  {message.voice && <span>{message.voice}</span>}
                  {message.createdAt && <span>{formatDate(message.createdAt)}</span>}
                </div>
                {message.error && <div className="services-error compact">{message.error}</div>}
              </div>
              {message.status === 'ready' && (
                <audio
                  aria-label={`Play ${message.text}`}
                  controls
                  preload="none"
                  src={`/api/services/tts/audio/${encodeURIComponent(message.id)}`}
                />
              )}
            </article>
          ))
        )}
      </div>
    </section>
  )
}

function ContextIntegrationsConsole({ service }: { service?: ServiceStatus }) {
  const contextAvailable = service?.tokenConfigured !== false && service?.status !== 'degraded'
  const [grants, setGrants] = useState<ContextGrant[]>([])
  const [ingestionItems, setIngestionItems] = useState<ContextIngestionItem[]>([])
  const [auditEvents, setAuditEvents] = useState<ContextAuditEvent[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [oneTimeToken, setOneTimeToken] = useState('')
  const [grantName, setGrantName] = useState('ChatGPT low-sensitivity context')
  const [grantDomains, setGrantDomains] = useState('personal, world')
  const [maxSensitivity, setMaxSensitivity] = useState('internal')
  const [previewGrantId, setPreviewGrantId] = useState('')
  const [previewQuestion, setPreviewQuestion] = useState('')
  const [previewProvider, setPreviewProvider] = useState('openai')
  const [previewModel, setPreviewModel] = useState('gpt-5.2')
  const [preview, setPreview] = useState<ContextGrantPreviewResponse['preview'] | null>(null)
  const [busyGrantId, setBusyGrantId] = useState('')
  const [busyIngestionPath, setBusyIngestionPath] = useState('')

  const loadIntegrations = useCallback(async () => {
    if (!contextAvailable) return
    setLoading(true)
    setError('')
    try {
      const [nextGrants, nextIngestion, nextAudit] = await Promise.all([
        getContextGrants(),
        getContextIngestionQueue(),
        getContextAudit(25),
      ])
      const grantList = nextGrants.grants || []
      setGrants(grantList)
      setIngestionItems(nextIngestion.items || [])
      setAuditEvents(nextAudit.events || [])
      setPreviewGrantId((current) => current || grantList[0]?.id || '')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [contextAvailable])

  useEffect(() => {
    loadIntegrations()
  }, [loadIntegrations])

  const handleCreateGrant = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!contextAvailable) return
    setError('')
    setOneTimeToken('')
    try {
      const result = await createContextGrant({
        name: grantName.trim() || 'ChatGPT low-sensitivity context',
        purpose: 'External tool access managed from CHROTE',
        client_type: 'chatgpt_action',
        scopes: ['retrieve', 'docs:excerpt', 'ask:preview', 'egress:llm'],
        constraints: {
          domains: parseList(grantDomains),
          max_sensitivity: maxSensitivity,
          lifecycles: ['active'],
          provenance: ['declared', 'observed', 'mixed'],
          kinds: ['identity', 'preference', 'knowledge', 'research'],
        },
        egress: {
          allowed_providers: parseList(previewProvider),
          allowed_models: parseList(previewModel),
        },
        limits: {
          max_chunks: 8,
          max_total_chars: 6000,
        },
      })
      setGrants((current) => [result.grant, ...current.filter((grant) => grant.id !== result.grant.id)])
      setPreviewGrantId(result.grant.id)
      setOneTimeToken(result.token || '')
      await loadIntegrations()
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const handleRevokeGrant = async (grant: ContextGrant) => {
    setBusyGrantId(grant.id)
    setError('')
    try {
      const result = await revokeContextGrant(grant.id)
      setGrants((current) => current.map((item) => (item.id === grant.id ? result.grant : item)))
      await loadIntegrations()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusyGrantId('')
    }
  }

  const handleRotateGrant = async (grant: ContextGrant) => {
    setBusyGrantId(grant.id)
    setError('')
    setOneTimeToken('')
    try {
      const result = await rotateContextGrant(grant.id)
      setGrants((current) => current.map((item) => (item.id === grant.id ? result.grant : item)))
      setOneTimeToken(result.token || '')
      await loadIntegrations()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusyGrantId('')
    }
  }

  const handlePreview = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!contextAvailable || !previewGrantId || !previewQuestion.trim()) return
    setError('')
    setPreview(null)
    try {
      const result = await previewContextGrant({
        grant_id: previewGrantId,
        question: previewQuestion.trim(),
        egress: {
          provider: previewProvider.trim(),
          model: previewModel.trim(),
          mode: 'cloud_llm',
        },
        limits: {
          max_chunks: 4,
          max_chunk_chars: 900,
          max_total_chars: 3000,
        },
      })
      setPreview(result.preview)
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const handleApproveCandidate = async (item: ContextIngestionItem) => {
    setBusyIngestionPath(item.path)
    setError('')
    try {
      await approveContextIngestionCandidate(item.path)
      await loadIntegrations()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusyIngestionPath('')
    }
  }

  const handleRejectCandidate = async (item: ContextIngestionItem) => {
    setBusyIngestionPath(item.path)
    setError('')
    try {
      await rejectContextIngestionCandidate(item.path, 'Rejected in CHROTE ingestion review.')
      await loadIntegrations()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusyIngestionPath('')
    }
  }

  return (
    <section className="services-panel context-integrations-panel" aria-label="Context Integrations">
      <div className="services-panel-header">
        <div>
          <h2>Context Integrations</h2>
          <p>status: {contextAvailable ? 'operator-ready' : statusLabel(service)}</p>
        </div>
        <button type="button" className="services-button secondary" onClick={loadIntegrations} disabled={!contextAvailable || loading}>
          Refresh
        </button>
      </div>

      {error && <div className="services-error" role="alert">{error}</div>}
      {oneTimeToken && (
        <div className="context-token-banner">
          <span>New grant token</span>
          <code aria-label="One-time grant token">{oneTimeToken}</code>
          <button type="button" className="services-button secondary" onClick={() => setOneTimeToken('')}>
            Dismiss token
          </button>
        </div>
      )}

      <div className="context-integrations-body">
        <form className="context-integration-form" onSubmit={handleCreateGrant}>
          <div className="services-subheader">
            <h3>Grants</h3>
            <span>{grants.length} configured</span>
          </div>
          <label className="services-field">
            <span>Grant name</span>
            <input
              aria-label="Grant name"
              value={grantName}
              onChange={(event) => setGrantName(event.target.value)}
            />
          </label>
          <label className="services-field">
            <span>Grant domains</span>
            <input
              aria-label="Grant domains"
              value={grantDomains}
              onChange={(event) => setGrantDomains(event.target.value)}
            />
          </label>
          <label className="services-field">
            <span>Max sensitivity</span>
            <select
              aria-label="Max sensitivity"
              value={maxSensitivity}
              onChange={(event) => setMaxSensitivity(event.target.value)}
            >
              <option value="public">public</option>
              <option value="internal">internal</option>
              <option value="private">private</option>
              <option value="sensitive">sensitive</option>
              <option value="restricted">restricted</option>
            </select>
          </label>
          <div className="services-form-actions">
            <button type="submit" className="services-button" disabled={!contextAvailable || !grantName.trim()}>
              Create Grant
            </button>
          </div>
        </form>

        <div className="context-grant-list">
          {loading && grants.length === 0 ? (
            <div className="services-empty">Loading grants...</div>
          ) : grants.length === 0 ? (
            <div className="services-empty">No external grants yet.</div>
          ) : (
            grants.map((grant) => (
              <article key={grant.id} className="context-grant-row">
                <div>
                  <strong>{grantLabel(grant)}</strong>
                  <div className="context-grant-meta">
                    <span>{grant.status || 'unknown'}</span>
                    {grant.client_type && <span>{grant.client_type}</span>}
                    {grant.token_fingerprint && <span>{grant.token_fingerprint}</span>}
                    {grantDomainsLabel(grant) && <span>{grantDomainsLabel(grant)}</span>}
                  </div>
                </div>
                <div className="services-panel-actions">
                  <button
                    type="button"
                    className="services-button secondary"
                    aria-label={`Revoke grant ${grantLabel(grant)}`}
                    disabled={!contextAvailable || busyGrantId === grant.id || grant.status === 'revoked'}
                    onClick={() => handleRevokeGrant(grant)}
                  >
                    Revoke
                  </button>
                  <button
                    type="button"
                    className="services-button secondary"
                    aria-label={`Rotate grant ${grantLabel(grant)}`}
                    disabled={!contextAvailable || busyGrantId === grant.id}
                    onClick={() => handleRotateGrant(grant)}
                  >
                    Rotate
                  </button>
                </div>
              </article>
            ))
          )}
        </div>

        <form className="context-preview-form" onSubmit={handlePreview}>
          <div className="services-subheader">
            <h3>Policy Preview</h3>
            <span>{preview?.chunks?.length || 0} chunks</span>
          </div>
          <label className="services-field">
            <span>Preview grant</span>
            <select
              aria-label="Preview grant"
              value={previewGrantId}
              onChange={(event) => setPreviewGrantId(event.target.value)}
            >
              <option value="">Select grant</option>
              {grants.map((grant) => (
                <option key={grant.id} value={grant.id}>{grantLabel(grant)}</option>
              ))}
            </select>
          </label>
          <label className="services-field services-field-wide">
            <span>Preview question</span>
            <textarea
              aria-label="Preview question"
              value={previewQuestion}
              onChange={(event) => setPreviewQuestion(event.target.value)}
              rows={3}
            />
          </label>
          <label className="services-field">
            <span>Preview provider</span>
            <input
              aria-label="Preview provider"
              value={previewProvider}
              onChange={(event) => setPreviewProvider(event.target.value)}
            />
          </label>
          <label className="services-field">
            <span>Preview model</span>
            <input
              aria-label="Preview model"
              value={previewModel}
              onChange={(event) => setPreviewModel(event.target.value)}
            />
          </label>
          <div className="services-form-actions">
            <button type="submit" className="services-button" disabled={!contextAvailable || !previewGrantId || !previewQuestion.trim()}>
              Preview Grant
            </button>
          </div>
        </form>

        {preview && (
          <div className="context-preview-result">
            <div className="context-egress-state">
              egress: {preview.egress_plan?.allowed ? 'allowed' : preview.egress_plan?.reason || 'denied'}
            </div>
            <div className="context-preview-columns">
              <div>
                <div className="services-subheader">
                  <h3>Selected</h3>
                  <span>{preview.chunks?.length || 0}</span>
                </div>
                {(preview.chunks || []).map((chunk) => (
                  <div key={`${chunk.document_id || ''}:${chunk.canonical_path || chunk.path}`} className="context-mini-row">
                    <strong>{chunk.canonical_path || chunk.path}</strong>
                    {chunk.snippet && <small>{chunk.snippet}</small>}
                  </div>
                ))}
              </div>
              <div>
                <div className="services-subheader">
                  <h3>Denied</h3>
                  <span>{preview.denied?.length || 0}</span>
                </div>
                {(preview.denied || []).map((item) => (
                  <div key={`${item.canonical_path || item.path}:${item.reason}`} className="context-mini-row">
                    <strong>{item.canonical_path || item.path}</strong>
                    {item.reason && <small>{item.reason}</small>}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        <div className="context-ops-columns">
          <div>
            <div className="services-subheader">
              <h3>Ingestion Queue</h3>
              <span>{ingestionItems.length}</span>
            </div>
            {ingestionItems.length === 0 ? (
              <div className="services-empty">No raw or candidate items.</div>
            ) : (
              ingestionItems.map((item) => (
                <div key={item.path} className="context-mini-row context-ingestion-row">
                  <div>
                    <strong>{item.path}</strong>
                    <small>{[item.lifecycle, item.review_status, item.prompt_injection_risk].filter(Boolean).join(' / ')}</small>
                  </div>
                  {item.lifecycle === 'candidate' && (
                    <div className="services-panel-actions">
                      <button
                        type="button"
                        className="services-button secondary"
                        aria-label={`Approve ${item.path}`}
                        disabled={!contextAvailable || busyIngestionPath === item.path || item.review_status === 'approved'}
                        onClick={() => handleApproveCandidate(item)}
                      >
                        Approve
                      </button>
                      <button
                        type="button"
                        className="services-button secondary"
                        aria-label={`Reject ${item.path}`}
                        disabled={!contextAvailable || busyIngestionPath === item.path || item.review_status === 'rejected'}
                        onClick={() => handleRejectCandidate(item)}
                      >
                        Reject
                      </button>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
          <div>
            <div className="services-subheader">
              <h3>Audit</h3>
              <span>{auditEvents.length}</span>
            </div>
            {auditEvents.length === 0 ? (
              <div className="services-empty">No audit events yet.</div>
            ) : (
              auditEvents.map((event) => (
                <div key={event.id || `${event.type}:${event.timestamp}`} className="context-mini-row">
                  <strong>{event.type}</strong>
                  <small>{[event.actor, event.operation, event.grant_id, formatDate(event.timestamp)].filter(Boolean).join(' / ')}</small>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </section>
  )
}

function ContextConsole({ service }: { service?: ServiceStatus }) {
  const contextAvailable = service?.tokenConfigured !== false && service?.status !== 'degraded'
  const [docs, setDocs] = useState<ContextDocSummary[]>([])
  const [selectedPath, setSelectedPath] = useState('')
  const [doc, setDoc] = useState<ContextDoc | null>(null)
  const [content, setContent] = useState('')
  const [history, setHistory] = useState<ContextHistoryEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saveStatus, setSaveStatus] = useState('')
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState<ContextAskResponse | null>(null)
  const [asking, setAsking] = useState(false)
  const selectedDocReady = Boolean(selectedPath && doc?.path === selectedPath)

  const loadDocs = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const result = await getContextDocs()
      setDocs(result.docs || [])
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadDocs()
  }, [loadDocs])

  const openDoc = async (path: string) => {
    setSelectedPath(path)
    setDoc(null)
    setContent('')
    setHistory([])
    setSaveStatus('')
    setAnswer(null)
    setError('')
    try {
      const [nextDoc, nextHistory] = await Promise.all([
        readContextDoc(path),
        getContextHistory(path),
      ])
      setDoc(nextDoc)
      setContent(nextDoc.content)
      setHistory(nextHistory.history || [])
    } catch (err) {
      setError(errorMessage(err))
    }
  }

  const handleSave = async () => {
    if (!selectedPath || !contextAvailable || doc?.path !== selectedPath) return
    setSaving(true)
    setSaveStatus('')
    setError('')
    try {
      const result = await saveContextDoc(selectedPath, content)
      setSaveStatus(`Saved ${result.path}`)
      const [nextDoc, nextHistory] = await Promise.all([
        readContextDoc(selectedPath),
        getContextHistory(selectedPath),
      ])
      setDoc(nextDoc)
      setContent(nextDoc.content)
      setHistory(nextHistory.history || [])
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const handleAsk = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!question.trim() || !contextAvailable) return
    setAsking(true)
    setAnswer(null)
    setError('')
    try {
      setAnswer(await askContext(question.trim()))
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setAsking(false)
    }
  }

  return (
    <section className="services-panel" aria-label="Context API">
      <div className="services-panel-header">
        <div>
          <h2>Context API</h2>
          <p>status: {statusLabel(service)}</p>
        </div>
        <button type="button" className="services-button secondary" onClick={loadDocs} disabled={loading}>
          Refresh
        </button>
      </div>

      {service?.message && <div className="services-warning">degraded: {service.message}</div>}
      {error && <div className="services-error" role="alert">{error}</div>}

      <div className="context-layout">
        <div className="context-docs-list">
          <div className="services-subheader">
            <h3>Documents</h3>
            <span>{docs.length} readable</span>
          </div>
          {loading && docs.length === 0 ? (
            <div className="services-empty">Loading documents...</div>
          ) : docs.length === 0 ? (
            <div className="services-empty">No readable Markdown documents.</div>
          ) : (
            docs.map((summary) => (
              <button
                type="button"
                key={summary.path}
                aria-label={summary.path}
                className={`context-doc-button ${selectedPath === summary.path ? 'active' : ''}`}
                onClick={() => openDoc(summary.path)}
              >
                <span>{summary.path}</span>
                {summary.title && <small>{summary.title}</small>}
              </button>
            ))
          )}
        </div>

        <div className="context-editor">
          <label className="services-field services-field-wide">
            <span>Context document content</span>
            <textarea
              aria-label="Context document content"
              value={content}
              onChange={(event) => setContent(event.target.value)}
              rows={12}
              disabled={!selectedDocReady}
            />
          </label>
          <div className="services-form-actions">
            <button
              type="button"
              className="services-button"
              onClick={handleSave}
              disabled={!selectedDocReady || !contextAvailable || saving}
            >
              Save Document
            </button>
            {saveStatus && <span className="services-inline-status">{saveStatus}</span>}
          </div>

          <div className="context-history">
            <div className="services-subheader">
              <h3>History</h3>
              <span>{history.length} commits</span>
            </div>
            {history.length === 0 ? (
              <div className="services-empty">No history for the selected document.</div>
            ) : (
              history.map((entry) => (
                <div key={entry.hash} className="context-history-entry">
                  <code>{entry.hash.slice(0, 7)}</code>
                  <span>{entry.subject || entry.message || 'context update'}</span>
                  {entry.date && <time>{formatDate(entry.date)}</time>}
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      <form className="context-ask" onSubmit={handleAsk}>
        <label className="services-field services-field-wide">
          <span>Ask Context</span>
          <textarea
            aria-label="Ask Context"
            value={question}
            onChange={(event) => setQuestion(event.target.value)}
            rows={3}
          />
        </label>
        <button
          type="submit"
          className="services-button"
          disabled={!question.trim() || !contextAvailable || asking}
        >
          Ask
        </button>
      </form>

      {answer && (
        <div className="context-answer">
          <div className="context-answer-text">{answer.answer}</div>
          <div className="context-sources">
            {answer.sources.map((source) => (
              <button
                type="button"
                key={`${source.path}:${source.line || ''}`}
                className="context-source"
                onClick={() => openDoc(source.path)}
              >
                <span>{source.path}</span>
                {source.snippet && <small>{source.snippet}</small>}
              </button>
            ))}
          </div>
        </div>
      )}
    </section>
  )
}

export default function ServicesView() {
  const [services, setServices] = useState<ServiceStatus[]>([])
  const [catalogError, setCatalogError] = useState('')
  const ttsService = useMemo(() => serviceById(services, 'tts'), [services])
  const contextService = useMemo(() => serviceById(services, 'context'), [services])

  const loadCatalog = useCallback(async () => {
    setCatalogError('')
    try {
      const catalog = await listServices()
      setServices(catalog.services || [])
    } catch (err) {
      setCatalogError(errorMessage(err))
    }
  }, [])

  useEffect(() => {
    loadCatalog()
  }, [loadCatalog])

  return (
    <div className="services-view">
      <div className="services-status-strip">
        {catalogError && <div className="services-error" role="alert">{catalogError}</div>}
        {services.map((service) => (
          <div key={service.id} className={`services-status-pill services-status-${service.status}`}>
            <span>{service.name}</span>
            <strong>{service.status}</strong>
          </div>
        ))}
      </div>

      <div className="services-grid">
        <TTSConsole service={ttsService} />
        <ContextConsole service={contextService} />
        <ContextIntegrationsConsole service={contextService} />
      </div>
    </div>
  )
}
