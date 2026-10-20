import { useEffect, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import MenuTarget from '../MenuTarget'
import { copyBrainView, emptyBrainViews, readBrainViews, writeBrainViews } from './brainViewState'
import type { BrainViewMode, BrainViewSlots, BrainViewSnapshot } from './brainViewState'
import './BrainViewControls.css'

export interface BrainViewControlsProps {
  snapshot: BrainViewSnapshot
  onRestore: (snapshot: BrainViewSnapshot) => void
  onMode: (mode: BrainViewMode) => void
  onFit: () => void
  timeControl: ReactNode
}

export default function BrainViewControls({ snapshot, onRestore, onMode, onFit, timeControl }: BrainViewControlsProps) {
  const [initial] = useState(() => {
    try { return { slots: readBrainViews(window.localStorage), error: '' } }
    catch (cause) { return { slots: emptyBrainViews(), error: cause instanceof Error ? cause.message : 'Saved Brain views could not be read.' } }
  })
  const [slots, setSlots] = useState(initial.slots)
  const [error, setError] = useState(initial.error)
  const [renaming, setRenaming] = useState<number | null>(null)
  const [name, setName] = useState('')
  const renameInput = useRef<HTMLInputElement>(null)
  const slotButtons = useRef<(HTMLButtonElement | null)[]>([])

  useEffect(() => {
    if (renaming === null) return
    renameInput.current?.focus()
    renameInput.current?.select()
  }, [renaming])

  const save = (next: BrainViewSlots) => {
    try {
      writeBrainViews(window.localStorage, next)
      setSlots(next)
      setError('')
      return true
    } catch (cause) {
      setError(`Brain views were not saved: ${cause instanceof Error ? cause.message : 'browser storage is unavailable.'}`)
      return false
    }
  }
  const replace = (index: number) => save(slots.map((slot, at) => at === index
    ? { name: slot?.name ?? `View ${index + 1}`, snapshot: copyBrainView(snapshot) } : slot))
  const closeRename = () => {
    const index = renaming
    setRenaming(null)
    if (index !== null) slotButtons.current[index]?.focus()
  }
  const rename = (event: FormEvent) => {
    event.preventDefault()
    if (renaming === null || !name.trim()) return
    if (save(slots.map((slot, index) => index === renaming && slot ? { ...slot, name: name.trim() } : slot))) closeRename()
  }

  return <footer className="brain-view-controls" aria-label="Brain view controls">
    <div className="brain-view-modes" role="group" aria-label="Brain view mode">
      {(['subjects', 'map'] as const).map(mode => <button type="button" key={mode}
        aria-pressed={snapshot.mode === mode} onClick={() => onMode(mode)}>{mode === 'subjects' ? 'Subjects' : 'Map'}</button>)}
    </div>
    <button type="button" className="brain-view-fit" onClick={onFit}>Fit</button>
    <div className="brain-view-time">{timeControl}</div>
    <div className="brain-view-slots" role="group" aria-label="Saved Brain views">
      {slots.map((slot, index) => {
        const button = <button type="button" className={`brain-view-slot${slot ? ' filled' : ''}`}
          ref={element => { slotButtons.current[index] = element }}
          aria-label={slot ? `Restore ${slot.name}` : `Save view ${index + 1}`}
          title={slot ? `${slot.name} — restore; menu to rename, replace or clear` : `Save current view in slot ${index + 1}`}
          onKeyDown={event => {
            if (!slot || (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10'))) return
            event.preventDefault()
            const box = event.currentTarget.getBoundingClientRect()
            event.currentTarget.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, clientX: box.left, clientY: box.top }))
          }}
          onClick={() => { if (slot) onRestore(copyBrainView(slot.snapshot)); else replace(index) }}>{index + 1}</button>
        return <span className="brain-view-slot-wrap" key={index}>
          {slot ? <MenuTarget label={`Actions for ${slot.name}`} groups={() => [{ id: 'view', rows: [
            { id: 'rename', label: 'Rename', onSelect: () => { setName(slot.name); setRenaming(index) } },
            { id: 'replace', label: 'Replace', onSelect: () => { replace(index) } },
            { id: 'clear', label: 'Clear', onSelect: () => { save(slots.map((saved, at) => at === index ? null : saved)) } },
          ] }]}>{button}</MenuTarget> : button}
          {renaming === index && <form className="brain-view-rename" onSubmit={rename} onKeyDown={event => {
            if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeRename() }
          }}>
            <input ref={renameInput} aria-label={`Name for view ${index + 1}`} value={name}
              onChange={event => setName(event.target.value)} />
            <button type="submit" disabled={!name.trim()}>Save</button>
            <button type="button" onClick={closeRename}>Cancel</button>
          </form>}
        </span>
      })}
    </div>
    {error && <p className="brain-view-error" role="alert">{error}</p>}
  </footer>
}
