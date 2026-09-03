import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import KeysPanel from './KeysPanel'
import { registerChords, resetChordsForTest, type Chord } from './chords'

const beads = vi.fn()
const send = vi.fn()

const chords: Chord[] = [
  { id: 'tab1', key: '1', direct: { alt: true, shift: false, key: '1' }, label: 'Terminal tab 1', scope: 'global', run: vi.fn() },
  { id: 'beads', key: 'b', direct: { alt: true, shift: false, key: 'b' }, label: 'Beads tab', scope: 'global', run: beads },
  { id: 'send', key: 's', direct: { alt: true, shift: false, key: 's' }, label: 'Send to the tile\'s session', scope: 'global', run: send },
]

describe('KeysPanel', () => {
  let retire: () => void

  beforeEach(() => {
    resetChordsForTest()
    retire = registerChords(chords)
    beads.mockClear()
    send.mockClear()
  })

  afterEach(() => {
    retire()
    resetChordsForTest()
  })

  it('lists every chord as CHORD then action, and lays no backdrop over the workspace', () => {
    const { container } = render(<KeysPanel isOpen onClose={vi.fn()} />)

    const rows = Array.from(container.querySelectorAll('.keys-panel-chord'))
    expect(rows.slice(0, 3).map(row => row.querySelector('.keys-panel-key')?.textContent))
      .toEqual(['ALT + 1', 'ALT + B', 'ALT + S'])
    expect(rows[1].querySelector('.keys-panel-label')?.textContent).toBe('Beads tab')
    // The leader is not a registered chord; it is stated once, as the way in.
    expect(container.textContent).toContain('CTRL + SHIFT + SPACE')
    expect(document.querySelector('.keys-panel-backdrop')).toBeNull()
    expect(screen.getByPlaceholderText('Keybindings…')).toHaveFocus()
  })

  it('filters on either column as the operator types', () => {
    const { container } = render(<KeysPanel isOpen onClose={vi.fn()} />)
    const search = screen.getByPlaceholderText('Keybindings…')

    fireEvent.change(search, { target: { value: 'beads' } })
    expect(container.querySelectorAll('.keys-panel-chord')).toHaveLength(1)

    fireEvent.change(search, { target: { value: 'alt + s' } })
    expect(container.querySelectorAll('.keys-panel-chord')).toHaveLength(1)
    expect(container.querySelector('.keys-panel-label')?.textContent).toBe("Send to the tile's session")

    fireEvent.change(search, { target: { value: 'no such chord' } })
    expect(container.querySelector('.keys-panel-empty')).toBeInTheDocument()
  })

  it('moves the current row with the arrows and runs it on Enter', () => {
    const onClose = vi.fn()
    const { container } = render(<KeysPanel isOpen onClose={onClose} />)
    const search = screen.getByPlaceholderText('Keybindings…')

    expect(container.querySelectorAll('.keys-panel-chord')[0]).toHaveClass('current')

    fireEvent.keyDown(search, { key: 'ArrowDown' })
    expect(container.querySelectorAll('.keys-panel-chord')[1]).toHaveClass('current')

    fireEvent.keyDown(search, { key: 'Enter' })
    expect(beads).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalled()
  })

  it('closes on Escape from wherever the cursor is', () => {
    const onClose = vi.fn()
    render(<KeysPanel isOpen onClose={onClose} />)

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
