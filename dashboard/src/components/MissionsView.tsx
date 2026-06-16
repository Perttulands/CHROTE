import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import FormationsCockpit, { FormationsCockpitHandle } from './FormationsCockpit'
import MissionsGallery from './MissionsGallery'
import MissionSessionPanel, { missionSessionName } from './MissionSessionPanel'
import { useToast } from '../context/ToastContext'
import type { BoardDocument } from './formationsTypes'
import type { SessionsResponse, TmuxSession } from '../types'

const FORMATIONS_SESSIONS_ENDPOINT = '/api/formations/tmux/sessions'

interface MissionsViewProps {
  active?: boolean
}

/**
 * The Missions tab. Two modes:
 *   - gallery (no board open): MissionsGallery lists / creates Mission Boards.
 *   - open board: the FormationsCockpit canvas + a session side-panel scoped to
 *     the FORMATIONS tmux socket.
 *
 * This view owns the formations-session list so the cockpit's card↔pane linkage
 * and the side-panel's tabs share one source of truth, and so the cockpit's
 * Terminal-tab machinery (SessionContext / IframePool / /api/tmux) is never touched.
 */
export default function MissionsView({ active = true }: MissionsViewProps) {
  const { addToast } = useToast()
  const [openSlug, setOpenSlug] = useState<string | null>(null)
  const [board, setBoard] = useState<BoardDocument | null>(null)
  const [selectedSession, setSelectedSession] = useState<string | null>(null)
  const [sessions, setSessions] = useState<TmuxSession[]>([])
  const [sessionError, setSessionError] = useState('')
  const [sessionsLoading, setSessionsLoading] = useState(true)
  const cockpitRef = useRef<FormationsCockpitHandle>(null)

  const refreshSessions = useCallback(async () => {
    try {
      const response = await fetch(FORMATIONS_SESSIONS_ENDPOINT, { signal: AbortSignal.timeout(10000) })
      const data: SessionsResponse = await response.json()
      if (data.error) {
        // Fail loud: a real server error is surfaced, not hidden behind an empty list.
        setSessionError(data.error)
        setSessions([])
      } else {
        setSessionError('')
        setSessions(data.sessions || [])
      }
    } catch (e) {
      setSessionError(e instanceof Error ? `Failed to list mission sessions: ${e.message}` : 'Failed to list mission sessions')
    } finally {
      setSessionsLoading(false)
    }
  }, [])

  // Poll the formations socket only while a board is open and the tab is active.
  useEffect(() => {
    if (!openSlug || !active) return
    refreshSessions()
    const interval = setInterval(refreshSessions, 4000)
    return () => clearInterval(interval)
  }, [openSlug, active, refreshSessions])

  const liveSessionNames = useMemo(() => new Set(sessions.map(session => session.name)), [sessions])

  // Assigned persona stems are the slot agentIds across the open board's formations.
  const personaStems = useMemo(() => {
    if (!board) return []
    const stems = new Set<string>()
    for (const formation of board.formations || []) {
      for (const slot of formation.slots || []) {
        if (slot.agentId) stems.add(slot.agentId)
      }
    }
    return [...stems]
  }, [board])

  const spawnSession = useCallback(async (personaStem: string) => {
    const name = missionSessionName(personaStem)
    try {
      const response = await fetch(FORMATIONS_SESSIONS_ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
        signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) {
        const text = await response.text()
        const message = `Failed to spawn ${name}: ${text || response.status}`
        setSessionError(message)
        addToast(message, 'error')
        return
      }
      setSessionError('')
      addToast(`Mission agent '${name}' spawned`, 'success')
      await refreshSessions()
      setSelectedSession(name)
    } catch (e) {
      const message = e instanceof Error ? `Failed to spawn ${name}: ${e.message}` : `Failed to spawn ${name}`
      setSessionError(message)
      addToast(message, 'error')
    }
  }, [addToast, refreshSessions])

  const openBoard = useCallback((slug: string) => {
    setOpenSlug(slug)
    setSelectedSession(null)
    setSessions([])
    setSessionError('')
    setSessionsLoading(true)
  }, [])

  const closeBoard = useCallback(() => {
    setOpenSlug(null)
    setBoard(null)
    setSelectedSession(null)
  }, [])

  if (!openSlug) {
    return <MissionsGallery onOpenBoard={openBoard} />
  }

  return (
    <div className="missions-open-board" data-testid="missions-open-board">
      <div className="missions-open-header">
        <button type="button" className="missions-back-btn" onClick={closeBoard}>
          ← Mission Boards
        </button>
        <button
          type="button"
          className="missions-edit-mission-btn"
          onClick={() => cockpitRef.current?.openMissionEditor()}
          disabled={!board?.missions?.length}
          title="Edit this Mission Board's title, goal and bead"
        >
          Edit mission
        </button>
      </div>
      <div className="missions-open-body">
        <div className="missions-canvas-host" data-testid="missions-canvas-host">
          <FormationsCockpit
            ref={cockpitRef}
            active={active}
            selectedSlug={openSlug}
            liveSessionNames={liveSessionNames}
            onViewAgentSession={setSelectedSession}
            onBoardLoaded={setBoard}
          />
        </div>
        <MissionSessionPanel
          sessions={sessions}
          error={sessionError}
          loading={sessionsLoading}
          personaStems={personaStems}
          selectedSession={selectedSession}
          onSelectSession={setSelectedSession}
          onRefresh={refreshSessions}
          onSpawn={spawnSession}
        />
      </div>
    </div>
  )
}
