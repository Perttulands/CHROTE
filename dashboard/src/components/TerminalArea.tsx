import { useState, useEffect, useRef } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useDndContext, useDroppable } from '@dnd-kit/core'
import { useSession } from '../context/SessionContext'
import TerminalWindow from './TerminalWindow'
import { useMediaQuery } from '../hooks/useMediaQuery'
import type { WorkspaceId } from '../types'

/** The layout never grows past the four windows the grid classes describe. */
const MAX_WINDOWS = 4

/**
 * How far either side of the 4px grid gap still counts as the gap. A seam is
 * easy to see and hard to hit, so the drop target is wider than the line the
 * operator aims at, and still far narrower than the tiles beside it.
 */
const GAP_HIT_PAD = 12

interface GapRect {
  index: number
  style: CSSProperties
}

/**
 * The seam between two tiles, live only while a session is in the air. Dropping
 * here adds a window to the layout and binds the dragged session to it, which is
 * the drag equivalent of Alt+= followed by a drop. The zone takes no pointer
 * events: dnd-kit collides against its rectangle, and the terminal underneath
 * keeps every click it would otherwise have had.
 */
function WindowGapDropZone({ workspaceId, index, style }: { workspaceId: WorkspaceId } & GapRect) {
  const { setNodeRef, isOver } = useDroppable({
    id: `gap-${workspaceId}-${index}`,
    data: { type: 'window-gap', workspaceId },
  })

  return (
    <div
      ref={setNodeRef}
      className={`terminal-window-gap ${isOver ? 'over' : ''}`}
      style={style}
      data-window-gap={index}
      aria-hidden="true"
    />
  )
}

interface TerminalAreaProps {
  workspaceId: WorkspaceId
  sidecarControls?: ReactNode
  onOpenFilesAtPath?: (path: string) => void
  workspaceActive?: boolean
}

function TerminalArea({ workspaceId, sidecarControls, onOpenFilesAtPath, workspaceActive = true }: TerminalAreaProps) {
  const { workspaces, windowRevealRequest } = useSession()
  const workspace = workspaces[workspaceId]
  const windows = workspace.windows
  const windowCount = workspace.windowCount

  const isMobile = useMediaQuery('(max-width: 768px)')
  const [mobileActiveIndex, setMobileActiveIndex] = useState(0)
  const lastConsumedRevealRequestId = useRef(0)
  const gridRef = useRef<HTMLDivElement>(null)
  const [gaps, setGaps] = useState<GapRect[]>([])
  const visibleWindows = windows.slice(0, windowCount)

  // A tag or a row in the air is the only reason a seam exists.
  const { active } = useDndContext()
  const activeDragType = (active?.data.current as { type?: string } | undefined)?.type
  const gapsOffered = (activeDragType === 'session' || activeDragType === 'tag')
    && !isMobile
    && workspaceActive
    && windowCount < MAX_WINDOWS

  // Ensure valid mobile index when configuration changes
  useEffect(() => {
    if (mobileActiveIndex >= windowCount) {
      setMobileActiveIndex(0)
    }
  }, [windowCount, mobileActiveIndex])

  // A reveal first expands windowCount in SessionContext. Consume the matching
  // request only once that canonical slot is actually part of this area's
  // visible slice, so mobile navigation lands on the revealed window.
  useEffect(() => {
    if (!windowRevealRequest || windowRevealRequest.workspaceId !== workspaceId) return
    if (windowRevealRequest.requestId <= lastConsumedRevealRequestId.current) return

    const targetIndex = windows.findIndex(window => window.id === windowRevealRequest.windowId)
    if (targetIndex < 0 || targetIndex >= windowCount) return

    lastConsumedRevealRequestId.current = windowRevealRequest.requestId
    setMobileActiveIndex(targetIndex)
  }, [windowCount, windowRevealRequest, windows, workspaceId])

  // The seams are measured from the tiles themselves, once, when the drag
  // starts: the grid decides where they fall, and every layout that can still
  // grow lays its tiles in one row.
  useEffect(() => {
    if (!gapsOffered) {
      setGaps(current => (current.length === 0 ? current : []))
      return
    }
    const grid = gridRef.current
    if (!grid) return

    const gridRect = grid.getBoundingClientRect()
    const tiles = Array.from(grid.querySelectorAll<HTMLElement>(':scope > .terminal-window'))
      .map(tile => tile.getBoundingClientRect())
    const measured: GapRect[] = []
    for (let index = 0; index < tiles.length - 1; index += 1) {
      const before = tiles[index]
      const after = tiles[index + 1]
      // Only a seam within one row is a seam the operator can aim at.
      if (Math.abs(before.top - after.top) > 1 || after.left <= before.right) continue
      measured.push({
        index,
        style: {
          left: before.right - gridRect.left - GAP_HIT_PAD,
          top: before.top - gridRect.top,
          width: after.left - before.right + GAP_HIT_PAD * 2,
          height: before.height,
        },
      })
    }
    setGaps(measured)
  }, [gapsOffered, windowCount])

  // Get grid class based on window count
  const getGridClass = () => {
    if (isMobile) return 'grid-1'

    switch (windowCount) {
      case 1: return 'grid-1'
      case 2: return 'grid-2'
      case 3: return 'grid-3'
      case 4: return 'grid-4'
      default: return 'grid-2'
    }
  }

  return (
    <div className="terminal-area">
      <div
        className="terminal-area-controls"
        data-ui="workspace.strip"
        aria-label="Terminal workspace controls"
      >
        {sidecarControls}
        {sidecarControls && <span className="terminal-controls-divider" aria-hidden="true" />}
        {isMobile ? (
          // A phone shows one window at a time, so the carousel keeps its
          // pager. There is no keyboard behind it to reach the same window.
          <div className="mobile-controls-row" role="group" aria-label="Window view controls">
            <span className="layout-label">View</span>
            {Array.from({ length: windowCount }).map((_, idx) => (
              <button
                key={`view-${idx}`}
                className={`layout-btn ${mobileActiveIndex === idx ? 'active' : ''}`}
                onClick={() => setMobileActiveIndex(idx)}
                aria-label={`View window ${idx + 1}`}
              >
                {idx + 1}
              </button>
            ))}
          </div>
        ) : (
          // The layout is a fact, not a control: it says what it is and names
          // the two chords that change it.
          <div className="layout-state">
            <span className="layout-label">Layout</span>
            <span className="layout-count">{windowCount}</span>
            <span className="layout-chords">Alt+= add window · Alt+- remove empty</span>
          </div>
        )}
      </div>

      <div ref={gridRef} className={`terminal-grid ${getGridClass()}`} data-workspace={workspaceId}>
        {visibleWindows.map((window, index) => {
          const isVisible = !isMobile || index === mobileActiveIndex
          return (
            <TerminalWindow
              key={window.id}
              workspaceId={workspaceId}
              window={window}
              style={{ display: isVisible ? 'flex' : 'none' }}
              onOpenFilesAtPath={onOpenFilesAtPath}
              workspaceActive={workspaceActive}
            />
          )
        })}
        {gapsOffered && gaps.map(gap => (
          <WindowGapDropZone key={`gap-${gap.index}`} workspaceId={workspaceId} {...gap} />
        ))}
      </div>
    </div>
  )
}

export default TerminalArea
