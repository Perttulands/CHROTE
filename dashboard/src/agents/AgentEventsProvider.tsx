/**
 * The tellings of an agent event on this device.
 *
 * Each poll of the session list is taken in against the ledger in
 * agentEvents.ts. An event that is news is announced through the toast and,
 * where the device has opted in, a tone and a browser notification while the
 * tab is hidden; it then marks its session's row and tab until the session's
 * tile gains focus, which tells the server the event was seen. Nothing here
 * polls: the session list refreshes as it already did.
 */

import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { getSessionKey } from '../types'
import type { TmuxSession } from '../types'
import { agentEventTitle, sessionOfBinding, takeInAgentEvents, type AgentEventLedger } from './agentEvents'
import { showAgentNotification } from './browserNotifications'
import { audioContext, playTone } from './tones'

const NO_MARKS: ReadonlySet<string> = new Set()

const AgentEventMarksContext = createContext<ReadonlySet<string>>(NO_MARKS)

/** The keys of the sessions whose last event is unseen news on this device. */
export function useAgentEventMarks(): ReadonlySet<string> {
  return useContext(AgentEventMarksContext)
}

async function postSeen(session: TmuxSession): Promise<void> {
  try {
    const response = await fetch('/api/agent/event/seen', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session: session.name,
        ...(session.unixUser ? { unixUser: session.unixUser } : {}),
      }),
      signal: AbortSignal.timeout(10000),
    })
    if (!response.ok) console.warn('Failed to mark the agent event seen:', await response.text())
  } catch (error) {
    console.warn('Failed to mark the agent event seen:', error)
  }
}

export function AgentEventsProvider({ children }: { children: ReactNode }) {
  const { sessions, loading, error, partialAnsweringUsers, focusedWindowKey, workspaces, settings } = useSession()
  const { announce } = useStatus()
  const ledger = useRef<AgentEventLedger>(null)
  // The event time each session was last told seen for, so a poll that has not
  // caught up with the post does not post it again.
  const seenPosted = useRef(new Map<string, string>())
  // The event time each session was marked for. A mark lives while the server
  // still holds that event unseen and the session has not been focused here.
  const [noticed, setNoticed] = useState<ReadonlyMap<string, string>>(() => new Map())
  const settingsRef = useRef(settings)
  settingsRef.current = settings

  // A list has landed when the poll answered, whole or in part. A poll that
  // failed outright says nothing about anyone and seeds nothing.
  const landed = !loading && (error === null || partialAnsweringUsers !== null)

  useEffect(() => {
    if (!landed) return
    const taken = takeInAgentEvents(ledger.current, sessions)
    ledger.current = taken.ledger
    setNoticed(previous => {
      const stale = [...previous.keys()].filter(sessionKey => !taken.ledger.has(sessionKey))
      if (stale.length === 0 && taken.notices.length === 0) return previous
      const next = new Map(previous)
      stale.forEach(sessionKey => next.delete(sessionKey))
      taken.notices.forEach(notice => next.set(notice.sessionKey, notice.time))
      return next
    })
    if (taken.notices.length === 0) return
    const { agentEventToast, agentEventTones, agentEventNotifications } = settingsRef.current
    for (const notice of taken.notices) {
      // The status line is the record of the last event and keeps it either
      // way. Severity is what decides the toast: the slot passes information
      // by, so turning the toast off leaves the line and drops the pop.
      announce(agentEventTitle(notice), agentEventToast ? 'success' : 'info')
      if (agentEventTones) {
        const context = audioContext()
        if (context) playTone(notice.event, context)
      }
      if (agentEventNotifications && document.hidden) showAgentNotification(notice)
    }
  }, [landed, sessions, announce])

  const focusedBinding = useMemo(() => {
    if (!focusedWindowKey) return null
    for (const [workspaceId, workspace] of Object.entries(workspaces)) {
      const found = workspace.windows.find(window => `${workspaceId}-${window.id}` === focusedWindowKey)
      if (found) return found.activeSession
    }
    return null
  }, [focusedWindowKey, workspaces])
  const focused = focusedBinding ? sessionOfBinding(sessions, focusedBinding) : undefined

  // Focus is what seeing means. The mark goes at once; the server is told so
  // the next poll, and every other device, agree.
  useEffect(() => {
    const event = focused?.lastEvent
    if (!focused || !event || event.seen) return
    const sessionKey = getSessionKey(focused.name, focused.unixUser)
    if (seenPosted.current.get(sessionKey) === event.time) return
    seenPosted.current.set(sessionKey, event.time)
    setNoticed(previous => {
      if (!previous.has(sessionKey)) return previous
      const next = new Map(previous)
      next.delete(sessionKey)
      return next
    })
    void postSeen(focused)
  }, [focused])

  // The setting decides what is drawn, not what is known: the ledger and the
  // seen post go on either way, so turning marks back on shows what is still
  // unseen, and focusing a session still tells the server on every device.
  const marked = useMemo(() => {
    if (!settings.agentEventMarks) return NO_MARKS
    const keys = new Set<string>()
    for (const session of sessions) {
      const event = session.lastEvent
      if (!event || event.seen) continue
      const sessionKey = getSessionKey(session.name, session.unixUser)
      if (noticed.get(sessionKey) === event.time) keys.add(sessionKey)
    }
    return keys
  }, [sessions, noticed, settings.agentEventMarks])

  return <AgentEventMarksContext.Provider value={marked}>{children}</AgentEventMarksContext.Provider>
}
