// Which of the five states a bound session's tile is in.
//
// A binding is the operator's stated intent, so nothing here ever removes one.
// Two facts are joined in from the host — whether tmux still lists the session,
// and whether another client holds it — and everything else the tile already
// knows from its own connection, including whether that connection ended or
// was lost.

import { getSessionKey, getSessionUserFromKey, type LaunchUser, type TmuxSession } from '../types'
import type { TerminalConnectionState } from './terminalSession'

export type TileState = 'idle' | 'live' | 'takenOver' | 'lost' | 'ended'

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

/**
 * The bindings whose session tmux reports a client CHROTE did not create, such
 * as an SSH login. Indexed in both key forms, like the live set.
 *
 * A tile attaches with `-d`, so dialling one of these takes the session away
 * from someone who is using it. That is a fine thing for the operator to ask
 * for and a bad thing to do behind his back, which is the whole reason this
 * set exists.
 */
export function heldElsewhereSessionKeys(sessions: readonly TmuxSession[]): Set<string> {
  const held = new Set<string>()
  sessions.forEach(session => {
    if ((session.foreignClients ?? []).length === 0) return
    held.add(getSessionKey(session.name, session.unixUser))
    held.add(session.name)
  })
  return held
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
  /**
   * The live set of the last response that spoke for every user, kept while a
   * partial one cannot. It decides the bindings `answering` excludes, and only
   * those; the users that did answer are always read from `live`.
   */
  held?: Set<string> | null
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
 * something, kept wherever the next poll says nothing.
 *
 * A poll that says nothing is the absence of news, not news that a session came
 * back, and a dead session does not come back — so re-deciding an Ended verdict
 * on it can only produce a wrong answer and a Reclaim button that dials a
 * session tmux does not have. The verdict stands until a poll that can speak
 * replaces it, which is also what lets a restarted session read Live again. An
 * open connection is first-hand proof and overrides all of this anyway.
 *
 * A poll says nothing in two shapes, and the rule is the same for both. One
 * that failed outright says nothing about anybody, so the last answer is kept
 * whole. A *partial* one says nothing about the users whose socket failed, so
 * the last whole answer is kept for exactly those bindings, while the users
 * that did answer are read from the fresh response as usual.
 *
 * This is a cache, deliberately: never more than one held set, dropped the
 * moment a whole response lands, and never consulted while the tile has its own
 * connection.
 */
export function retainSessionEvidence(previous: SessionEvidence, fresh: SessionEvidence): SessionEvidence {
  if (fresh.live === null) return previous
  if (fresh.answering === null) return fresh
  const held = previous.answering === null ? previous.live : previous.held
  return held == null ? fresh : { ...fresh, held }
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
    if (!unixUser || !evidence.answering.has(unixUser)) {
      // This response cannot speak for the binding, so it overturns nothing:
      // the last one that could still decides, and nothing is claimed when
      // there has not been one.
      return evidence.held != null && !evidence.held.has(sessionKey)
    }
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
  /** The last poll saw a client CHROTE did not create attached to this session. */
  heldElsewhere: boolean
}

export function tileStateFor({ sessionKey, onScreen, evidence, connection, heldElsewhere }: TileStateInput): TileState {
  // An open connection is first-hand proof the session is alive, and it beats a
  // poll that has not caught up with a session the operator just restarted.
  if (connection === 'open') return onScreen ? 'live' : 'idle'
  if (isSessionEnded(sessionKey, evidence)) return 'ended'
  if (!onScreen) return 'idle'
  // The host ends a terminal when the pty hangs up, and on a live session that
  // means another client attached with -d and won it.
  if (connection === 'closed') return 'takenOver'
  // A connection that was lost says nothing about who holds the session, so ask
  // tmux. Nobody else attached means the tile simply fell off — every
  // chrote-srv restart does this to every open tile — and it dials again. A
  // client that is really there is a takeover the operator has to choose,
  // because taking it back would evict them.
  if (connection === 'dropped') return heldElsewhere ? 'takenOver' : 'lost'
  // 'idle' and 'connecting' are a tile on its way up, not one that lost its
  // session.
  return 'live'
}

/** Taken over, Lost and Ended are the same shape: no connection, last frame, an action. */
export function isDetached(state: TileState): state is 'takenOver' | 'lost' | 'ended' {
  return state === 'takenOver' || state === 'lost' || state === 'ended'
}
