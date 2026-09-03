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
 * Escape closes it while it is the topmost surface: with the Send drawer or
 * Peek open, Escape is theirs. Alt+I closes it from anywhere.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react'
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
import './TableColumn.css'

/** What one arrow key on the handle is worth. */
const KEYBOARD_STEP = 16

/** Surfaces whose focus means Escape is theirs, until the dismissal owner lands. */
const ESCAPE_OWNERS = '[role="menu"], [role="dialog"], .keys-panel'

export default function TableColumn() {
  const object = useTableObject()
  const session = useSession()
  const { openInBeads } = useTableActions()
  const columnRef = useRef<HTMLElement>(null)
  const dragRef = useRef<number | null>(null)
  const [dragWidth, setDragWidth] = useState<number | null>(null)

  const open = object !== null
  const drawerOpen = Boolean(session.sendToSessionRequest)
  const peekOpen = Boolean(session.floatingSession)

  useEffect(() => {
    if (!open || drawerOpen || peekOpen) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      const target = event.target instanceof HTMLElement ? event.target : null
      if (target?.closest(ESCAPE_OWNERS)) return
      event.preventDefault()
      dismissTable()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, drawerOpen, peekOpen])

  const { settings, updateSettings, openSendToSession } = session
  const remembered = clampTableWidth(settings?.tableWidth)

  /** The widest the column may be here: the content keeps its 480px. */
  const widest = useCallback(() => {
    const room = columnRef.current?.parentElement?.clientWidth || Number.POSITIVE_INFINITY
    return Math.max(TABLE_WIDTH_MIN, room - TABLE_CONTENT_MIN)
  }, [])

  const startDrag = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const column = columnRef.current
    if (event.button !== 0 || !column) return
    event.preventDefault()
    const handle = event.currentTarget
    // The edge follows the pointer from where it was grabbed, so the handle
    // does not jump under the finger by however far into it the press landed.
    const grabbedAt = event.clientX
    const grabbedWidth = column.getBoundingClientRect().width
    const max = widest()
    handle.setPointerCapture(event.pointerId)
    const move = (moveEvent: PointerEvent) => {
      const next = Math.min(max, Math.max(TABLE_WIDTH_MIN, Math.round(grabbedWidth + grabbedAt - moveEvent.clientX)))
      dragRef.current = next
      setDragWidth(next)
    }
    const end = () => {
      handle.removeEventListener('pointermove', move)
      handle.removeEventListener('pointerup', end)
      handle.removeEventListener('pointercancel', end)
      const dragged = dragRef.current
      dragRef.current = null
      setDragWidth(null)
      if (dragged !== null) updateSettings({ tableWidth: dragged })
    }
    handle.addEventListener('pointermove', move)
    handle.addEventListener('pointerup', end)
    handle.addEventListener('pointercancel', end)
  }, [updateSettings, widest])

  // The handle is at the left edge, so left is wider and right is narrower.
  const resizeByKey = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const delta = event.key === 'ArrowLeft' ? KEYBOARD_STEP : event.key === 'ArrowRight' ? -KEYBOARD_STEP : 0
    if (delta === 0) return
    event.preventDefault()
    updateSettings({ tableWidth: Math.min(widest(), Math.max(TABLE_WIDTH_MIN, remembered + delta)) })
  }, [remembered, updateSettings, widest])

  if (object === null) return null

  const width = dragWidth ?? remembered
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
        className={`table-column-handle${dragWidth !== null ? ' dragging' : ''}`}
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the table"
        aria-valuenow={width}
        aria-valuemin={TABLE_WIDTH_MIN}
        tabIndex={0}
        onPointerDown={startDrag}
        onKeyDown={resizeByKey}
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
