/**
 * Every chord CHROTE knows, searchable, grouped by the scope that offers it.
 * It replaces the static shortcuts overlay: this list is the registry itself,
 * so a chord that exists is listed and a chord that is listed can be run from
 * here by clicking it. The Alt chord is the first column and the bare leader
 * key the second; the search reads both.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import { LEADER_LABEL, SCOPE_TITLES, directChordLabel, useLeader, type Chord, type ChordScope } from './chords'
import './KeysPanel.css'

const SCOPE_ORDER: ChordScope[] = ['global', 'workspace', 'tile']

interface KeysPanelProps {
  isOpen: boolean
  onClose: () => void
}

export default function KeysPanel({ isOpen, onClose }: KeysPanelProps) {
  const { allChords } = useLeader()
  const [query, setQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!isOpen) return
    setQuery('')
    searchRef.current?.focus()
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      onClose()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [isOpen, onClose])

  const groups = useMemo(() => {
    const needle = query.trim().toLowerCase()
    // Both columns are searchable: "alt+s" and "s" find the same chord.
    const matches = (chord: Chord) => needle === '' ||
      chord.label.toLowerCase().includes(needle) ||
      chord.key.toLowerCase().includes(needle) ||
      (chord.direct !== undefined && directChordLabel(chord.direct).toLowerCase().includes(needle))
    return SCOPE_ORDER
      .map(scope => ({ scope, chords: allChords.filter(chord => chord.scope === scope && matches(chord)) }))
      .filter(group => group.chords.length > 0)
  }, [allChords, query])

  if (!isOpen) return null

  const run = (chord: Chord) => {
    onClose()
    chord.run()
  }

  return (
    <div className="keys-panel-backdrop" onClick={event => { if (event.target === event.currentTarget) onClose() }}>
      <div className="keys-panel" role="dialog" aria-modal="true" aria-label="Keys">
        <div className="keys-panel-header">
          <span className="keys-panel-title">Keys</span>
          <span className="keys-panel-leader">Alt and a key, or {LEADER_LABEL} then the bare key</span>
          <button type="button" className="keys-panel-close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <input
          ref={searchRef}
          className="keys-panel-search"
          type="text"
          value={query}
          placeholder="Search chords"
          aria-label="Search chords"
          onChange={event => setQuery(event.target.value)}
        />
        <div className="keys-panel-body">
          {groups.length === 0 && <p className="keys-panel-empty">No chord matches “{query}”.</p>}
          {groups.map(group => (
            <section key={group.scope} className="keys-panel-group">
              <h3 className="keys-panel-group-title">{SCOPE_TITLES[group.scope]}</h3>
              {group.chords.map(chord => (
                <button
                  key={chord.id}
                  type="button"
                  className="keys-panel-chord"
                  onClick={() => run(chord)}
                >
                  <span className="keys-panel-key">
                    {chord.direct ? directChordLabel(chord.direct) : chord.key}
                  </span>
                  <span className="keys-panel-leader-key">{chord.direct ? chord.key : ''}</span>
                  <span className="keys-panel-label">{chord.label}</span>
                </button>
              ))}
            </section>
          ))}
        </div>
      </div>
    </div>
  )
}
