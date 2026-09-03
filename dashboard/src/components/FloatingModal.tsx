/**
 * Peek: a second look at a session, docked at the left of the workspace.
 *
 * It is a sheet, not a modal. It opens over the Sessions panel it was reached
 * from, and its width snaps to the nearest tile boundary at or below 60% of the
 * workspace, so the tiles to its right stay whole and readable rather than cut
 * mid-glyph. There is no backdrop and no drag handle: Escape closes it, the
 * header's close does, and a click anywhere else does what that click means.
 */

import { useEffect, useMemo, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'
import TerminalSurface, { useTerminalSession } from './TerminalSurface'
import { SessionCommandMark } from './sessionLabel'
import Sheet from './Sheet'
import { terminalSocketUrl } from '../terminal/ttydProtocol'
import { isSessionEnded } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'

/** The most of the workspace Peek is allowed to take before it snaps down. */
export const PEEK_MAX_SHARE = 0.6

/**
 * The widest tile boundary that fits inside the share, measured from the live
 * grid rather than guessed from the layout count: a boundary is where a tile
 * actually ends, whatever the grid is doing.
 */
export function peekExtent(): string {
  if (typeof document === 'undefined') return `${PEEK_MAX_SHARE * 100}%`
  const content = document.querySelector<HTMLElement>('.dashboard-content')
  if (!content) return `${PEEK_MAX_SHARE * 100}%`
  const total = content.clientWidth
  const cap = total * PEEK_MAX_SHARE
  const left = content.getBoundingClientRect().left
  const boundaries = Array.from(
    document.querySelectorAll<HTMLElement>('.terminal-workspace-dock[data-active="true"] .terminal-window'),
  )
    .map(tile => tile.getBoundingClientRect().right - left)
    .filter(edge => edge > 0 && edge <= cap)
  return boundaries.length > 0 ? `${Math.round(Math.max(...boundaries))}px` : `${PEEK_MAX_SHARE * 100}%`
}

function FloatingModal() {
  const { floatingSession, closeFloatingModal, openSendToSession, settings, sessions } = useSession()
  const [extent, setExtent] = useState<string>(`${PEEK_MAX_SHARE * 100}%`)

  const displayName = floatingSession ? getSessionNameFromKey(floatingSession) : ''
  const keyUser = floatingSession ? getSessionUserFromKey(floatingSession) : ''
  const matchingSessions = !floatingSession
    ? []
    : keyUser
      ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === floatingSession)
      : sessions.filter(item => item.name === displayName)
  const session = matchingSessions.length === 1 ? matchingSessions[0] : undefined
  const unixUser = session?.unixUser ?? keyUser
  const canOpenSession = Boolean(floatingSession && (session || unixUser.trim()))

  // The same join the tile makes, asked of the same answer. A glance at a
  // session tmux no longer lists is entitled to the same explanation the tile
  // gives, rather than a dead terminal and no reason for it.
  const evidence = useSessionEvidence()
  const ended = floatingSession !== null && isSessionEnded(floatingSession, evidence)

  // Peek owns its terminal for the life of the sheet: it is a second observer
  // of the session, not the tile's terminal moved onto the overlay. It attaches
  // as an observer, so it never displaces the tile or resizes the window.
  const socketUrl = useMemo(
    () => (canOpenSession ? terminalSocketUrl(displayName, unixUser, 'peek') : null),
    [canOpenSession, displayName, unixUser],
  )
  // The URL is kept even once the session has ended, so the terminal holding
  // the last frame is not disposed; `connect` is what stops it dialling again.
  const { session: terminal, connectionState } = useTerminalSession(socketUrl, settings.fontSize, settings.hideScrollbar)

  // The grid the boundary comes from is laid out in the same paint that opened
  // the sheet, so the measurement is taken after it and again on every resize.
  useEffect(() => {
    if (!floatingSession) return
    const measure = () => setExtent(peekExtent())
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [floatingSession])

  if (!floatingSession) return null

  const header = (
    <>
      <SessionCommandMark command={session?.currentCommand} />
      <span className="peek-name">{displayName}</span>
      {canOpenSession && !ended && connectionState !== 'open' && (
        <span className="terminal-loading-state">
          {connectionState === 'closed' || connectionState === 'dropped' ? 'Terminal disconnected' : 'Loading terminal…'}
        </span>
      )}
      <button type="button" className="peek-send" onClick={() => openSendToSession(floatingSession)}>Send</button>
      <button type="button" className="peek-close" onClick={closeFloatingModal} aria-label="Close Peek">×</button>
    </>
  )

  return (
    <Sheet open edge="left" extent={extent} label={`Peek ${displayName}`} onClose={closeFloatingModal} header={header}>
      <div className={ended ? 'peek-body detached' : 'peek-body'}>
        {canOpenSession ? (
          <>
            <TerminalSurface session={terminal} connect={!ended} />
            {ended && (
              <div className="terminal-tile-detached" data-tile-state="ended" role="status">
                <span className="terminal-tile-detached-note">
                  {displayName} ended. This frame shows its last output.
                </span>
              </div>
            )}
          </>
        ) : (
          <div className="empty-window-content">Ambiguous legacy session name; attach the user-qualified session from the session list.</div>
        )}
      </div>
    </Sheet>
  )
}

export default FloatingModal
import './FloatingModal.css'
