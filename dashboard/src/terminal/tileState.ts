// Which of the four states a bound session's tile is in.
//
// A binding is the operator's stated intent, so nothing here ever removes one.
// The single fact joined in from the host is whether tmux still lists the
// session; everything else the tile already knows from its own connection.

import { getSessionKey, getSessionUserFromKey, type LaunchUser, type TmuxSession } from '../types'
import type { TerminalConnectionState } from './terminalSession'

export type TileState = 'idle' | 'live' | 'takenOver' | 'ended'

/**
 * The live set a binding is matched against. Bindings exist in both the
 * qualified `user:name` form and the bare `name` form, so both are indexed.
 */
export function liveSessionKeys(sessions: readonly TmuxSession[]): Set<string> {
  const live = new Set<string>()
  sessions.forEach(session => {
    live.add(getSessionKey(session.name, session.unixUser))
    live.add(session.name)
  })
  return live
}

/** What the last session poll can be held to have said about a binding. */
export interface SessionEvidence {
  /** The live set, or null while no trustworthy session list has arrived. */
  live: Set<string> | null
  /**
   * The Unix users whose tmux answered a *partial* response. Only a binding
   * qualified by one of them may be joined against `live`: the response says
   * nothing about the users whose socket failed, and a bare binding names no
   * user to check. Null when the response was whole and every binding joins.
   */
  answering: ReadonlySet<LaunchUser> | null
}

/** Nothing has been heard, so nothing is claimed about any binding. */
export const NO_SESSION_EVIDENCE: SessionEvidence = { live: null, answering: null }

export interface SessionEvidenceInput {
  sessions: readonly TmuxSession[]
  loading: boolean
  error: string | null
  /** From the poll: the users that answered, when the last response was partial. */
  partialAnsweringUsers: readonly LaunchUser[] | null
}

/**
 * Turn a poll result into what it is entitled to claim.
 *
 * A poll that failed outright reports nothing about anyone, so no binding is
 * joined. A *partial* poll is different: one configured user's tmux failing
 * used to suppress the join for every binding in the dashboard, including
 * bindings under users that answered perfectly well, which left a session that
 * really had died showing as Live for as long as the other socket stayed
 * broken. Trust is scoped per user instead.
 */
export function sessionEvidenceFrom(
  { sessions, loading, error, partialAnsweringUsers }: SessionEvidenceInput,
): SessionEvidence {
  // `== null` deliberately: an empty answering list is a partial response that
  // nobody answered, which is not the same as a response that was whole.
  if (loading || (error !== null && partialAnsweringUsers == null)) return NO_SESSION_EVIDENCE
  return {
    live: liveSessionKeys(sessions),
    answering: partialAnsweringUsers == null ? null : new Set(partialAnsweringUsers),
  }
}

/**
 * What the tile layer holds after a poll: the last evidence that actually said
 * something, kept when the next poll says nothing at all.
 *
 * A failed poll is the absence of news, not news that a session came back, and
 * a dead session does not come back — so re-deciding an Ended verdict on a
 * failure can only produce a wrong answer and a Reclaim button that dials a
 * session tmux does not have. The verdict stands until a poll that answered
 * replaces it, which is also what lets a restarted session read Live again. An
 * open connection is first-hand proof and overrides all of this anyway.
 *
 * This is a cache, deliberately: one poll deep, replaced whole by the next
 * response, and never consulted while the tile has its own connection.
 */
export function retainSessionEvidence(previous: SessionEvidence, fresh: SessionEvidence): SessionEvidence {
  return fresh.live === null ? previous : fresh
}

/**
 * Whether the last poll positively says this binding's session is gone. False
 * both for a session tmux still lists and for one the poll cannot speak about:
 * no binding is ever declared ended on absent evidence.
 */
export function isSessionEnded(sessionKey: string, evidence: SessionEvidence): boolean {
  if (evidence.live === null) return false
  if (evidence.answering !== null) {
    const unixUser = getSessionUserFromKey(sessionKey)
    if (!unixUser || !evidence.answering.has(unixUser)) return false
  }
  return !evidence.live.has(sessionKey)
}

export interface TileStateInput {
  sessionKey: string
  /** This binding is the one its window shows, and that window is visible. */
  onScreen: boolean
  /** What the last session poll can say about this binding. */
  evidence: SessionEvidence
  connection: TerminalConnectionState
}

export function tileStateFor({ sessionKey, onScreen, evidence, connection }: TileStateInput): TileState {
  // An open connection is first-hand proof the session is alive, and it beats a
  // poll that has not caught up with a session the operator just restarted.
  if (connection === 'open') return onScreen ? 'live' : 'idle'
  if (isSessionEnded(sessionKey, evidence)) return 'ended'
  if (!onScreen) return 'idle'
  // 'idle' and 'connecting' are a tile on its way up, not one that lost its
  // session; only a connection that actually dropped means another client won.
  return connection === 'closed' ? 'takenOver' : 'live'
}

/** Taken over and Ended are the same shape: no connection, last frame, an action. */
export function isDetached(state: TileState): state is 'takenOver' | 'ended' {
  return state === 'takenOver' || state === 'ended'
}
