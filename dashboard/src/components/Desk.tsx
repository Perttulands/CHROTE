/**
 * A desk: the one line at the foot of a tab where a named agent is on duty.
 *
 * The Library's Front desk and the Agents tab's Tender desk are the same
 * furniture with different names, so they are one component. The line says who
 * is on duty and whether they are there; the Ask field sends a question to that
 * session with the surface's own reference as its first line; Expand turns the
 * line into that session's terminal without leaving the tab.
 *
 * The desk owns the terminal it shows rather than taking the pool's. A pooled
 * terminal belongs to the tile that bound it, and a terminal can be attached in
 * one place at a time: borrowing it here would blank the operator's tile. Peek
 * answers the same problem the same way.
 */

import { useCallback, useEffect, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import { SessionCommandMark } from './sessionLabel'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { getSessionKey } from '../types'
import './Desk.css'

export interface DeskProps {
  /** "Front desk" or "Tender": what this desk is called, in the operator's words. */
  label: string
  /** The configured tmux session. Undefined means nobody is on duty here. */
  sessionName?: string
  /** The first line of every message the desk sends, naming what is on screen. */
  reference: string
  /** What the Ask field says before anything is typed. */
  placeholder: string
  /** The folder the launcher is offered when the session is not running. */
  launchFolder?: string
}

/** The word for what the desk found, in the order the operator cares about. */
export type DeskState = 'live' | 'idle' | 'not running' | 'not configured'

/**
 * Whether the desk is expanded, remembered across tab switches.
 *
 * The expansion is a working posture, not a preference: the operator who
 * opened the Librarian's terminal expects it still open when he comes back
 * from a terminal tab. It is keyed by desk and session so the Library's desk
 * and the Agents tab's desk remember separately, and it is deliberately not
 * persisted: a reload is a fresh start.
 */
const expandedDesks = new Set<string>()

function deskKey(label: string, sessionName: string | undefined): string {
  return `${label} ${sessionName ?? ''}`
}

export function resetDesksForTest(): void {
  expandedDesks.clear()
}

function Desk({ label, sessionName, reference, placeholder, launchFolder }: DeskProps) {
  const { sessions, settings, openSendToSession, createSession, sendToSession } = useSession()
  const { announce } = useStatus()
  const [question, setQuestion] = useState('')
  const [sending, setSending] = useState(false)
  const [expanded, setExpanded] = useState(() => expandedDesks.has(deskKey(label, sessionName)))

  const session = sessionName ? sessions.find(candidate => candidate.name === sessionName) : undefined
  const state: DeskState = !sessionName
    ? 'not configured'
    : !session
      ? 'not running'
      : session.attached
        ? 'live'
        : 'idle'
  const running = state === 'live' || state === 'idle'

  useEffect(() => {
    const key = deskKey(label, sessionName)
    if (expanded) expandedDesks.add(key)
    else expandedDesks.delete(key)
  }, [expanded, label, sessionName])

  // A desk whose session went away has nothing to show, so it folds back to
  // the line rather than holding a dead terminal open.
  useEffect(() => {
    if (!running) setExpanded(false)
  }, [running])

  const socketUrl = expanded && session && sessionName
    ? terminalSocketUrl(sessionName, session.unixUser ?? '', 'tile')
    : null
  const { session: terminal } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  const ask = useCallback(async () => {
    const text = question.trim()
    if (!text || !session || !sessionName || sending) return
    setSending(true)
    const report = await sendToSession(sessionName, {
      text: `${reference}\n${text}`,
      submit: true,
      files: [],
    }, session.unixUser)
    setSending(false)
    if (report.outcome === 'sent') {
      setQuestion('')
      announce(`Asked ${sessionName}`, 'info')
    }
  }, [announce, question, reference, sendToSession, sending, session, sessionName])

  const handleAskKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      void ask()
      return
    }
    // Alt+S from the Ask field is the same hand-off with room to edit it: the
    // drawer opens on this session with the reference already in place.
    if (event.altKey && event.key.toLowerCase() === 's' && session && sessionName) {
      event.preventDefault()
      event.stopPropagation()
      openSendToSession({
        targetSessionKey: getSessionKey(sessionName, session.unixUser),
        reference,
        note: question.trim() || undefined,
      })
    }
  }

  const launch = useCallback(() => {
    if (!sessionName) return
    void createSession({ name: sessionName, cwd: launchFolder })
  }, [createSession, launchFolder, sessionName])

  const askField = (
    <input
      className={`desk-ask ${expanded ? 'desk-ask-wide' : ''}`}
      type="text"
      value={question}
      placeholder={placeholder}
      aria-label={`Ask ${sessionName}`}
      disabled={sending}
      onChange={event => setQuestion(event.target.value)}
      onKeyDown={handleAskKeyDown}
    />
  )

  return (
    <div className={`desk ${expanded ? 'desk-expanded' : ''}`} data-testid="desk">
      <div className="desk-line">
        <span className="desk-label">{label}</span>
        {session && <SessionCommandMark command={session.currentCommand} />}
        {sessionName && <span className="desk-session">{sessionName}</span>}
        <span className="desk-state">{state}</span>
        {sessionName && !running && (
          <button type="button" className="desk-action desk-launch" onClick={launch}>Launch</button>
        )}
        {running && (
          <>
            {!expanded && askField}
            <button
              type="button"
              className="desk-action desk-expand"
              onClick={() => setExpanded(open => !open)}
            >
              {expanded ? 'Collapse' : 'Expand'}
            </button>
          </>
        )}
      </div>
      {/* Expanded, the question gets the width of the desk: it is the one
          place input goes on this surface, and the terminal is under it. */}
      {expanded && running && askField}
      {expanded && terminal && (
        <div className="desk-tile">
          <TerminalSurface session={terminal} />
        </div>
      )}
    </div>
  )
}

export default Desk
