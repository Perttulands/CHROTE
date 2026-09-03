/**
 * One menu for every object in CHROTE.
 *
 * A menu is a flat sheet of words: no icons, no radius, no shadow blur. Each
 * row is the action in the operator's words with its chord at the right, so the
 * keyboard is learned at the point of use rather than in a manual. Hairlines
 * separate groups, the highlighted row takes `--surface-secondary` and a 2px
 * `--accent` bar at its left, and the sheet attaches flush to the edge of the
 * control that opened it.
 *
 * A destructive row confirms in place: the first press arms it, the label
 * becomes the confirmation, and a second press within three seconds runs it.
 * Nothing here opens a dialog, and nothing here calls `window.confirm`.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import DismissiblePanel from './DismissiblePanel'
import { CONFIRM_WINDOW_MS } from './confirmInPlace'
import './Menu.css'

export interface MenuAction {
  id: string
  /** The action in words. Never an icon. */
  label: string
  /** The chord that runs the same action, shown at the row's right. */
  chord?: string
  /** Why the action is unavailable, or what it will do; a second, dim line. */
  reason?: string
  danger?: boolean
  disabled?: boolean
  /** Marks the row the object is already in, as a check at the right. */
  checked?: boolean
  /** What the setting this row toggles currently reads, at the row's right. */
  state?: string
  /** Present means confirm in place: this label is what the armed row reads. */
  confirmLabel?: string
  /** Rows this row opens beside itself. */
  submenu?: MenuAction[]
  /** The row turns the menu into an editor rather than dismissing it. */
  keepOpen?: boolean
  onSelect?: () => void
}

/** A row the caller draws itself: an inline input, and nothing more exotic. */
export interface MenuCustomRow {
  id: string
  node: ReactNode
}

export type MenuRow = MenuAction | MenuCustomRow

export interface MenuGroup {
  id: string
  rows: MenuRow[]
}

export interface MenuProps {
  /** The trigger's edge in viewport coordinates; the sheet attaches flush to it. */
  at: { x: number; y: number }
  /** What the menu is for, for assistive technology. */
  label: string
  groups: MenuGroup[]
  onClose: () => void
  zIndex?: number
  /** Estimated size, so the first paint is already inside the viewport. */
  estimatedSize?: { width: number; height: number }
}

function isAction(row: MenuRow): row is MenuAction {
  return (row as MenuAction).label !== undefined
}

function focusableRows(menu: HTMLElement): HTMLButtonElement[] {
  return Array.from(menu.querySelectorAll<HTMLButtonElement>('button.menu-row:not(:disabled)'))
}

interface MenuRowsProps {
  rows: MenuRow[]
  armed: string | null
  onArm: (id: string) => void
  onSelect: (action: MenuAction) => void
  openSubmenu: string | null
  onToggleSubmenu: (id: string) => void
  firstRowRef?: React.RefObject<HTMLButtonElement>
}

function MenuRows({ rows, armed, onArm, onSelect, openSubmenu, onToggleSubmenu, firstRowRef }: MenuRowsProps) {
  return (
    <>
      {rows.map((row, index) => {
        if (!isAction(row)) return <div key={row.id} className="menu-custom-row">{row.node}</div>
        const isArmed = armed === row.id
        const hasSubmenu = row.submenu !== undefined && row.submenu.length > 0
        return (
          <div key={row.id} className={hasSubmenu ? 'menu-submenu-holder' : undefined}>
            <button
              ref={index === 0 ? firstRowRef : undefined}
              type="button"
              role="menuitem"
              className={`menu-row${row.danger || isArmed ? ' menu-row-danger' : ''}`}
              disabled={row.disabled}
              title={row.reason}
              // The chord is printed, not spoken: assistive technology gets it
              // as the shortcut it is rather than as part of the action's name.
              aria-keyshortcuts={row.chord}
              aria-expanded={hasSubmenu ? openSubmenu === row.id : undefined}
              onClick={() => {
                if (hasSubmenu) {
                  onToggleSubmenu(row.id)
                  return
                }
                if (row.confirmLabel !== undefined && !isArmed) {
                  onArm(row.id)
                  return
                }
                onSelect(row)
              }}
            >
              <span className="menu-row-label">
                {isArmed ? row.confirmLabel : row.label}
                {row.reason !== undefined && <span className="menu-row-reason">{row.reason}</span>}
              </span>
              {row.checked === true && <span className="menu-row-check" aria-hidden="true">·</span>}
              {/* A row that opens a submenu still reads back what it is set to. */}
              {row.state !== undefined && (
                <span className="menu-row-chord menu-row-state">{row.state}</span>
              )}
              {hasSubmenu && <span className="menu-row-chord" aria-hidden="true">▸</span>}
              {!hasSubmenu && row.chord !== undefined && (
                <span className="menu-row-chord" aria-hidden="true">{row.chord}</span>
              )}
            </button>
            {hasSubmenu && openSubmenu === row.id && (
              <div className="menu-sheet menu-submenu" role="menu" aria-label={row.label}>
                <MenuRows
                  rows={row.submenu!}
                  armed={armed}
                  onArm={onArm}
                  onSelect={onSelect}
                  openSubmenu={openSubmenu}
                  onToggleSubmenu={onToggleSubmenu}
                />
              </div>
            )}
          </div>
        )
      })}
    </>
  )
}

export default function Menu({ at, label, groups, onClose, zIndex, estimatedSize }: MenuProps) {
  const position = useViewportMenuPosition<HTMLDivElement>(at, {
    estimatedSize: estimatedSize ?? { width: 240, height: 280 },
  })
  const firstRowRef = useRef<HTMLButtonElement>(null)
  const [armed, setArmed] = useState<string | null>(null)
  const [openSubmenu, setOpenSubmenu] = useState<string | null>(null)

  useEffect(() => {
    firstRowRef.current?.focus()
  }, [])

  // An armed row disarms itself: the operator who walked away did not confirm.
  useEffect(() => {
    if (armed === null) return
    const timer = setTimeout(() => setArmed(null), CONFIRM_WINDOW_MS)
    return () => clearTimeout(timer)
  }, [armed])

  const handleSelect = useCallback((action: MenuAction) => {
    setArmed(null)
    if (action.keepOpen !== true) onClose()
    action.onSelect?.()
  }, [onClose])

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = focusableRows(event.currentTarget)
    if (items.length === 0) return
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : event.key === 'ArrowDown'
          ? (current + 1) % items.length
          : (current - 1 + items.length) % items.length
    items[next]?.focus()
  }

  return (
    <DismissiblePanel onDismiss={onClose} panelPosition="fixed" panelZIndex={zIndex}>
      <div
        ref={position.ref}
        className="menu-sheet"
        style={position.style}
        role="menu"
        aria-label={label}
        onClick={event => event.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {groups.map((group, groupIndex) => (
          <div key={group.id} className="menu-group">
            <MenuRows
              rows={group.rows}
              armed={armed}
              onArm={setArmed}
              onSelect={handleSelect}
              openSubmenu={openSubmenu}
              onToggleSubmenu={id => setOpenSubmenu(open => (open === id ? null : id))}
              firstRowRef={groupIndex === 0 ? firstRowRef : undefined}
            />
          </div>
        ))}
      </div>
    </DismissiblePanel>
  )
}
