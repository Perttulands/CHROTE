/**
 * The Services tab: what runs on this host, and the switch for each.
 *
 * A table like the Beads table, one row per service in the host's catalog
 * order: its name, what it is, its status as a glyph and a word, where it is
 * served, and the one action that changes it, Start or Stop, with Restart,
 * Open and Send in the row's menu. An action reads as what it is doing until
 * the controller has answered and a fresh read has finished, and a failure takes the
 * row's description in the controller's own words. The footer says what was
 * read and when. The resident Service agent has the chosen row as its
 * reference.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
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

const STATE_WORDS: Record<ServiceState, string> = {
  running: 'running',
  stopped: 'stopped',
  starting: 'starting…',
  stopping: 'stopping…',
  partial: 'partial',
  failed: 'failed',
  unknown: 'unknown',
}

const CHANGE_WORDS: Record<Change, { doing: string; done: string; verb: string }> = {
  start: { doing: 'starting…', done: 'started', verb: 'Start' },
  stop: { doing: 'stopping…', done: 'stopped', verb: 'Stop' },
  restart: { doing: 'restarting…', done: 'restarted', verb: 'Restart' },
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
  return `service ${service.id} (${service.name}, ${STATE_WORDS[state].replace('…', '')})`
}

/** Where a link goes, in fewer words than the link: its host and port. */
function linkPlace(link: string): string {
  try { return new URL(link).host } catch { return link }
}

/** The Beads footer's clock: hours, minutes and seconds, the same everywhere. */
function clockTime(at: number): string {
  const time = new Date(at)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(time.getHours())}:${pad(time.getMinutes())}:${pad(time.getSeconds())}`
}

export default function ServicesView({ active }: { active: boolean }) {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()
  const [services, setServices] = useState<Service[] | null>(readCachedServices)
  const [readError, setReadError] = useState<string | null>(null)
  const [changing, setChanging] = useState<Record<string, Change>>({})
  const [failures, setFailures] = useState<Record<string, string>>({})
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [readAt, setReadAt] = useState<number | null>(null)
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
      setReadAt(Date.now())
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
      const expected = kind === 'stop' ? 'stopped' : 'running'
      if (reported?.state === expected) announce(`${service.name} ${CHANGE_WORDS[kind].done}`, 'success')
    } catch (cause) {
      landed.current += 1
      const words = message(cause, `Could not ${kind} ${service.name}`)
      setFailures(previous => ({ ...previous, [service.id]: words }))
      announce(`${service.name}: ${words}`, 'error')
    }
    // Keep the action visible until the subsequent read finishes, even if it fails.
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
  const count = (state: ServiceState) => services?.filter(service => service.state === state).length ?? 0

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
        <div className="services-row services-head" aria-hidden="true">
          <span className="services-cell kit-label">Service</span>
          <span className="services-cell kit-label">Description</span>
          <span className="services-cell kit-label">Status</span>
          <span className="services-cell kit-label">Link</span>
          <span className="services-cell" />
        </div>
        <div className="services-body" role="list" aria-label="Services" ref={listRef} onKeyDown={onListKey}>
          {services?.map(service => {
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
                  className={`services-row state-${state}${current ? ' is-current' : ''}${failure ? ' has-failure' : ''}`}
                  aria-current={current ? 'true' : undefined}
                  aria-label={`${service.name}, ${STATE_WORDS[state].replace('…', '')}`}
                  tabIndex={service.id === focusableId ? 0 : -1}
                  onClick={() => setSelectedId(service.id)}
                  onFocus={event => { if (event.target === event.currentTarget) setSelectedId(service.id) }}
                >
                  <span className="services-cell services-name">{service.name}</span>
                  {failure
                    ? <span className="services-cell services-failure" role="alert" title={failure}>{failure}</span>
                    : <span className="services-cell services-description" title={service.description}>{service.description}</span>}
                  <span className="services-cell services-status">
                    <span className="services-light" aria-hidden="true" />
                    {kind ? CHANGE_WORDS[kind].doing : STATE_WORDS[state]}
                  </span>
                  <span className="services-cell services-link">
                    {link && <a href={link} target="_blank" rel="noreferrer" title={link} tabIndex={-1} onClick={event => event.stopPropagation()}>{linkPlace(link)}</a>}
                  </span>
                  <span className="services-cell services-act">
                    {!service.controllable ? (
                      <span className="services-readonly" title="The host reports this service but does not let CHROTE change it">read-only</span>
                    ) : kind || !primary ? null : (
                      <button
                        type="button"
                        className="kit-button"
                        onClick={event => { event.stopPropagation(); setSelectedId(service.id); void change(service, primary) }}
                      >
                        {CHANGE_WORDS[primary].verb}
                      </button>
                    )}
                  </span>
                </div>
              </MenuTarget>
            )
          })}
        </div>
        <footer className="services-footer" data-ui="services.footer">
          {services === null && readError === null && <span>Reading services</span>}
          {services !== null && (
            <>
              <span>{`${services.length} ${services.length === 1 ? 'service' : 'services'}`}</span>
              <span>{`${count('running')} running`}</span>
            </>
          )}
          {count('partial') > 0 && <span>{`${count('partial')} partial`}</span>}
          {count('failed') > 0 && <span className="services-footer-error">{`${count('failed')} failed`}</span>}
          {readError && <span className="services-footer-error" title={readError}>{readError}</span>}
          {readAt !== null && <span>{`refreshed ${clockTime(readAt)}`}</span>}
        </footer>
      </main>
      <TableSlot active={active} />
      <ResidentColumn active={active} tab="services" reference={reference} />
    </div>
  )
}
