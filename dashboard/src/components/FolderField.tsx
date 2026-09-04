/* The Folder field: one input that takes a path three ways. Empty, the list
   beneath it offers the recent folders; a fragment ranks the host's
   workspaces by fuzzy match on the tail of their paths; an absolute path lists
   the directories under the typed prefix. Tab completes the highlighted row,
   the arrows move, Enter takes the highlighted row or the typed path. The list
   keeps the same extent whatever it holds, so nothing moves under the pointer
   while suggestions change. */

import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { fetchDirectory } from './FilesView/fileService'
import { fetchWorkspaces, type Workspace } from '../workspaces/workspacesApi'
import { lastSegments, rankByFuzzy } from '../utils/fuzzy'
import './FolderField.css'

/** Rows the list shows at once. It is this tall whatever it holds. */
export const FOLDER_FIELD_ROWS = 6

export interface FolderFieldProps {
  id?: string
  value: string
  onChange: (value: string) => void
  /** Enter: the highlighted suggestion, or the typed path. */
  onSubmit?: (value: string) => void
  /** What the list offers before anything is typed. */
  recents?: readonly string[]
  placeholder?: string
  ariaLabel: string
  inputClassName?: string
}

type Mode = 'recent' | 'workspaces' | 'folders'

interface Suggestion {
  path: string
  /** What Tab writes into the field: a folder gets its slash, so the next Tab lists inside it. */
  complete: string
}

interface Listing {
  dir: string
  names: string[]
  failed: boolean
}

const MODE_LABEL: Record<Mode, string> = {
  recent: 'Recent',
  workspaces: 'Workspaces',
  folders: 'Folders',
}

function splitTypedPath(value: string): { dir: string; segment: string } {
  const cut = value.lastIndexOf('/') + 1
  return { dir: value.slice(0, cut), segment: value.slice(cut) }
}

export default function FolderField({
  id,
  value,
  onChange,
  onSubmit,
  recents = [],
  placeholder,
  ariaLabel,
  inputClassName,
}: FolderFieldProps) {
  // Typing is what turns the list from recents into suggestions for what is
  // typed; a folder picked, submitted, or left alone is not being typed.
  const [editing, setEditing] = useState(false)
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [listing, setListing] = useState<Listing | null>(null)
  // The highlight belongs to the value it was moved on; a new value gets the
  // mode's default, which is the top match for a fragment and nothing for a
  // path, so Enter on a typed folder means that folder.
  const [moved, setMoved] = useState<{ forValue: string; index: number } | null>(null)
  const rowsRef = useRef<HTMLDivElement>(null)

  const mode: Mode = !editing || value === '' ? 'recent' : value.startsWith('/') ? 'folders' : 'workspaces'
  const typed = mode === 'folders' ? splitTypedPath(value) : null

  useEffect(() => {
    let current = true
    fetchWorkspaces()
      .then(found => { if (current) setWorkspaces(found) })
      .catch(() => { /* a host that cannot list workspaces still takes a typed path */ })
    return () => { current = false }
  }, [])

  const dir = typed?.dir ?? ''
  useEffect(() => {
    if (mode !== 'folders' || dir === '' || listing?.dir === dir) return
    let current = true
    fetchDirectory(dir)
      .then(items => {
        if (!current) return
        setListing({ dir, names: items.filter(item => item.isDir).map(item => item.name), failed: false })
      })
      .catch(() => { if (current) setListing({ dir, names: [], failed: true }) })
    return () => { current = false }
  }, [mode, dir, listing?.dir])

  const suggestions = useMemo<Suggestion[]>(() => {
    if (mode === 'recent') {
      const offered = recents.length > 0 ? recents : workspaces.map(workspace => workspace.path)
      return offered.map(path => ({ path, complete: path }))
    }
    if (mode === 'workspaces') {
      return rankByFuzzy(value, workspaces, workspace => lastSegments(workspace.path))
        .map(workspace => ({ path: workspace.path, complete: workspace.path }))
    }
    if (!typed || listing?.dir !== typed.dir) return []
    const wanted = typed.segment.toLowerCase()
    const names = listing.names.filter(name => name.toLowerCase().startsWith(wanted))
    const plain = names.filter(name => !name.startsWith('.'))
    const dotted = names.filter(name => name.startsWith('.'))
    return [...plain, ...dotted].map(name => ({ path: typed.dir + name, complete: `${typed.dir}${name}/` }))
  }, [listing, mode, recents, typed, value, workspaces])

  const defaultHighlight = mode === 'workspaces' && suggestions.length > 0 ? 0 : -1
  const highlight = moved?.forValue === value ? Math.min(moved.index, suggestions.length - 1) : defaultHighlight

  useEffect(() => {
    if (highlight < 0) return
    const row = rowsRef.current?.children[highlight] as HTMLElement | undefined
    row?.scrollIntoView?.({ block: 'nearest' })
  }, [highlight])

  const pick = (path: string) => {
    onChange(path)
    setEditing(false)
  }

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (suggestions.length === 0) return
      const floor = mode === 'workspaces' ? 0 : -1
      const next = event.key === 'ArrowDown'
        ? Math.min(highlight + 1, suggestions.length - 1)
        : Math.max(highlight - 1, floor)
      setMoved({ forValue: value, index: next })
      return
    }
    if (event.key === 'Tab' && !event.shiftKey) {
      if (suggestions.length === 0) return
      event.preventDefault()
      const target = suggestions[highlight >= 0 ? highlight : 0]
      onChange(target.complete)
      setEditing(true)
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      const chosen = highlight >= 0 ? suggestions[highlight].path : value.trim()
      if (chosen === '') return
      pick(chosen)
      onSubmit?.(chosen)
      return
    }
    if (event.key === 'Escape' && editing) {
      event.preventDefault()
      event.stopPropagation()
      setEditing(false)
    }
  }

  const note = mode === 'recent'
    ? 'Nothing recent'
    : mode === 'workspaces'
      ? 'No workspace matches'
      : listing?.dir !== typed?.dir
        ? 'Reading…'
        : listing?.failed
          ? 'Cannot list this folder'
          : 'No folders here'

  return (
    <div className="folder-field">
      <input
        id={id}
        type="text"
        className={`folder-field-input${inputClassName ? ` ${inputClassName}` : ''}`}
        aria-label={ariaLabel}
        autoComplete="off"
        spellCheck={false}
        placeholder={placeholder}
        value={value}
        onChange={event => { onChange(event.target.value); setEditing(true) }}
        onKeyDown={onKeyDown}
        onBlur={() => setEditing(false)}
      />
      <div className="folder-field-list" style={{ '--folder-field-rows': FOLDER_FIELD_ROWS } as React.CSSProperties}>
        <span className="folder-field-label">{mode === 'recent' && recents.length === 0 ? MODE_LABEL.workspaces : MODE_LABEL[mode]}</span>
        <div className="folder-field-rows" role="listbox" aria-label={`${ariaLabel} suggestions`} ref={rowsRef}>
          {suggestions.length === 0 && <span className="folder-field-note">{note}</span>}
          {suggestions.map((suggestion, index) => (
            <div
              key={suggestion.path}
              role="option"
              aria-selected={index === highlight}
              className={`folder-field-row${index === highlight ? ' highlighted' : ''}${suggestion.path === value ? ' selected' : ''}`}
              title={suggestion.path}
              // A press keeps the field focused, so the list is still this
              // list when the click lands.
              onMouseDown={event => event.preventDefault()}
              onClick={() => pick(suggestion.path)}
            >
              {suggestion.path}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
