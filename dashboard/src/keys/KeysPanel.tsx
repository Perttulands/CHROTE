/**
 * Every chord CHROTE knows, in one centred list.
 *
 * It is the registry itself rather than a written manual: a chord that exists
 * is listed, and a chord that is listed runs from here. Typing filters, the
 * arrows move the current row, Enter runs it and Escape closes. There is no
 * backdrop — the panel is content-sized and the workspace behind it stays
 * readable — which is the whole difference between a list and a dialog.
 *
 * The leader opens it and then closes its own window, because from here the
 * next key is search text, not a chord.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import { LEADER_LABEL, chordCaps, directChordLabel, useLeader, type Chord } from './chords'
import { useSurface } from './dismiss'
import './KeysPanel.css'

interface KeysPanelProps {
  isOpen: boolean
  onClose: () => void
}

/** How a chord reads on a row: `ALT + SHIFT + W`, the way a key cap is printed. */
export function chordDisplay(chord: Chord): string {
  if (chord.direct === undefined) return ''
  return chordCaps(chord).map(cap => cap.label.toUpperCase()).join(' + ')
}

export default function KeysPanel({ isOpen, onClose }: KeysPanelProps) {
  const { allChords } = useLeader()
  const [query, setQuery] = useState('')
  const [cursor, setCursor] = useState(0)
  const searchRef = useRef<HTMLInputElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  // A glance: Escape from wherever the cursor is, a press outside, or the
  // chord that opened it closes it.
  useSurface({ open: isOpen, kind: 'glance', onClose, ref: panelRef })

  useEffect(() => {
    if (!isOpen) return
    setQuery('')
    setCursor(0)
    searchRef.current?.focus()
  }, [isOpen])

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase()
    // Both columns are searchable: "alt+s", "alt + s" and "send" find one chord.
    const matches = (chord: Chord) => needle === '' ||
      chord.label.toLowerCase().includes(needle) ||
      chordDisplay(chord).toLowerCase().includes(needle) ||
      (chord.direct !== undefined && directChordLabel(chord.direct).toLowerCase().includes(needle))
    return allChords.filter(matches)
  }, [allChords, query])

  useEffect(() => {
    setCursor(current => (current < rows.length ? current : 0))
  }, [rows.length])

  if (!isOpen) return null

  const run = (chord: Chord) => {
    onClose()
    chord.run()
  }

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (rows.length === 0) return
      const delta = event.key === 'ArrowDown' ? 1 : -1
      setCursor(current => (current + delta + rows.length) % rows.length)
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      const chord = rows[cursor]
      if (chord) run(chord)
    }
  }

  return (
    <div ref={panelRef} className="keys-panel" data-ui="keys.panel" role="dialog" aria-label="Keybindings" onKeyDown={handleKeyDown}>
      <div className="keys-panel-search-row">
        <input
          ref={searchRef}
          className="keys-panel-search"
          type="text"
          value={query}
          placeholder="Keybindings…"
          aria-label="Search keybindings"
          onChange={event => setQuery(event.target.value)}
        />
      </div>
      <div className="keys-panel-body">
        {rows.length === 0 && <p className="keys-panel-empty">No chord matches “{query}”.</p>}
        {rows.map((chord, index) => (
          <button
            key={chord.id}
            type="button"
            className={index === cursor ? 'keys-panel-chord current' : 'keys-panel-chord'}
            onMouseEnter={() => setCursor(index)}
            onClick={() => run(chord)}
          >
            <span className="keys-panel-key">{chordDisplay(chord)}</span>
            <span className="keys-panel-arrow" aria-hidden="true">→</span>
            <span className="keys-panel-label">{chord.label}</span>
          </button>
        ))}
        {query.trim() === '' && (
          <div className="keys-panel-chord keys-panel-leader-row">
            <span className="keys-panel-key">{LEADER_LABEL.toUpperCase().replace(/\+/g, ' + ')}</span>
            <span className="keys-panel-arrow" aria-hidden="true">→</span>
            <span className="keys-panel-label">Keybindings</span>
          </div>
        )}
      </div>
    </div>
  )
}
