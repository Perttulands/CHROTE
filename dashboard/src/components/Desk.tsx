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

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import Launcher from './Launcher'
import { SessionCommandMark } from './sessionLabel'
import { useTerminalPool } from './TerminalPool'
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

/** How much of the tab an expanded desk takes. */
const EXPANDED_HEIGHT = '40%'

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
  const pool = useTerminalPool()
  const [question, setQuestion] = useState('')
  const [launching, setLaunching] = useState(false)
  const [expanded, setExpanded] = useState(() => readExpanded(label, sessionName ?? ''))
  const askRef = useRef<HTMLInputElement>(null)

  const session = useMemo(
    () => (sessionName ? sessions.find(candidate => candidate.name === sessionName) ?? null : null),
    [sessionName, sessions],
  )

  const state: DeskState = sessionName === undefined || sessionName === ''
    ? 'not configured'
    : session === null
      ? 'not running'
      : session.attached ? 'live' : 'idle'

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

  // The pool holds a terminal for every session bound to a window; a desk over
  // a session no tile shows owns one for as long as it is expanded.
  const pooled = sessionKey ? pool.terminals.get(sessionKey) ?? null : null
  const socketUrl = useMemo(
    () => (expanded && session && !pooled
      ? terminalSocketUrl(session.name, session.unixUser ?? '', 'tile')
      : null),
    [expanded, pooled, session],
  )
  const { session: owned } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)
  const terminal = pooled ?? owned

  const ask = useCallback(async () => {
    const text = question.trim()
    if (!text || !session) return
    setQuestion('')
    const report = await sendToSession(
      session.name,
      { text: `${reference}\n${text}`, files: [], submit: true },
      session.unixUser,
    )
    if (report.outcome === 'sent') announce(`Asked ${session.name}`, 'success')
  }, [announce, question, reference, sendToSession, session])

  const handleAskKeyDown = useCallback((event: React.KeyboardEvent<HTMLInputElement>) => {
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

  const actionWord = state === 'not running' && launchFolder !== undefined
    ? (
      <button type="button" className="desk-word" onClick={() => setLaunching(open => !open)}>
        {launching ? 'Cancel' : 'Launch'}
      </button>
    )
    : (state === 'live' || state === 'idle')
      ? (
        <button type="button" className="desk-word" onClick={() => setExpanded(open => !open)}>
          {expanded ? 'Collapse' : 'Expand'}
        </button>
      )
      : null

  return (
    <div className="desk" style={expanded && terminal ? { height: EXPANDED_HEIGHT } : undefined}>
      <div className="desk-line">
        <span className="desk-label">{label}</span>
        {session && <SessionCommandMark command={session.currentCommand} />}
        {sessionName && <span className="desk-session">{sessionName}</span>}
        <span className="desk-state">{state}</span>
        {actionWord}
        {state !== 'not configured' && (
          <input
            ref={askRef}
            type="text"
            className="desk-ask"
            aria-label={`Ask ${sessionName}`}
            placeholder={placeholder}
            value={question}
            disabled={state === 'not running'}
            onChange={event => setQuestion(event.target.value)}
            onKeyDown={handleAskKeyDown}
          />
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
          <TerminalSurface session={terminal} />
        </div>
      )}
    </div>
  )
}
