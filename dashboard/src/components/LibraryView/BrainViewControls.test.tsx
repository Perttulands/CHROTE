import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BrainViewControls from './BrainViewControls'
import { BRAIN_VIEWS_KEY } from './brainViewState'
import type { BrainViewSnapshot } from './brainViewState'
import { resetSurfacesForTest } from '../../keys/dismiss'

const snapshot: BrainViewSnapshot = {
  mode: 'map', subject: 'knowledge/testing', picked: 'knowledge/testing/note.md', facet: 'software', collection: 'knowledge',
  hiddenKinds: ['source'], hiddenShelves: ['projects'], scrub: 670,
  cameras: { map: { x: -120, y: 340, scale: 1.8 }, subjects: { x: 20, y: 40, scale: 0.8 } },
}

function controls(value = snapshot) {
  const onRestore = vi.fn()
  const onMode = vi.fn()
  const onFit = vi.fn()
  const props = { snapshot: value, onRestore, onMode, onFit, timeControl: <input type="range" aria-label="Map time" /> }
  return { ...render(<BrainViewControls {...props} />), props, onRestore, onMode, onFit }
}

beforeEach(() => { localStorage.clear(); resetSurfacesForTest() })
afterEach(() => vi.restoreAllMocks())

describe('Brain view controls', () => {
  it('saves a complete current view and restores it after remounting', () => {
    const first = controls()
    fireEvent.click(screen.getByRole('button', { name: 'Save view 1' }))
    first.unmount()
    const second = controls({ ...snapshot, mode: 'subjects', facet: '' })
    fireEvent.click(screen.getByRole('button', { name: 'Restore View 1' }))
    expect(second.onRestore).toHaveBeenCalledWith(snapshot)
    expect(screen.getAllByRole('button', { name: /^Save view/ })).toHaveLength(3)
  })

  it('renames, replaces and clears a slot through its accessible menu', () => {
    const view = controls()
    fireEvent.click(screen.getByRole('button', { name: 'Save view 2' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Restore View 2' }), { key: 'F10', shiftKey: true })
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename' }))
    const input = screen.getByRole('textbox', { name: 'Name for view 2' })
    expect(input).toHaveFocus()
    fireEvent.change(input, { target: { value: 'Evidence' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    const current = { ...snapshot, mode: 'subjects' as const, collection: 'self', scrub: 1000 }
    view.rerender(<BrainViewControls {...view.props} snapshot={current} />)
    fireEvent.contextMenu(screen.getByRole('button', { name: 'Restore Evidence' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Replace' }))
    fireEvent.click(screen.getByRole('button', { name: 'Restore Evidence' }))
    expect(view.onRestore).toHaveBeenLastCalledWith(current)
    fireEvent.contextMenu(screen.getByRole('button', { name: 'Restore Evidence' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Clear' }))
    expect(screen.getByRole('button', { name: 'Save view 2' })).toBeInTheDocument()
    view.unmount()
    controls()
    expect(screen.getByRole('button', { name: 'Save view 2' })).toBeInTheDocument()
  })

  it('keeps a slot unchanged when rename is cancelled', () => {
    controls()
    fireEvent.click(screen.getByRole('button', { name: 'Save view 1' }))
    fireEvent.contextMenu(screen.getByRole('button', { name: 'Restore View 1' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Rename' }))
    const input = screen.getByRole('textbox', { name: 'Name for view 1' })
    fireEvent.change(input, { target: { value: 'Unsaved name' } })
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.getByRole('button', { name: 'Restore View 1' })).toHaveFocus()
  })

  it('switches modes, fits the drawing, and retains the supplied time control', () => {
    const view = controls()
    expect(screen.getByRole('button', { name: 'Map' })).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(screen.getByRole('button', { name: 'Subjects' }))
    expect(view.onMode).toHaveBeenCalledWith('subjects')
    fireEvent.click(screen.getByRole('button', { name: 'Fit' }))
    expect(view.onFit).toHaveBeenCalledOnce()
    expect(screen.getByRole('slider', { name: 'Map time' })).toBeInTheDocument()
  })

  it('reports storage failure and never presents a failed write as saved', () => {
    controls()
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Storage is full') })
    fireEvent.click(screen.getByRole('button', { name: 'Save view 1' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Storage is full')
    expect(screen.getByRole('button', { name: 'Save view 1' })).toBeInTheDocument()
  })

  it('reports unreadable storage while keeping the original record', () => {
    localStorage.setItem(BRAIN_VIEWS_KEY, '{broken')
    controls()
    expect(screen.getByRole('alert')).toHaveTextContent('invalid stored data')
    expect(localStorage.getItem(BRAIN_VIEWS_KEY)).toBe('{broken')
  })
})
