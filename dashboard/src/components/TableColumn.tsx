/**
 * The table's column: where a tab shows the one selected object.
 *
 * It is a flex sibling at the right of the tab's content, never a layer over
 * it: the grid, the map or the stack narrows to make room, and the column
 * stays where it is until the operator closes it. A 4px handle at its left
 * edge resizes it, the width is remembered per device, and when the tab runs
 * out of room the content keeps 480px first, then the column shrinks to its
 * minimum. Below 900px there is no column to give, so it overlays the tab.
 *
 * It is a work surface for the dismissal owner: Escape closes it while it is
 * the topmost thing open (the Send drawer or Peek above it take Escape first),
 * a click outside leaves it, and Alt+I closes it from anywhere.
 */

import { useCallback, useRef } from 'react'
import type { CSSProperties } from 'react'
import BeadCard from './BeadCard'
import AgentContextSheet from './AgentContextSheet'
import FilePanelViewer from './FilePanelViewer'
import { useSession } from '../context/SessionContext'
import {
  TABLE_CONTENT_MIN,
  TABLE_WIDTH_MIN,
  clampTableWidth,
  clearTable,
  dismissTable,
  putOnTable,
  tableLabel,
  useTableActions,
  useTableObject,
} from '../context/TableContext'
import { useSurface } from '../keys/dismiss'
import { useResizableWidth } from '../hooks/useResizableWidth'
import './TableColumn.css'

export default function TableColumn() {
  const object = useTableObject()
  const session = useSession()
  const { openInBeads } = useTableActions()
  const columnRef = useRef<HTMLElement>(null)

  const open = object !== null

  useSurface({ open, kind: 'work', onClose: dismissTable, ref: columnRef })

  const { settings, updateSettings, openSendToSession } = session
  const remembered = clampTableWidth(settings?.tableWidth)

  /** The widest the column may be here: the content keeps its 480px. */
  const widest = useCallback(() => {
    const room = columnRef.current?.parentElement?.clientWidth || Number.POSITIVE_INFINITY
    return Math.max(TABLE_WIDTH_MIN, room - TABLE_CONTENT_MIN)
  }, [])

  const commitWidth = useCallback((tableWidth: number) => {
    updateSettings({ tableWidth })
  }, [updateSettings])

  const resize = useResizableWidth({
    elementRef: columnRef,
    width: remembered,
    minWidth: TABLE_WIDTH_MIN,
    maxWidth: widest,
    edge: 'left',
    onCommit: commitWidth,
  })

  if (object === null) return null

  const width = resize.width
  const style = { '--table-width': `${width}px` } as CSSProperties

  return (
    <aside
      ref={columnRef}
      className="table-column"
      role="complementary"
      aria-label={tableLabel(object)}
      data-ui="table.column"
      tabIndex={-1}
      style={style}
    >
      <div
        {...resize.handleProps}
        className={`table-column-handle${resize.resizing ? ' dragging' : ''}`}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the table"
        aria-valuenow={width}
        aria-valuemin={TABLE_WIDTH_MIN}
        tabIndex={0}
      />
      {object.kind === 'bead' && <BeadCard onOpenInBeads={openInBeads} />}
      {object.kind === 'agent-context' && <AgentContextSheet />}
      {object.kind === 'file' && (
        <FilePanelViewer
          path={object.path}
          onBack={clearTable}
          onOpenPath={path => putOnTable({ kind: 'file', path })}
          onSend={path => openSendToSession({ reference: `path ${path}` })}
        />
      )}
    </aside>
  )
}
