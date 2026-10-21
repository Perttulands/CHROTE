/**
 * The Beads table: one row per Bead in the slice, sortable by every column.
 *
 * Rows are virtual: only the rows in view exist, so five thousand Beads cost
 * what forty do. A click selects a row and the graph follows it; Enter or a
 * double-click puts the Bead on the table. The arrow keys move the selection.
 * Numeric columns carry a faint bar against the largest value on the host, so
 * a bar keeps its length whatever the filter does.
 */

import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import MenuTarget from '../MenuTarget'
import type { MenuGroup } from '../Menu'
import { beadGlyph, formatBeadTime } from '../../beads/beadStatus'
import { beadReference } from '../../beads/beadReference'
import {
  NATURAL_DIRECTION,
  SLICE_STATUSES,
  STATUS_LABELS,
  dayLabel,
  formatSize,
  type SliceIndex,
  type SliceSortKey,
  type SortDirection,
} from '../../beads/beadsSlice'

export const ROW_HEIGHT = 36
const OVERSCAN = 8

const COLUMNS: { key: SliceSortKey; label: string; numeric?: boolean; title: string }[] = [
  { key: 'id', label: 'ID', title: 'Bead id' },
  { key: 'title', label: 'Title', title: 'Title' },
  { key: 'project', label: 'Project', title: 'The store the Bead lives in' },
  { key: 'status', label: 'Status', title: 'Status, blocked by any unfinished blocker in any store' },
  { key: 'updated', label: 'Updated', numeric: true, title: 'Last update' },
  { key: 'idle', label: 'Idle', numeric: true, title: 'Days since the last update' },
  { key: 'size', label: 'Size', numeric: true, title: 'Characters of description, design, acceptance and notes' },
  { key: 'kids', label: 'Kids', numeric: true, title: 'Direct children, in any store' },
]

interface BeadTableProps {
  index: SliceIndex
  ordered: Int32Array
  selected: number
  sort: SliceSortKey
  dir: SortDirection
  /** Rises when the selection moved by something other than a click here. */
  reveal: number
  onSort: (key: SliceSortKey, dir: SortDirection) => void
  onSelect: (row: number) => void
  onOpen: (row: number) => void
  onSend: (reference: string) => void
  onCopy: (text: string, what: string) => void
}

function Bar({ fraction }: { fraction: number }) {
  return (
    <span className="beads-cell-bar" aria-hidden="true">
      <span style={{ width: `${Math.round(Math.max(0, Math.min(1, fraction)) * 100)}%` }} />
    </span>
  )
}

function BeadTable({ index, ordered, selected, sort, dir, reveal, onSort, onSelect, onOpen, onSend, onCopy }: BeadTableProps) {
  const body = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewport, setViewport] = useState(600)
  const position = useMemo(() => {
    if (selected < 0) return -1
    for (let k = 0; k < ordered.length; k += 1) if (ordered[k] === selected) return k
    return -1
  }, [ordered, selected])

  useLayoutEffect(() => {
    const element = body.current
    if (!element) return
    const read = () => setViewport(element.clientHeight || 600)
    read()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(read)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const scrollIntoView = useCallback((at: number) => {
    const element = body.current
    if (!element || at < 0) return
    const top = at * ROW_HEIGHT
    if (top < element.scrollTop) element.scrollTop = top
    else if (top + ROW_HEIGHT > element.scrollTop + element.clientHeight) element.scrollTop = top + ROW_HEIGHT - element.clientHeight
  }, [])

  // A selection made elsewhere (the graph, a link, a reveal) is brought into view.
  useEffect(() => {
    if (reveal > 0) scrollIntoView(position)
    // Only a new reveal request scrolls; a re-sort keeps the operator's place.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reveal])

  const move = (to: number) => {
    if (ordered.length === 0) return
    const at = Math.max(0, Math.min(ordered.length - 1, to))
    onSelect(ordered[at])
    scrollIntoView(at)
  }

  const onKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    const page = Math.max(1, Math.floor(viewport / ROW_HEIGHT) - 1)
    switch (event.key) {
      case 'ArrowDown': move(position < 0 ? 0 : position + 1); break
      case 'ArrowUp': move(position < 0 ? 0 : position - 1); break
      case 'PageDown': move(position + page); break
      case 'PageUp': move(position - page); break
      case 'Home': move(0); break
      case 'End': move(ordered.length - 1); break
      case 'Enter': if (selected >= 0) onOpen(selected); break
      default: return
    }
    event.preventDefault()
  }

  const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN)
  const last = Math.min(ordered.length, Math.ceil((scrollTop + viewport) / ROW_HEIGHT) + OVERSCAN)
  const logSize = Math.log1p(index.maxSize)
  const logKids = Math.log1p(index.maxKids)

  const menu = (row: number): MenuGroup[] => {
    const bead = index.rows[row]
    return [
      {
        id: 'work',
        rows: [
          { id: 'open', label: 'Open', chord: 'Enter', onSelect: () => onOpen(row) },
          { id: 'send', label: 'Send', chord: 'Alt+S', onSelect: () => onSend(beadReference(bead)) },
        ],
      },
      {
        id: 'copy',
        rows: [
          { id: 'copy-id', label: 'Copy id', onSelect: () => onCopy(bead.id, bead.id) },
          { id: 'copy-id-title', label: 'Copy id and title', onSelect: () => onCopy(`${bead.id}: ${bead.title}`, `${bead.id} and its title`) },
        ],
      },
    ]
  }

  const rows = []
  for (let k = first; k < last; k += 1) {
    const row = ordered[k]
    const bead = index.rows[row]
    const status = SLICE_STATUSES[index.status[row]]
    const closed = status === 'closed'
    const idle = index.idle[row]
    const kids = index.kids[row]
    const size = bead.size ?? 0
    const project = index.projects[index.project[row]]
    rows.push(
      <MenuTarget key={row} label={`Actions for ${bead.id}`} groups={() => menu(row)}>
        <div
          className={[
            'beads-row',
            row === selected ? 'selected' : '',
            closed ? 'closed' : '',
            bead.type === 'epic' ? 'epic' : '',
          ].filter(Boolean).join(' ')}
          role="row"
          aria-rowindex={k + 2}
          aria-selected={row === selected}
          data-ui="beads.row"
          style={{ transform: `translateY(${k * ROW_HEIGHT}px)` }}
          onClick={() => { onSelect(row); body.current?.focus({ preventScroll: true }) }}
          onDoubleClick={() => onOpen(row)}
        >
          <span className="beads-cell beads-cell-id" role="gridcell">{bead.id}</span>
          <span className="beads-cell beads-cell-title" role="gridcell" title={bead.title}>{bead.title}</span>
          <span className="beads-cell beads-cell-project" role="gridcell" title={project.path}>{project.label}</span>
          <span className="beads-cell beads-cell-status" role="gridcell" title={bead.status.replace(/_/g, ' ')}>
            <span className="beads-glyph" aria-hidden="true">{beadGlyph(closed ? 'closed' : bead.status, status === 'blocked')}</span>
            {STATUS_LABELS[status]}
          </span>
          <span className="beads-cell beads-cell-number" role="gridcell" title={formatBeadTime(bead.updated ? new Date(bead.updated * 1000).toISOString() : undefined)}>
            {index.updatedDay[row] >= 0 ? dayLabel(index.updatedDay[row]) : ''}
          </span>
          <span className="beads-cell beads-cell-number" role="gridcell">
            {idle >= 0 ? `${idle}d` : ''}
            <Bar fraction={index.maxIdle > 0 && idle > 0 ? idle / index.maxIdle : 0} />
          </span>
          <span className="beads-cell beads-cell-number" role="gridcell" title={`${size} characters`}>
            {formatSize(size)}
            <Bar fraction={logSize > 0 ? Math.log1p(size) / logSize : 0} />
          </span>
          <span className="beads-cell beads-cell-number" role="gridcell">
            {kids}
            <Bar fraction={logKids > 0 ? Math.log1p(kids) / logKids : 0} />
          </span>
        </div>
      </MenuTarget>,
    )
  }

  return (
    <div className="beads-table" role="grid" aria-label="Beads" aria-rowcount={ordered.length + 1} data-ui="beads.table">
      <div className="beads-row beads-head" role="row" aria-rowindex={1}>
        {COLUMNS.map(column => {
          const active = sort === column.key
          return (
            <span
              key={column.key}
              role="columnheader"
              className={`beads-cell${column.numeric ? ' beads-cell-number' : ''}`}
              aria-sort={active ? (dir === 'asc' ? 'ascending' : 'descending') : 'none'}
            >
              <button
                type="button"
                className={`beads-sort${active ? ' active' : ''}`}
                title={column.title}
                onClick={() => onSort(column.key, active ? (dir === 'asc' ? 'desc' : 'asc') : NATURAL_DIRECTION[column.key])}
              >
                {column.label}
                <span className="beads-sort-arrow" aria-hidden="true">{active ? (dir === 'asc' ? '↑' : '↓') : ''}</span>
              </button>
            </span>
          )
        })}
      </div>
      <div
        ref={body}
        className="beads-table-body"
        role="rowgroup"
        tabIndex={0}
        onScroll={event => setScrollTop(event.currentTarget.scrollTop)}
        onKeyDown={onKeyDown}
      >
        <div className="beads-table-space" role="presentation" style={{ height: ordered.length * ROW_HEIGHT }}>
          {rows}
        </div>
      </div>
    </div>
  )
}

export default memo(BeadTable)
