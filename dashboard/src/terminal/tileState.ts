// Which of the four states a bound session's tile is in.
//
// A binding is the operator's stated intent, so nothing here ever removes one.
// The single fact joined in from the host is whether tmux still lists the
// session; everything else the tile already knows from its own connection.

import { getSessionKey, type TmuxSession } from '../types'
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

export interface TileStateInput {
  sessionKey: string
  /** This binding is the one its window shows, and that window is visible. */
  onScreen: boolean
  /** The live set, or null while no trustworthy session list has arrived. */
  liveSessions: Set<string> | null
  connection: TerminalConnectionState
}

export function tileStateFor({ sessionKey, onScreen, liveSessions, connection }: TileStateInput): TileState {
  // An open connection is first-hand proof the session is alive, and it beats a
  // poll that has not caught up with a session the operator just restarted.
  if (connection === 'open') return onScreen ? 'live' : 'idle'
  // Without a trustworthy list, no binding is declared ended on no evidence.
  if (liveSessions !== null && !liveSessions.has(sessionKey)) return 'ended'
  if (!onScreen) return 'idle'
  // 'idle' and 'connecting' are a tile on its way up, not one that lost its
  // session; only a connection that actually dropped means another client won.
  return connection === 'closed' ? 'takenOver' : 'live'
}

/** Taken over and Ended are the same shape: no connection, last frame, an action. */
export function isDetached(state: TileState): state is 'takenOver' | 'ended' {
  return state === 'takenOver' || state === 'ended'
}
