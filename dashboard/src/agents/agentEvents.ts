/**
 * What this device makes of each session's last agent event.
 *
 * The server keeps one event per session, with the time it arrived and
 * whether the operator has focused the session since. This device keeps a
 * ledger of the event times it has already taken in, so a poll can say which
 * of them are news. The first list a page load receives is history: it seeds
 * the ledger and tells nothing, so a reload does not replay the afternoon.
 * After that, an event whose time the ledger does not hold for its session is
 * new, and if the server still holds it unseen it is told: a mark on the row
 * and the tab, the toast, and whatever the device has opted into.
 */

import type { AgentEventKind, LaunchUser, TmuxSession } from '../types'
import { getSessionKey } from '../types'

export interface AgentEventNotice {
  sessionKey: string
  session: string
  unixUser?: LaunchUser
  event: AgentEventKind
  time: string
  summary?: string
}

/** Event times by session key, or null before the first list has landed. */
export type AgentEventLedger = ReadonlyMap<string, string> | null

/**
 * Take a session list in against the ledger. Returns the ledger the list
 * leaves behind and the events that are news: present with a time the ledger
 * did not hold, and still unseen. A seen event is never news, whoever saw it.
 */
export function takeInAgentEvents(
  ledger: AgentEventLedger,
  sessions: readonly TmuxSession[],
): { ledger: ReadonlyMap<string, string>; notices: AgentEventNotice[] } {
  const next = new Map<string, string>()
  const notices: AgentEventNotice[] = []
  for (const session of sessions) {
    const event = session.lastEvent
    if (!event) continue
    const sessionKey = getSessionKey(session.name, session.unixUser)
    next.set(sessionKey, event.time)
    if (ledger === null || ledger.get(sessionKey) === event.time || event.seen) continue
    notices.push({
      sessionKey,
      session: session.name,
      unixUser: session.unixUser,
      event: event.event,
      time: event.time,
      ...(event.summary ? { summary: event.summary } : {}),
    })
  }
  return { ledger: next, notices }
}

/** What the toast and a notification say: the session, then what happened. */
export function agentEventTitle(notice: Pick<AgentEventNotice, 'session' | 'event'>): string {
  return `${notice.session} ${notice.event === 'finished' ? 'finished' : 'needs input'}`
}

/** The row's second line: the summary's first characters, cut with a mark. */
export const SUMMARY_LINE_CHARACTERS = 60

export function summaryLine(summary: string): string {
  const characters = Array.from(summary.trim())
  if (characters.length <= SUMMARY_LINE_CHARACTERS) return characters.join('')
  return `${characters.slice(0, SUMMARY_LINE_CHARACTERS).join('')}…`
}

/**
 * The session a window binding names. A binding is the session's key, or its
 * bare name from before keys carried a user.
 */
export function sessionOfBinding(sessions: readonly TmuxSession[], binding: string): TmuxSession | undefined {
  return sessions.find(session => getSessionKey(session.name, session.unixUser) === binding)
    ?? sessions.find(session => session.name === binding)
}

/** Whether any of a window's bindings names a marked session. */
export function bindingsCarryMark(
  marked: ReadonlySet<string>,
  sessions: readonly TmuxSession[],
  bindings: readonly string[],
): boolean {
  if (marked.size === 0) return false
  return bindings.some(binding => {
    if (marked.has(binding)) return true
    const session = sessionOfBinding(sessions, binding)
    return session !== undefined && marked.has(getSessionKey(session.name, session.unixUser))
  })
}
