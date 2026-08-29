import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  askContext,
  enqueueTTS,
  getContextDocs,
  getContextHistory,
  getTTSHealth,
  getTTSMessages,
  listServices,
  readContextDoc,
  saveContextDoc,
  type ContextAskResponse,
  type ContextDoc,
  type ContextDocSummary,
  type ContextHistoryEntry,
  type ServiceStatus,
  type TTSHealth,
  type TTSMessage,
} from '../../services/servicesClient'
import ContextIntegrationsConsole from './ContextIntegrationsConsole'
import { errorMessage, formatDate, statusLabel } from './helpers'

function serviceById(services: ServiceStatus[], id: string) {
  return services.find((service) => service.id === id)
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
    <section className="services-panel" aria-label="Context Citadel">
      <div className="services-panel-header">
        <div>
          <h2>Context Citadel</h2>
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
import './ServicesView.css'
