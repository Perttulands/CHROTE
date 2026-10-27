/**
 * The Services tab: what runs on this host, and the switch for each.
 *
 * One row per service in the host's catalog order, so nothing moves while
 * states change: a light that says how it is, the state in a word, and the one
 * action that changes it, Start or Stop, with Restart, Open and Send in the
 * row's menu. An action reads as what it is doing until the controller has
 * answered and a fresh read agrees, and a failure stays on its row in the
 * controller's own words. The list draws at once from the last read; the
 * resident Service agent sits behind the spine with the chosen row as its
 * reference.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent } from 'react'
import MenuTarget from '../MenuTarget'
import type { MenuGroup } from '../Menu'
import ResidentColumn from '../ResidentColumn'
import { TableSlot } from '../TableHost'
import { useSession } from '../../context/SessionContext'
import { useStatus } from '../../context/StatusContext'
import { actOnService, readCachedServices, readServices, type Service, type ServiceState } from './servicesApi'
import './services.css'

type Change = 'start' | 'stop' | 'restart'

const READ_EVERY_MS = 10_000
/** While the controller reports a service on its way up or down. */
const READ_MOVING_MS = 2_000
/** A first read slower than this says so; a quicker one just arrives. */
const SLOW_READ_MS = 600

const STATE_WORDS: Record<ServiceState, string> = {
  running: 'Running',
  stopped: 'Stopped',
  starting: 'Starting…',
  stopping: 'Stopping…',
  partial: 'Partly running',
  failed: 'Failed',
  unknown: 'Unknown',
}

const CHANGE_WORDS: Record<Change, { doing: string; done: string; verb: string }> = {
  start: { doing: 'Starting…', done: 'started', verb: 'Start' },
  stop: { doing: 'Stopping…', done: 'stopped', verb: 'Stop' },
  restart: { doing: 'Restarting…', done: 'restarted', verb: 'Restart' },
}

/** The change the row's one button makes: towards running, unless it runs. */
function primaryChange(state: ServiceState): Change | null {
  if (state === 'running') return 'stop'
  if (state === 'starting' || state === 'stopping') return null
  return 'start'
}

function safeLink(value: string): string {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : ''
  } catch {
    return ''
  }
}

function message(cause: unknown, fallback: string): string {
  return cause instanceof Error && cause.message ? cause.message : fallback
}

function without<T>(record: Record<string, T>, key: string): Record<string, T> {
  if (!(key in record)) return record
  const next = { ...record }
  delete next[key]
  return next
}

function serviceReference(service: Service, state: ServiceState): string {
  return `service ${service.id} (${service.name}, ${STATE_WORDS[state].replace('…', '').toLowerCase()})`
}

/** The tally beside the title: the states present, in the order they matter. */
function tally(services: Service[]): { words: string; failed: string } {
  const count = (state: ServiceState) => services.filter(service => service.state === state).length
  const partial = count('partial')
  const failed = count('failed')
  return {
    words: [`${count('running')} of ${services.length} running`, partial ? `${partial} partly` : ''].filter(Boolean).join(' · '),
    failed: failed ? `${failed} failed` : '',
  }
}

export default function ServicesView({ active }: { active: boolean }) {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()
  const [services, setServices] = useState<Service[] | null>(readCachedServices)
  const [readError, setReadError] = useState<string | null>(null)
  const [changing, setChanging] = useState<Record<string, Change>>({})
  const [failures, setFailures] = useState<Record<string, string>>({})
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [slow, setSlow] = useState(false)
  const changingRef = useRef(changing)
  changingRef.current = changing
  // Rises whenever an action lands, so a read that left before it cannot
  // paint the state from before the change.
  const landed = useRef(0)
  const listRef = useRef<HTMLDivElement>(null)

  const read = useCallback(async (fresh = false) => {
    const started = landed.current
    try {
      const list = await readServices(fresh)
      if (started !== landed.current) return
      setServices(list)
      setReadError(null)
    } catch (cause) {
      if (started !== landed.current) return
      setReadError(message(cause, 'Could not read the services'))
    }
  }, [])

  const moving = services?.some(service => service.state === 'starting' || service.state === 'stopping') ?? false
  useEffect(() => {
    if (!active) return
    void read()
    const timer = window.setInterval(() => void read(), moving ? READ_MOVING_MS : READ_EVERY_MS)
    return () => window.clearInterval(timer)
  }, [active, moving, read])

  const waiting = services === null && readError === null
  useEffect(() => {
    if (!waiting) return
    const timer = window.setTimeout(() => setSlow(true), SLOW_READ_MS)
    return () => window.clearTimeout(timer)
  }, [waiting])

  const change = useCallback(async (service: Service, kind: Change) => {
    if (changingRef.current[service.id]) return
    changingRef.current = { ...changingRef.current, [service.id]: kind }
    setChanging(changingRef.current)
    setFailures(previous => without(previous, service.id))
    try {
      let reported: Service | null = null
      if (kind !== 'start') reported = await actOnService(service.id, 'stop')
      if (kind !== 'stop') reported = await actOnService(service.id, 'start')
      landed.current += 1
      if (reported) {
        const now = reported
        setServices(list => list?.map(entry => (entry.id === now.id ? now : entry)) ?? list)
      }
      announce(`${service.name} ${CHANGE_WORDS[kind].done}`, 'success')
    } catch (cause) {
      landed.current += 1
      const words = message(cause, `Could not ${kind} ${service.name}`)
      setFailures(previous => ({ ...previous, [service.id]: words }))
      announce(`${service.name}: ${words}`, 'error')
    }
    // The row keeps saying what it is doing until a read after the change agrees.
    await read(true)
    changingRef.current = without(changingRef.current, service.id)
    setChanging(changingRef.current)
  }, [announce, read])

  const shownState = useCallback((service: Service): ServiceState => {
    const kind = changing[service.id]
    if (kind === 'stop') return 'stopping'
    if (kind) return 'starting'
    return service.state
  }, [changing])

  const selected = services?.find(service => service.id === selectedId) ?? null
  const reference = selected ? serviceReference(selected, shownState(selected)) : null
  const summary = useMemo(() => (services && services.length > 0 ? tally(services) : null), [services])

  const menu = (service: Service) => (): MenuGroup[] => {
    const state = shownState(service)
    const busy = Boolean(changing[service.id])
    const link = safeLink(service.link)
    const readOnly = service.controllable ? undefined : 'The host reports this service but does not let CHROTE change it'
    return [
      {
        id: 'control',
        rows: [
          { id: 'start', label: 'Start', disabled: busy || !service.controllable || state === 'running', reason: readOnly, onSelect: () => void change(service, 'start') },
          { id: 'stop', label: 'Stop', disabled: busy || !service.controllable || state === 'stopped', reason: readOnly, onSelect: () => void change(service, 'stop') },
          { id: 'restart', label: 'Restart', disabled: busy || !service.controllable || state === 'stopped', reason: readOnly, onSelect: () => void change(service, 'restart') },
        ],
      },
      {
        id: 'elsewhere',
        rows: [
          ...(link ? [{ id: 'open', label: 'Open', onSelect: () => { window.open(link, '_blank', 'noopener,noreferrer') } }] : []),
          { id: 'send', label: 'Send', onSelect: () => openSendToSession({ reference: serviceReference(service, state) }) },
        ],
      },
    ]
  }

  const focusRow = (id: string) => {
    setSelectedId(id)
    requestAnimationFrame(() => listRef.current?.querySelector<HTMLElement>(`[data-service="${CSS.escape(id)}"]`)?.focus())
  }

  const onListKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!services || services.length === 0) return
    if ((event.target as HTMLElement).closest('button, a')) return
    const at = services.findIndex(service => service.id === selectedId)
    const step = (to: number) => {
      event.preventDefault()
      focusRow(services[Math.max(0, Math.min(services.length - 1, to))].id)
    }
    if (event.key === 'ArrowDown') step(at + 1)
    else if (event.key === 'ArrowUp') step(at < 0 ? 0 : at - 1)
    else if (event.key === 'Home') step(0)
    else if (event.key === 'End') step(services.length - 1)
    else if (event.key === 'Escape' && selectedId) { event.preventDefault(); setSelectedId(null) }
    else if (event.key === 'Enter' && selected && selected.controllable) {
      const kind = primaryChange(shownState(selected))
      if (kind && !changing[selected.id]) { event.preventDefault(); void change(selected, kind) }
    }
  }

  // The one row Tab lands on: the chosen one, or the first while none is.
  const focusableId = selected?.id ?? services?.[0]?.id ?? null

  return (
    <div className="services-view" data-ui="services.view">
      <main className="services-main">
        <div className="services-subject">
          <span>Services</span>
          {summary && (
            <span className="services-tally">
              {summary.words}
              {summary.failed && <> · <span className="services-tally-failed">{summary.failed}</span></>}
            </span>
          )}
        </div>
        <p className="services-purpose">What runs on this host, and the switch for each.</p>

        {readError && services !== null && (
          <p className="services-stale" role="status">Showing the last read. {readError}</p>
        )}

        {services === null && readError !== null && (
          <div className="services-empty" role="alert">
            <p className="services-empty-head">The service controller did not answer.</p>
            <p>{readError}</p>
            <p className="services-empty-hint">The host names its controller's socket in services.json beside the launch configuration. CHROTE asks again while this tab is open.</p>
          </div>
        )}

        {waiting && slow && <p className="services-note" aria-live="polite">Asking the host…</p>}

        {services !== null && services.length === 0 && (
          <div className="services-empty"><p className="services-empty-head">The host's catalog lists no services.</p></div>
        )}

        {services !== null && services.length > 0 && (
          <div className="services-list" role="list" aria-label="Services" ref={listRef} onKeyDown={onListKey}>
            {services.map((service, index) => {
              const state = shownState(service)
              const kind = changing[service.id]
              const primary = service.controllable ? primaryChange(state) : null
              const link = safeLink(service.link)
              const failure = failures[service.id]
              const current = service.id === selectedId
              return (
                <MenuTarget key={service.id} label={`${service.name} menu`} groups={menu(service)}>
                  <div
                    role="listitem"
                    data-service={service.id}
                    className={`services-row state-${state}${current ? ' is-current' : ''}`}
                    style={{ '--i': index } as CSSProperties}
                    aria-current={current ? 'true' : undefined}
                    aria-label={`${service.name}, ${STATE_WORDS[state].replace('…', '')}`}
                    tabIndex={service.id === focusableId ? 0 : -1}
                    onClick={() => setSelectedId(service.id)}
                    onFocus={event => { if (event.target === event.currentTarget) setSelectedId(service.id) }}
                  >
                    <span className="services-light" aria-hidden="true" />
                    <span className="services-words">
                      <span className="services-name">{service.name}</span>
                      {service.description && <span className="services-description">{service.description}</span>}
                      {failure && <span className="services-failure" role="alert">{failure}</span>}
                    </span>
                    <span className="services-state">{kind ? CHANGE_WORDS[kind].doing : STATE_WORDS[state]}</span>
                    <span className="services-open">
                      {link && (
                        <a className="kit-button" href={link} target="_blank" rel="noreferrer" title={link} tabIndex={-1} onClick={event => event.stopPropagation()}>
                          Open
                        </a>
                      )}
                    </span>
                    <span className="services-act">
                      {!service.controllable ? (
                        <span className="services-readonly" title="The host reports this service but does not let CHROTE change it">Read-only</span>
                      ) : kind || !primary ? null : (
                        <button
                          type="button"
                          className="kit-button services-switch"
                          onClick={event => { event.stopPropagation(); setSelectedId(service.id); void change(service, primary) }}
                        >
                          {CHANGE_WORDS[primary].verb}
                          {current && <span className="kit-chord">Enter</span>}
                        </button>
                      )}
                    </span>
                  </div>
                </MenuTarget>
              )
            })}
          </div>
        )}
      </main>
      <TableSlot active={active} />
      <ResidentColumn active={active} tab="services" reference={reference} />
    </div>
  )
}
