import { useMemo, useState } from 'react'
import type { TmuxSession } from '../types'

/** The dedicated formations-socket session prefix. A persona stem "scout" runs as "mission-scout". */
export const MISSION_SESSION_PREFIX = 'mission-'

/** The formations-socket session name for a persona stem. */
export function missionSessionName(personaStem: string): string {
  return `${MISSION_SESSION_PREFIX}${personaStem}`
}

/** Attach URL for a formations-socket session (the second ttyd proxy, NOT the cockpit's). */
export function formationsTerminalUrl(sessionName: string): string {
  return `/terminal-formations/?arg=${encodeURIComponent(sessionName)}&theme=${encodeURIComponent('{"background":"transparent"}')}`
}

interface MissionSessionPanelProps {
  /** Live formations-socket sessions (fetched once by the parent and shared with the cockpit). */
  sessions: TmuxSession[]
  /** A real session-list error to surface loudly, or '' when healthy. */
  error: string
  loading: boolean
  /** Persona stems assigned on the board (slot agentIds), used to offer spawn affordances. */
  personaStems: string[]
  /** The currently attached session name, or null for none. Controlled by the parent. */
  selectedSession: string | null
  /** Called when the operator picks a session tab. */
  onSelectSession: (sessionName: string | null) => void
  /** Re-fetch the formations session list. */
  onRefresh: () => void
  /** Spawn mission-<stem> on the formations socket and attach it. Rejects on failure. */
  onSpawn: (personaStem: string) => Promise<void>
}

/**
 * The open-board session side-panel, scoped to the FORMATIONS tmux socket.
 *
 * It is deliberately scoped to /api/formations/tmux/* and /terminal-formations/,
 * never the cockpit's /api/tmux or /terminal/ surface or the global SessionContext
 * / IframePool. That keeps the Terminal tabs and their iframe pool untouched. The
 * session list is owned by the parent (MissionsView) so the cockpit's card↔pane
 * linkage and this panel's tabs share one source of truth.
 *
 * The executor never creates sessions — the operator does — so a persona that is
 * assigned on the board but has no live "mission-<stem>" gets an explicit spawn
 * button rather than a fabricated session.
 */
export default function MissionSessionPanel({
  sessions,
  error,
  loading,
  personaStems,
  selectedSession,
  onSelectSession,
  onRefresh,
  onSpawn,
}: MissionSessionPanelProps) {
  const [spawning, setSpawning] = useState<string | null>(null)

  const liveNames = useMemo(() => new Set(sessions.map(session => session.name)), [sessions])

  // Assigned personas whose mission-<stem> session is not yet running. The operator
  // can spawn these; we never fabricate a session for an unspawned persona.
  const missingStems = useMemo(
    () => personaStems.filter(stem => !liveNames.has(missionSessionName(stem))),
    [personaStems, liveNames],
  )

  const spawn = async (stem: string) => {
    const name = missionSessionName(stem)
    setSpawning(name)
    try {
      await onSpawn(stem)
    } finally {
      setSpawning(null)
    }
  }

  return (
    <div className="mission-session-panel" data-testid="mission-session-panel">
      <div className="mission-session-header">
        <span className="panel-title">Mission agents</span>
        <button className="refresh-btn" type="button" onClick={onRefresh} title="Refresh mission sessions">↻</button>
      </div>

      {error && <div className="mission-session-error" role="alert">{error}</div>}

      <div className="mission-session-tabs">
        {loading && sessions.length === 0 && !error && (
          <div className="mission-session-empty">Loading mission sessions…</div>
        )}
        {!loading && sessions.length === 0 && !error && (
          <div className="mission-session-empty">No mission agents running on the formations socket.</div>
        )}
        {sessions.map(session => (
          <button
            key={session.name}
            type="button"
            className={`mission-session-tab${selectedSession === session.name ? ' active' : ''}`}
            data-testid={`mission-session-tab-${session.name}`}
            onClick={() => onSelectSession(session.name)}
          >
            {session.name}
          </button>
        ))}
      </div>

      {missingStems.length > 0 && (
        <div className="mission-session-spawn">
          <div className="mission-session-spawn-title">Assigned personas without a live session</div>
          {missingStems.map(stem => (
            <button
              key={stem}
              type="button"
              className="mission-session-spawn-btn"
              disabled={spawning === missionSessionName(stem)}
              onClick={() => void spawn(stem)}
            >
              {spawning === missionSessionName(stem) ? `Spawning ${missionSessionName(stem)}…` : `Spawn ${missionSessionName(stem)}`}
            </button>
          ))}
        </div>
      )}

      <div className="mission-session-body">
        {selectedSession ? (
          <iframe
            key={selectedSession}
            title={`Mission agent — ${selectedSession}`}
            src={formationsTerminalUrl(selectedSession)}
            allow="clipboard-read; clipboard-write"
            className="mission-session-iframe"
          />
        ) : (
          <div className="mission-session-placeholder">
            Select a mission agent above to attach its terminal.
          </div>
        )}
      </div>
    </div>
  )
}
