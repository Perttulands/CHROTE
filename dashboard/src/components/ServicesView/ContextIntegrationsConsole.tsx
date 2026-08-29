import { useCallback, useEffect, useState } from 'react'
import {
  approveContextIngestionCandidate,
  createContextGrant,
  getContextAudit,
  getContextGrants,
  getContextIngestionQueue,
  previewContextGrant,
  rejectContextIngestionCandidate,
  revokeContextGrant,
  rotateContextGrant,
  type ContextAuditEvent,
  type ContextGrant,
  type ContextGrantPreviewResponse,
  type ContextIngestionItem,
  type ServiceStatus,
} from '../../services/servicesClient'
import { errorMessage, formatDate, statusLabel } from './helpers'

function parseList(value: string) {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

function grantLabel(grant: ContextGrant) {
  return grant.name || grant.id
}

function grantDomainsLabel(grant: ContextGrant) {
  const domains = grant.constraints?.domains
  return Array.isArray(domains) ? domains.filter((item): item is string => typeof item === 'string').join(', ') : ''
}

export default function ContextIntegrationsConsole({ service }: { service?: ServiceStatus }) {
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
