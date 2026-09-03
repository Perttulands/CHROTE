/* The flags catalogue: what this harness's own `--help` said it takes, as rows
   that put a flag on the line or take it off again. The line stays the truth —
   the panel is a way of typing it, never a second place where flags live. */

import { useEffect, useMemo, useRef, useState } from 'react'
import { addFlag, flagNames, flagValue, hasFlag, removeFlag } from './launchFlags'
import type { LaunchFlag } from './launchFlags'
import './FlagPanel.css'

interface FlagPanelProps {
  /** The harness the catalogue belongs to, said the way the launcher says it. */
  harnessLabel: string
  flags: readonly LaunchFlag[]
  /** The flags line as it stands. */
  line: string
  onChange: (line: string) => void
  onClose: () => void
}

export default function FlagPanel({ harnessLabel, flags, line, onChange, onClose }: FlagPanelProps) {
  const [query, setQuery] = useState('')
  // The flag whose value is being typed. Only one row opens at a time, because
  // only one flag is being added at a time.
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const editorRef = useRef<HTMLInputElement | HTMLSelectElement | null>(null)

  useEffect(() => { searchRef.current?.focus() }, [])
  useEffect(() => { if (editing !== null) editorRef.current?.focus() }, [editing])

  const shown = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (needle === '') return flags
    return flags.filter(entry => (
      `${entry.name} ${entry.short ?? ''} ${entry.description}`.toLowerCase().includes(needle)
    ))
  }, [flags, query])

  const commit = (entry: LaunchFlag) => {
    onChange(addFlag(line, entry, draft.trim()))
    setEditing(null)
    setDraft('')
  }

  // One click says the whole intent: a flag that is on comes off, a boolean
  // that is off goes on, and a value flag that is off asks for its value.
  const choose = (entry: LaunchFlag) => {
    if (hasFlag(line, entry)) {
      onChange(removeFlag(line, entry))
      setEditing(null)
      return
    }
    if (entry.value !== undefined) {
      setDraft(entry.values?.[0] ?? '')
      setEditing(entry.name)
      return
    }
    onChange(addFlag(line, entry))
  }

  return (
    <div
      className="flag-panel"
      onKeyDown={event => {
        if (event.key !== 'Escape') return
        event.stopPropagation()
        onClose()
      }}
    >
      <div className="flag-panel-head">
        <span className="flag-panel-title">Flags for {harnessLabel}</span>
        <button type="button" className="flag-panel-close" aria-label="Close flags" onClick={onClose}>
          ✕
        </button>
      </div>

      <input
        ref={searchRef}
        type="text"
        className="flag-panel-search"
        aria-label="Search flags"
        placeholder="Search"
        value={query}
        onChange={event => setQuery(event.target.value)}
      />

      {shown.length === 0 && (
        <div className="flag-panel-empty">
          {flags.length === 0 ? 'No flags read from this harness yet' : 'No flag matches'}
        </div>
      )}

      <ul className="flag-panel-list">
        {shown.map(entry => {
          const selected = hasFlag(line, entry)
          const takesValue = entry.value !== undefined
          const shownValue = selected && takesValue ? flagValue(line, entry) : entry.value
          // Said in one piece, because a name assembled out of adjacent spans
          // is read as one run-on word.
          const spoken = `${flagNames(entry)}${shownValue ? ` ${shownValue}` : ''} ${entry.description}`.trim()
          return (
            <li key={entry.name} className={`flag-row${selected ? ' selected' : ''}`}>
              <button
                type="button"
                className="flag-row-main"
                aria-pressed={selected}
                aria-label={spoken}
                onClick={() => choose(entry)}
              >
                <span className="flag-row-check" aria-hidden="true">{selected ? '✓' : ''}</span>
                <span className="flag-row-text">
                  <span className="flag-row-names">
                    {flagNames(entry)}
                    {shownValue && <span className="flag-row-value"> {shownValue}</span>}
                  </span>
                  <span className="flag-row-description">{entry.description}</span>
                </span>
              </button>

              {editing === entry.name && (
                <div className="flag-row-edit">
                  {entry.values && entry.values.length > 0 ? (
                    <select
                      ref={node => { editorRef.current = node }}
                      className="flag-row-input"
                      aria-label={`Value for ${entry.name}`}
                      value={draft}
                      onChange={event => setDraft(event.target.value)}
                      onKeyDown={event => { if (event.key === 'Enter') commit(entry) }}
                    >
                      {entry.values.map(value => <option key={value} value={value}>{value}</option>)}
                    </select>
                  ) : (
                    <input
                      ref={node => { editorRef.current = node }}
                      type="text"
                      className="flag-row-input"
                      aria-label={`Value for ${entry.name}`}
                      placeholder={entry.value}
                      value={draft}
                      onChange={event => setDraft(event.target.value)}
                      onKeyDown={event => { if (event.key === 'Enter') commit(entry) }}
                    />
                  )}
                  <button type="button" className="flag-row-add" onClick={() => commit(entry)}>Add</button>
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
