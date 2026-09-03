/**
 * One Bead, as every view of the tab draws it: its state as a glyph, its type,
 * its id and its title. The whole row is the target: a click puts the Bead on
 * the table and, where the row has children, folds them; the keyboard does the
 * same from the focused row.
 */

import type { KeyboardEvent, ReactNode } from 'react'
import { openBeadCard } from '../../beads/beadCard'
import { beadGlyph, beadStatusLabel } from '../../beads/beadStatus'
import type { WorkRow } from '../../beads/beadsTree'

export interface BeadRowFold {
  /** How many rows open beneath this one. */
  count: number
  expanded: boolean
  setExpanded: (expanded: boolean) => void
}

export interface BeadRowProps {
  row: WorkRow
  /** How deep under an epic the row sits, in the map. */
  depth?: number
  /** Present on a row with children: the chevron and the count at the left. */
  fold?: BeadRowFold
  /** What the view puts at the right of the row, such as an age and a Send. */
  trailing?: ReactNode
}

// Up and down walk the rows of the list the focused row is in, in the order
// they are drawn, which is the order the operator reads them.
function moveFocus(from: HTMLButtonElement, step: 1 | -1): void {
  const scope = from.closest('.beads-content') ?? document.body
  const rows = Array.from(scope.querySelectorAll<HTMLButtonElement>('.bead-row-open'))
  rows[rows.indexOf(from) + step]?.focus()
}

export default function BeadRow({ row, depth = 0, fold, trailing }: BeadRowProps) {
  const glyph = beadGlyph(row.status, row.blocked)
  const state = beadStatusLabel(row.status, row.blocked)

  const select = () => {
    openBeadCard(row.id, row.projectPath)
    fold?.setExpanded(!fold.expanded)
  }

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowLeft':
        if (!fold) return
        event.preventDefault()
        fold.setExpanded(event.key === 'ArrowRight')
        return
      case 'ArrowDown':
      case 'ArrowUp':
        event.preventDefault()
        moveFocus(event.currentTarget, event.key === 'ArrowDown' ? 1 : -1)
        return
      default:
        return
    }
  }

  return (
    <div className="bead-row" data-ui="beads.row" style={{ paddingLeft: `${12 + depth * 22}px` }}>
      <button
        type="button"
        className="bead-row-open"
        aria-expanded={fold ? fold.expanded : undefined}
        onClick={select}
        onKeyDown={onKeyDown}
      >
        {/* The fold slot is drawn on every row so the glyphs line up beneath each other. */}
        <span className="bead-row-fold">
          {fold && (
            <>
              {fold.expanded ? '▾' : '▸'}
              <span className="bead-row-count">{fold.count}</span>
            </>
          )}
        </span>
        <span className="bead-row-glyph" title={state}>{glyph}</span>
        <span className="bead-row-type">{row.type || 'task'}</span>
        <span className="bead-row-id">{row.id}</span>
        <span className="bead-row-title">{row.title}</span>
      </button>
      {trailing}
    </div>
  )
}
