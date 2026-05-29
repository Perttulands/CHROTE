import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { FileText, RefreshCw, Send } from 'lucide-react'
import {
  GasCityApiError,
  getGasCityMail,
  getGasCityObserver,
  getGasCityReviewQuorumCapability,
  getGasCityTranscript,
  launchGasCityReviewQuorum,
  sendGasCityPiPoem,
  type GasCityEvent,
  type GasCityFormula,
  type GasCityMailList,
  type GasCityMailMessage,
  type GasCityObserver,
  type GasCityPiPoemResponse,
  type GasCityReviewQuorumCapability,
  type GasCityReviewQuorumResponse,
  type GasCitySession,
  type GasCityTranscript,
  type GasCityWorkItem,
} from '../../services/gascityClient'

function errorMessage(error: unknown): string {
  if (error instanceof GasCityApiError) return `${error.code}: ${error.message}`
  if (error instanceof Error) return error.message
  return 'Gas City observer request failed'
}

function formatDate(raw?: string) {
  if (!raw) return '--'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  return date.toLocaleString()
}

function formatUptime(seconds?: number) {
  if (!seconds || seconds < 0) return '--'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}

function statusTone(status: string) {
  if (status === 'ok') return 'ok'
  if (status === 'unavailable' || status === 'misconfigured') return 'error'
  return 'warn'
}

function Metric({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <section className="gascity-metric">
      <span>{label}</span>
      <strong>{value}</strong>
      {detail && <small>{detail}</small>}
    </section>
  )
}

function SessionsPanel({
  sessions,
  transcript,
  transcriptLoading,
  transcriptError,
  onLoadTranscript,
}: {
  sessions: GasCitySession[]
  transcript: GasCityTranscript | null
  transcriptLoading: boolean
  transcriptError: string
  onLoadTranscript: (session: GasCitySession) => void
}) {
  return (
    <section className="gascity-panel" aria-label="Gas City sessions">
      <div className="gascity-panel-header">
        <h2>Sessions</h2>
        <span>{sessions.length} active</span>
      </div>
      <div className="gascity-list">
        {sessions.length === 0 ? (
          <div className="gascity-empty">No active Gas City sessions.</div>
        ) : (
          sessions.map(session => (
            <article key={`${session.city}:${session.id}`} className="gascity-row">
              <div>
                <strong>{session.title || session.alias || session.id}</strong>
                <small>{[session.city, session.template, session.provider].filter(Boolean).join(' / ')}</small>
              </div>
              <div className="gascity-row-meta">
                <span>{session.state || 'unknown'}</span>
                <span>{session.running ? 'running' : 'stopped'}</span>
                <span>{formatDate(session.lastActive)}</span>
                <button
                  type="button"
                  className="gascity-icon-button gascity-row-action"
                  aria-label={`Recover transcript for ${session.title || session.alias || session.id}`}
                  title={`Recover transcript for ${session.title || session.alias || session.id}`}
                  onClick={() => onLoadTranscript(session)}
                  disabled={transcriptLoading}
                >
                  <FileText size={16} aria-hidden="true" />
                </button>
              </div>
            </article>
          ))
        )}
      </div>
      {transcriptError && <div className="gascity-inline-error" role="alert">{transcriptError}</div>}
      {transcriptLoading && <div className="gascity-transcript-status">Recovering transcript...</div>}
      {transcript && (
        <div className="gascity-transcript">
          <div className="gascity-transcript-title">
            <strong>Transcript: {transcript.alias || transcript.sessionId}</strong>
            <small>
              {[
                transcript.source,
                transcript.sessionId,
                transcript.state,
                `${transcript.lineCount} lines`,
                transcript.truncated ? 'truncated' : '',
                transcript.capturedAt ? `captured ${transcript.capturedAt}` : '',
              ].filter(Boolean).join(' / ')}
            </small>
          </div>
          {transcript.stale && (
            <div className="gascity-transcript-stale" role="status">
              Recovered from CHROTE archive (live peek unavailable). This is the
              last captured snapshot, not live output.
            </div>
          )}
          <pre>{transcript.transcript || '(empty transcript)'}</pre>
        </div>
      )}
    </section>
  )
}

function MailPanel({
  mail,
  topic,
  requesting,
  requestResult,
  requestError,
  onTopicChange,
  onSend,
}: {
  mail: GasCityMailList | null
  topic: string
  requesting: boolean
  requestResult: GasCityPiPoemResponse | null
  requestError: string
  onTopicChange: (topic: string) => void
  onSend: (event: FormEvent<HTMLFormElement>) => void
}) {
  const messages = mail?.messages || []
  return (
    <section className="gascity-panel" aria-label="Gas City human mail">
      <div className="gascity-panel-header">
        <h2>Mail</h2>
        <span>{messages.length} human</span>
      </div>
      <form className="gascity-poem-form" onSubmit={onSend}>
        <label className="gascity-visually-hidden" htmlFor="gascity-pi-topic">Pi poem topic</label>
        <input
          id="gascity-pi-topic"
          aria-label="Pi poem topic"
          value={topic}
          onChange={event => onTopicChange(event.target.value)}
          maxLength={80}
          placeholder="topic"
          disabled={requesting}
        />
        <button
          type="submit"
          className="gascity-icon-button"
          aria-label="Send Pi poem smoke"
          title="Send Pi poem smoke"
          disabled={requesting || topic.trim().length === 0}
        >
          <Send size={16} aria-hidden="true" />
        </button>
      </form>
      {requestError && <div className="gascity-inline-error" role="alert">{requestError}</div>}
      {requestResult && (
        <div className="gascity-request-result">
          <strong>{requestResult.nonce}</strong>
          <small>{requestResult.subject}</small>
          <small>{[requestResult.targetAlias || requestResult.target, requestResult.targetSessionId, requestResult.targetTemplate, requestResult.recipient].filter(Boolean).join(' / ')}</small>
          {requestResult.output && <span>{requestResult.output}</span>}
        </div>
      )}
      <div className="gascity-list">
        {messages.length === 0 ? (
          <div className="gascity-empty">No human mail reported.</div>
        ) : (
          messages.map(message => <MailRow key={message.id} message={message} />)
        )}
      </div>
    </section>
  )
}

function MailRow({ message }: { message: GasCityMailMessage }) {
  return (
    <article className="gascity-mail-row">
      <div className="gascity-mail-title">
        <strong>{message.subject || message.id}</strong>
        <small>
          {[message.id, message.from, message.read ? 'read' : 'unread', formatDate(message.createdAt)]
            .filter(Boolean)
            .join(' / ')}
        </small>
      </div>
      <p>{message.body}{message.bodyTruncated ? '...' : ''}</p>
    </article>
  )
}

interface ReviewQuorumFormState {
  subject: string
  title: string
  baseRef: string
  scopeKind: string
  scopeRef: string
}

const defaultReviewQuorumForm: ReviewQuorumFormState = {
  subject: '',
  title: '',
  baseRef: 'origin/main',
  scopeKind: 'city',
  scopeRef: 'gascity',
}

let reviewQuorumLaneSerial = 0

function createReviewQuorumLaneId(prefix: string) {
  reviewQuorumLaneSerial += 1
  return `${prefix}-${Date.now().toString(36)}-${reviewQuorumLaneSerial}`
}

function WorkflowPanel({
  formulas,
  molecules,
  wisps,
  convoys,
  reviewQuorumCapability,
  onLaunched,
}: {
  formulas: GasCityFormula[]
  molecules: GasCityWorkItem[]
  wisps: GasCityWorkItem[]
  convoys: GasCityWorkItem[]
  reviewQuorumCapability: GasCityReviewQuorumCapability | null
  onLaunched: () => Promise<void>
}) {
  const [reviewForm, setReviewForm] = useState(defaultReviewQuorumForm)
  const [launching, setLaunching] = useState(false)
  const [launchError, setLaunchError] = useState('')
  const [launchResult, setLaunchResult] = useState<GasCityReviewQuorumResponse | null>(null)

  const updateReviewForm = (field: keyof ReviewQuorumFormState, value: string) => {
    setReviewForm(current => ({ ...current, [field]: value }))
  }

  const launchReviewQuorum = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (reviewQuorumCapability?.available !== true) return
    const subject = reviewForm.subject.trim()
    const baseRef = reviewForm.baseRef.trim()
    if (!subject || !baseRef) return

    const title = reviewForm.title.trim()
    const scopeKind = reviewForm.scopeKind === 'rig' ? 'rig' : 'city'
    const scopeRef = reviewForm.scopeRef.trim()
    const [laneOneTarget = 'codex-review', laneTwoTarget = 'claude-review', synthesisTarget = 'codex-synth'] = reviewQuorumCapability.targets ?? []
    setLaunchError('')
    setLaunchResult(null)
    setLaunching(true)
    try {
      const result = await launchGasCityReviewQuorum({
        subject,
        title: title || undefined,
        baseRef,
        scopeKind,
        scopeRef: scopeRef || 'gascity',
        laneOne: {
          id: createReviewQuorumLaneId('codex-review'),
          provider: 'codex',
          model: 'codex-cli-default',
          target: laneOneTarget,
        },
        laneTwo: {
          id: createReviewQuorumLaneId('claude-review'),
          provider: 'claude',
          model: 'claude-cli-default',
          target: laneTwoTarget,
        },
        synthesisTarget,
      })
      setLaunchResult(result)
      setReviewForm(current => ({
        ...current,
        subject: '',
        title: '',
        baseRef: 'origin/main',
        scopeKind: 'city',
        scopeRef: 'gascity',
      }))
      await onLaunched()
    } catch (err) {
      setLaunchError(errorMessage(err))
    } finally {
      setLaunching(false)
    }
  }

  const canLaunch = reviewForm.subject.trim().length > 0
    && reviewForm.baseRef.trim().length > 0
  const [laneOneTarget = 'codex-review', laneTwoTarget = 'claude-review', synthesisTarget = 'codex-synth'] = reviewQuorumCapability?.targets ?? []

  return (
    <section className="gascity-panel" aria-label="Gas City workflows">
      <div className="gascity-panel-header">
        <h2>Workflows</h2>
        <span>{formulas.length} formulas</span>
      </div>
      {reviewQuorumCapability?.available === true && (
        <form className="gascity-workflow-launch-form" onSubmit={launchReviewQuorum}>
          <div className="gascity-launch-grid">
            <label className="gascity-field gascity-field-wide">
              <span>Subject</span>
              <input
                aria-label="Review subject"
                value={reviewForm.subject}
                onChange={event => updateReviewForm('subject', event.target.value)}
                maxLength={160}
                required
                disabled={launching}
              />
            </label>
            <label className="gascity-field">
              <span>Title</span>
              <input
                aria-label="Review title"
                value={reviewForm.title}
                onChange={event => updateReviewForm('title', event.target.value)}
                maxLength={120}
                disabled={launching}
              />
            </label>
            <label className="gascity-field">
              <span>Base</span>
              <input
                aria-label="Review base ref"
                value={reviewForm.baseRef}
                onChange={event => updateReviewForm('baseRef', event.target.value)}
                maxLength={120}
                required
                disabled={launching}
              />
            </label>
            <label className="gascity-field">
              <span>Scope</span>
              <select
                aria-label="Review scope kind"
                value={reviewForm.scopeKind}
                onChange={event => updateReviewForm('scopeKind', event.target.value)}
                disabled={launching}
              >
                <option value="city">city</option>
                <option value="rig">rig</option>
              </select>
            </label>
            <label className="gascity-field">
              <span>Ref</span>
              <input
                aria-label="Review scope ref"
                value={reviewForm.scopeRef}
                onChange={event => updateReviewForm('scopeRef', event.target.value)}
                maxLength={120}
                disabled={launching}
              />
            </label>
            <div className="gascity-workflow-targets" aria-label="Review quorum targets">
              <span>{laneOneTarget}</span>
              <span>{laneTwoTarget}</span>
              <span>{synthesisTarget}</span>
            </div>
            <button
              type="submit"
              className="gascity-action-button"
              aria-label="Launch review quorum"
              disabled={launching || !canLaunch}
            >
              <Send size={15} aria-hidden="true" />
              <span>{launching ? 'Launching' : 'Launch'}</span>
            </button>
          </div>
          {launchError && <div className="gascity-inline-error" role="alert">{launchError}</div>}
          {launchResult && (
            <div className="gascity-workflow-launch-result" role="status">
              <strong>{launchResult.workflowId || launchResult.beadId}</strong>
              <small>{[launchResult.title, launchResult.mode, `base ${launchResult.baseRef}`, `target ${launchResult.target}`].filter(Boolean).join(' / ')}</small>
              {launchResult.output && <span>{launchResult.output}</span>}
            </div>
          )}
        </form>
      )}
      <div className="gascity-workflow-columns">
        <div>
          <h3>Formulas</h3>
          {formulas.length === 0 ? (
            <div className="gascity-empty compact">No formulas reported.</div>
          ) : (
            formulas.slice(0, 8).map(formula => (
              <div key={`${formula.city}:${formula.name}`} className="gascity-mini-row">
                <strong>{formula.name}</strong>
                <small>{[`v${formula.version || '?'}`, `${formula.runCount} runs`, formula.city].join(' / ')}</small>
              </div>
            ))
          )}
        </div>
        <div>
          <h3>Molecules</h3>
          {molecules.length === 0 ? (
            <div className="gascity-empty compact">No molecules reported.</div>
          ) : (
            molecules.slice(0, 8).map(item => <WorkItemRow key={`${item.city}:${item.id}`} item={item} />)
          )}
        </div>
        <div>
          <h3>Wisps</h3>
          {wisps.length === 0 ? (
            <div className="gascity-empty compact">No wisps reported.</div>
          ) : (
            wisps.slice(0, 8).map(item => <WorkItemRow key={`${item.city}:${item.id}`} item={item} />)
          )}
        </div>
        <div>
          <h3>Convoys</h3>
          {convoys.length === 0 ? (
            <div className="gascity-empty compact">No convoys reported.</div>
          ) : (
            convoys.slice(0, 8).map(item => <WorkItemRow key={`${item.city}:${item.id}`} item={item} />)
          )}
        </div>
      </div>
    </section>
  )
}

function WorkItemRow({ item }: { item: GasCityWorkItem }) {
  return (
    <div className="gascity-mini-row">
      <strong>{item.title || item.id}</strong>
      <small>{[item.id, item.status, item.ref, item.routedTo ? `routed ${item.routedTo}` : ''].filter(Boolean).join(' / ')}</small>
    </div>
  )
}

function EventsPanel({ events }: { events: GasCityEvent[] }) {
  return (
    <section className="gascity-panel" aria-label="Gas City recent events">
      <div className="gascity-panel-header">
        <h2>Recent Events</h2>
        <span>{events.length}</span>
      </div>
      <div className="gascity-list">
        {events.length === 0 ? (
          <div className="gascity-empty">No recent events reported.</div>
        ) : (
          events.map(event => (
            <article key={`${event.city || 'supervisor'}:${event.seq}:${event.type}`} className="gascity-row">
              <div>
                <strong>{event.type}</strong>
                <small>{[event.city, event.actor, event.subject].filter(Boolean).join(' / ')}</small>
              </div>
              <div className="gascity-row-meta">
                <span>#{event.seq}</span>
                <span>{formatDate(event.time)}</span>
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  )
}

export default function GasCityView() {
  const [observer, setObserver] = useState<GasCityObserver | null>(null)
  const [mail, setMail] = useState<GasCityMailList | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [topic, setTopic] = useState('')
  const [requesting, setRequesting] = useState(false)
  const [requestError, setRequestError] = useState('')
  const [requestResult, setRequestResult] = useState<GasCityPiPoemResponse | null>(null)
  const [reviewQuorumCapability, setReviewQuorumCapability] = useState<GasCityReviewQuorumCapability | null>(null)
  const [transcript, setTranscript] = useState<GasCityTranscript | null>(null)
  const [transcriptLoading, setTranscriptLoading] = useState(false)
  const [transcriptError, setTranscriptError] = useState('')

  const refresh = useCallback(async () => {
    setError('')
    try {
      const [nextObserver, nextMail, nextReviewQuorumCapability] = await Promise.all([
        getGasCityObserver(),
        getGasCityMail('human', 20),
        getGasCityReviewQuorumCapability().catch(() => null),
      ])
      setObserver(nextObserver)
      setMail(nextMail)
      setReviewQuorumCapability(nextReviewQuorumCapability)
    } catch (err) {
      setError(errorMessage(err))
      setReviewQuorumCapability(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const tone = statusTone(observer?.status || (error ? 'unavailable' : 'loading'))
  const cityLabel = useMemo(() => {
    const running = observer?.health.citiesRunning ?? observer?.cities.filter(city => city.running).length ?? 0
    const total = observer?.health.citiesTotal ?? observer?.cities.length ?? 0
    return `${running} running / ${total} total`
  }, [observer])

  const sendPoemRequest = useCallback(async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const nextTopic = topic.trim()
    if (!nextTopic) return
    setRequestError('')
    setRequesting(true)
    try {
      const result = await sendGasCityPiPoem({ topic: nextTopic })
      setRequestResult(result)
      setTopic('')
      await refresh()
    } catch (err) {
      setRequestError(errorMessage(err))
    } finally {
      setRequesting(false)
    }
  }, [refresh, topic])

  const loadTranscript = useCallback(async (session: GasCitySession) => {
    setTranscriptError('')
    setTranscriptLoading(true)
    try {
      setTranscript(await getGasCityTranscript(session.id, 120))
    } catch (err) {
      setTranscriptError(errorMessage(err))
    } finally {
      setTranscriptLoading(false)
    }
  }, [])

  return (
    <div className="gascity-view">
      <header className="gascity-header">
        <div>
          <h1>Gas City</h1>
          <p>status: {observer?.status || (loading ? 'loading' : 'unavailable')}</p>
        </div>
        <button
          type="button"
          className="gascity-icon-button"
          aria-label="Refresh Gas City observer"
          onClick={refresh}
          disabled={loading}
          title="Refresh Gas City observer"
        >
          <RefreshCw size={16} aria-hidden="true" />
        </button>
      </header>

      {error && <div className="gascity-error" role="alert">{error}</div>}
      {observer?.error && <div className={`gascity-status-banner gascity-status-${tone}`}>{observer.error}</div>}
      {observer?.upstreamErrors?.length ? (
        <div className="gascity-upstream-errors">
          {observer.upstreamErrors.map(item => (
            <span key={`${item.route}:${item.message}`}>{item.route}: {item.message}</span>
          ))}
        </div>
      ) : null}

      <section className="gascity-metrics" aria-label="Gas City summary">
        <Metric label="City Health" value={observer?.health.status || '--'} detail={cityLabel} />
        <Metric label="Supervisor" value={observer?.health.version || '--'} detail={formatUptime(observer?.health.uptimeSeconds)} />
        <Metric label="Mail" value={`${observer?.mail.unread ?? 0} unread`} detail={`${observer?.mail.total ?? 0} total`} />
        <Metric label="Work" value={`${observer?.work.open ?? 0} open`} detail={`${observer?.work.ready ?? 0} ready / ${observer?.work.inProgress ?? 0} active`} />
        <Metric label="Routed" value={`${observer?.work.routed ?? 0}`} detail={`${observer?.work.convoys ?? 0} convoys`} />
        <Metric label="Workflows" value={`${observer?.work.molecules ?? 0} molecules`} detail={`${observer?.work.wisps ?? 0} wisps`} />
      </section>

      <main className="gascity-grid">
        <SessionsPanel
          sessions={observer?.sessions || []}
          transcript={transcript}
          transcriptLoading={transcriptLoading}
          transcriptError={transcriptError}
          onLoadTranscript={loadTranscript}
        />
        <MailPanel
          mail={mail}
          topic={topic}
          requesting={requesting}
          requestResult={requestResult}
          requestError={requestError}
          onTopicChange={setTopic}
          onSend={sendPoemRequest}
        />
        <WorkflowPanel
          formulas={observer?.formulas || []}
          molecules={observer?.molecules || []}
          wisps={observer?.wisps || []}
          convoys={observer?.convoys || []}
          reviewQuorumCapability={reviewQuorumCapability}
          onLaunched={refresh}
        />
        <EventsPanel events={observer?.recentEvents || []} />
      </main>
    </div>
  )
}
