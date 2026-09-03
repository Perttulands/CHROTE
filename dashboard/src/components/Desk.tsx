/**
 * A desk: the one line through which a tab talks to the agent that tends it.
 *
 * The Library has a Front desk staffed by the Librarian; the Agents tab has a
 * Tender desk staffed by the tender. Both are the same thing — a named session,
 * its state as a word, and a field that asks it a question about whatever the
 * operator is looking at — so both are this component.
 *
 * Every message carries a reference as its first line ("library
 * preferences/workflow.md", "agents /srv/chrote claude-code"), so the agent is
 * never asked about something it cannot see. The desk expands into the
 * session's own terminal when the answer needs reading rather than skimming,
 * and the expansion is remembered for that session.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import Launcher from './Launcher'
import { SessionCommandMark } from './sessionLabel'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { getSessionKey } from '../types'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import './Desk.css'

export interface DeskProps {
  /** What the desk is called: "Front desk" or "Tender". */
  label: string
  /** The tmux session that staffs it; absent when the host configured none. */
  sessionName?: string
  /** The first line of every message the desk sends. */
  reference: string
  placeholder: string
  /** The folder the launcher offers when the session is not running. */
  launchFolder?: string
}

/** What the state word can read, in the order the desk decides it. */
export type DeskState = 'live' | 'idle' | 'not running' | 'not configured'

function expansionKey(label: string, sessionName: string): string {
  return `chrote.desk.${label}.${sessionName}`
}

function readExpanded(label: string, sessionName: string): boolean {
  if (!sessionName) return false
  try {
    return window.localStorage.getItem(expansionKey(label, sessionName)) === 'true'
  } catch {
    return false
  }
}

export default function Desk({ label, sessionName, reference, placeholder, launchFolder }: DeskProps) {
  const { sessions, settings, openSendToSession, sendToSession, refreshSessions } = useSession()
  const { announce } = useStatus()
  const [question, setQuestion] = useState('')
  const [sending, setSending] = useState(false)
  const [launching, setLaunching] = useState(false)
  const [expanded, setExpanded] = useState(() => readExpanded(label, sessionName ?? ''))

  const session = useMemo(
    () => (sessionName ? sessions.find(candidate => candidate.name === sessionName) ?? null : null),
    [sessionName, sessions],
  )

  const state: DeskState = sessionName === undefined || sessionName === ''
    ? 'not configured'
    : session === null
      ? 'not running'
      : session.attached ? 'live' : 'idle'
  const running = state === 'live' || state === 'idle'

  const sessionKey = session ? getSessionKey(session.name, session.unixUser) : ''

  useEffect(() => {
    setExpanded(readExpanded(label, sessionName ?? ''))
    setLaunching(false)
  }, [label, sessionName])

  useEffect(() => {
    if (!sessionName) return
    try {
      window.localStorage.setItem(expansionKey(label, sessionName), expanded ? 'true' : 'false')
    } catch {
      // A device that refuses storage still gets a working desk.
    }
  }, [expanded, label, sessionName])

  /*
   * The desk owns the terminal it shows rather than taking the pool's. A pooled
   * terminal belongs to the tile that bound it, and a terminal can be attached
   * in one place at a time: mounting it here would move it out of that tile and
   * leave the operator's workspace blank behind him. Peek answers the same
   * problem the same way.
   */
  const socketUrl = useMemo(
    () => (expanded && session ? terminalSocketUrl(session.name, session.unixUser ?? '', 'tile') : null),
    [expanded, session],
  )
  const { session: terminal } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  const ask = useCallback(async () => {
    const text = question.trim()
    if (!text || !session || sending) return
    setSending(true)
    const report = await sendToSession(
      session.name,
      { text: `${reference}\n${text}`, files: [], submit: true },
      session.unixUser,
    )
    setSending(false)
    // A question that did not land stays in the field: the status line already
    // carries the reason, and retyping it is the operator's time.
    if (report.outcome === 'sent') {
      setQuestion('')
      announce(`Asked ${session.name}`, 'success')
    }
  }, [announce, question, reference, sendToSession, sending, session])

  const handleAskKeyDown = useCallback((event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault()
      void ask()
      return
    }
    // The drawer is the way to send a file or to hold the message back; from
    // the desk it opens on the same session, with the same reference.
    if (event.altKey && event.key.toLowerCase() === 's' && sessionKey) {
      event.preventDefault()
      event.stopPropagation()
      openSendToSession({ targetSessionKey: sessionKey, reference, note: question })
    }
  }, [ask, openSendToSession, question, reference, sessionKey])

  const askField = state === 'not configured' ? null : (
    <input
      type="text"
      className={`desk-ask ${expanded ? 'desk-ask-wide' : ''}`}
      aria-label={`Ask ${sessionName}`}
      placeholder={placeholder}
      value={question}
      disabled={state === 'not running' || sending}
      onChange={event => setQuestion(event.target.value)}
      onKeyDown={handleAskKeyDown}
    />
  )

  return (
    <div className={`desk ${expanded && terminal ? 'desk-expanded' : ''}`}>
      <div className="desk-line" data-ui="desk">
        <span className="desk-label">{label}</span>
        {session && <SessionCommandMark command={session.currentCommand} />}
        {sessionName && <span className="desk-session">{sessionName}</span>}
        <span className="desk-state">{state}</span>
        {state === 'not running' && launchFolder !== undefined && (
          <button type="button" className="desk-word desk-launch" onClick={() => setLaunching(open => !open)}>
            {launching ? 'Cancel' : 'Launch'}
          </button>
        )}
        {/* Collapsed, the question shares the line; expanded, it takes a row of
            its own beneath, because then it is the one place input goes here. */}
        {!expanded && askField}
        {running && (
          <button type="button" className="desk-word desk-expand" onClick={() => setExpanded(open => !open)}>
            {expanded ? 'Collapse' : 'Expand'}
          </button>
        )}
      </div>
      {launching && launchFolder !== undefined && (
        <div className="desk-launcher">
          <Launcher
            workspaceId="terminal1"
            initialFolder={launchFolder}
            onLaunched={() => { setLaunching(false); refreshSessions() }}
          />
        </div>
      )}
      {expanded && askField}
      {expanded && terminal && (
        <div className="desk-tile">
          <div className="desk-tile-head">
            <SessionCommandMark command={session?.currentCommand} />
            <span className="desk-session">{sessionName}</span>
            <button
              type="button"
              className="desk-word desk-tile-send"
              onClick={() => { if (sessionKey) openSendToSession({ targetSessionKey: sessionKey, reference }) }}
            >
              Send
            </button>
          </div>
          <div className="desk-tile-body">
            <TerminalSurface session={terminal} />
          </div>
        </div>
      )}
    </div>
  )
}
