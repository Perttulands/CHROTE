import { FormEvent, useCallback, useEffect, useState } from 'react'
import { ApiRequestError, createBoard, fetchBoardSummaries } from './formationsApi'
import type { BoardSummary } from './formationsTypes'

interface MissionsGalleryProps {
  /** Open a board's canvas + session side-panel by slug. */
  onOpenBoard: (slug: string) => void
}

/** Human label for a board's latest run; honest "never run" when there is none. */
function runLabel(board: BoardSummary): string {
  return board.latestRun ? board.latestRun.status : 'never run'
}

/**
 * Missions tab landing view: a gallery of Mission Boards. Mirrors AgentsView's
 * list+create+detail shape. A failed boards fetch or create surfaces a clear,
 * precise message — never fake data or a silent empty gallery.
 */
export default function MissionsGallery({ onOpenBoard }: MissionsGalleryProps) {
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [slug, setSlug] = useState('')
  const [title, setTitle] = useState('')
  const [goal, setGoal] = useState('')
  const [beadId, setBeadId] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const list = await fetchBoardSummaries()
      setBoards(list)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load Mission Boards')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const create = useCallback(async (event: FormEvent) => {
    event.preventDefault()
    if (creating) return
    const trimmedSlug = slug.trim()
    const trimmedTitle = title.trim()
    const trimmedGoal = goal.trim()
    if (!trimmedSlug || !trimmedTitle || !trimmedGoal) {
      setError('Slug, title and mission goal are all required to create a Mission Board.')
      return
    }
    setCreating(true)
    try {
      const board = await createBoard({ slug: trimmedSlug, title: trimmedTitle, goal: trimmedGoal, beadId })
      setSlug('')
      setTitle('')
      setGoal('')
      setBeadId('')
      setError('')
      await refresh()
      onOpenBoard(board.slug)
    } catch (err) {
      // ApiRequestError carries the server status/code so the message is precise
      // (409 duplicate slug, 400 invalid slug, 400 malformed bead).
      if (err instanceof ApiRequestError) {
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Failed to create Mission Board')
      }
    } finally {
      setCreating(false)
    }
  }, [beadId, creating, goal, onOpenBoard, refresh, slug, title])

  return (
    <div className="missions-gallery" data-testid="missions-gallery">
      <div className="oracle-status-bar">
        <div className="oracle-status-items">
          <span className="oracle-stat">Mission Boards: {loading ? '--' : boards.length}</span>
        </div>
        <div className="oracle-status-right">
          <button className="oracle-refresh-btn" type="button" onClick={refresh} disabled={loading}>
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {error && <div className="oracle-empty" role="alert">{error}</div>}

      <div className="oracle-content">
        <div className="oracle-agents-section">
          <form className="mission-create-form" onSubmit={create} aria-label="Create Mission Board">
            <input
              aria-label="Board slug"
              value={slug}
              onChange={event => setSlug(event.target.value)}
              placeholder="board-slug"
            />
            <input
              aria-label="Board title"
              value={title}
              onChange={event => setTitle(event.target.value)}
              placeholder="Board title"
            />
            <input
              aria-label="Mission goal"
              value={goal}
              onChange={event => setGoal(event.target.value)}
              placeholder="What should this mission achieve?"
            />
            <input
              aria-label="Mission bead (optional)"
              value={beadId}
              onChange={event => setBeadId(event.target.value)}
              placeholder="bead id (optional)"
            />
            <button type="submit" className="oracle-refresh-btn" disabled={creating}>
              {creating ? 'Creating...' : 'Create Mission Board'}
            </button>
          </form>

          {!loading && boards.length === 0 && !error && (
            <div className="oracle-empty">
              No Mission Boards yet. Create one above to start a mission.
            </div>
          )}

          <div className="oracle-agents-grid">
            {boards.map(board => (
              <button
                key={board.slug}
                type="button"
                className="oracle-agent-card mission-board-card"
                data-testid={`mission-board-card-${board.slug}`}
                onClick={() => onOpenBoard(board.slug)}
              >
                <div className="oracle-agent-header">
                  <span className="oracle-agent-name">{board.title || board.slug}</span>
                  <span className="oracle-status-badge oracle-badge-idle">{runLabel(board)}</span>
                </div>
                <div className="mission-board-goal">
                  {board.mission?.goal || 'No mission goal set'}
                </div>
                <div className="oracle-agent-meta">
                  <span>{board.slug}</span>
                  {board.mission?.beadId ? <span className="mission-board-bead">{board.mission.beadId}</span> : null}
                </div>
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
