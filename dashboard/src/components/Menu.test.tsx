import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Menu, { type MenuGroup } from './Menu'

const groups: MenuGroup[] = [
  {
    id: 'look',
    rows: [
      { id: 'peek', label: 'Peek', chord: 'Alt+P', onSelect: vi.fn() },
      { id: 'send', label: 'Send to session', chord: 'Alt+S', onSelect: vi.fn() },
    ],
  },
  {
    id: 'end',
    rows: [
      { id: 'kill', label: 'Kill session', danger: true, confirmLabel: 'Confirm kill', onSelect: vi.fn() },
    ],
  },
]

function renderMenu(overrides: Partial<Parameters<typeof Menu>[0]> = {}) {
  const onClose = vi.fn()
  render(<Menu at={{ x: 10, y: 20 }} label="Session actions" groups={groups} onClose={onClose} {...overrides} />)
  return { onClose }
}

describe('Menu', () => {
  it('draws rows of words with their chords, in groups, and no icons', () => {
    renderMenu()

    const menu = screen.getByRole('menu', { name: 'Session actions' })
    // Words, not glyphs: every row's own text is the action itself.
    expect(screen.getAllByRole('menuitem').map(row => row.querySelector('.menu-row-label')?.textContent))
      .toEqual(['Peek', 'Send to session', 'Kill session'])
    expect(menu.querySelectorAll('.menu-group')).toHaveLength(2)

    // The chord is printed beside the action and spoken as a shortcut, so it is
    // discoverable at the point of use without becoming part of the row's name.
    expect(screen.getByRole('menuitem', { name: 'Peek' })).toHaveAttribute('aria-keyshortcuts', 'Alt+P')
    expect(menu.textContent).toContain('Alt+P')
  })

  it('moves the highlight with the arrows and wraps at the ends', () => {
    renderMenu()

    const menu = screen.getByRole('menu', { name: 'Session actions' })
    expect(screen.getByRole('menuitem', { name: 'Peek' })).toHaveFocus()

    fireEvent.keyDown(menu, { key: 'ArrowDown' })
    expect(screen.getByRole('menuitem', { name: 'Send to session' })).toHaveFocus()

    fireEvent.keyDown(menu, { key: 'End' })
    expect(screen.getByRole('menuitem', { name: 'Kill session' })).toHaveFocus()

    fireEvent.keyDown(menu, { key: 'ArrowDown' })
    expect(screen.getByRole('menuitem', { name: 'Peek' })).toHaveFocus()

    fireEvent.keyDown(menu, { key: 'ArrowUp' })
    expect(screen.getByRole('menuitem', { name: 'Kill session' })).toHaveFocus()
  })

  it('runs a plain row once and closes behind it', () => {
    const { onClose } = renderMenu()

    fireEvent.click(screen.getByRole('menuitem', { name: 'Peek' }))

    expect(groups[0].rows[0]).toHaveProperty('onSelect')
    expect((groups[0].rows[0] as { onSelect: () => void }).onSelect).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalled()
  })

  describe('confirming in place', () => {
    beforeEach(() => vi.useFakeTimers())
    afterEach(() => vi.useRealTimers())

    it('arms a destructive row and runs it only on a second press inside three seconds', () => {
      const kill = vi.fn()
      const onClose = vi.fn()
      render(
        <Menu
          at={{ x: 0, y: 0 }}
          label="Session actions"
          onClose={onClose}
          groups={[{ id: 'end', rows: [{ id: 'kill', label: 'Kill session', danger: true, confirmLabel: 'Confirm kill', onSelect: kill }] }]}
        />,
      )

      fireEvent.click(screen.getByRole('menuitem', { name: 'Kill session' }))
      expect(kill).not.toHaveBeenCalled()
      expect(onClose).not.toHaveBeenCalled()
      expect(screen.getByRole('menuitem', { name: 'Confirm kill' })).toBeInTheDocument()

      fireEvent.click(screen.getByRole('menuitem', { name: 'Confirm kill' }))
      expect(kill).toHaveBeenCalledTimes(1)
      expect(onClose).toHaveBeenCalled()
    })

    it('disarms itself when the second press does not come', () => {
      const kill = vi.fn()
      render(
        <Menu
          at={{ x: 0, y: 0 }}
          label="Session actions"
          onClose={vi.fn()}
          groups={[{ id: 'end', rows: [{ id: 'kill', label: 'Kill session', danger: true, confirmLabel: 'Confirm kill', onSelect: kill }] }]}
        />,
      )

      fireEvent.click(screen.getByRole('menuitem', { name: 'Kill session' }))
      act(() => { vi.advanceTimersByTime(3000) })

      expect(screen.getByRole('menuitem', { name: 'Kill session' })).toBeInTheDocument()
      expect(screen.queryByRole('menuitem', { name: 'Confirm kill' })).not.toBeInTheDocument()
      expect(kill).not.toHaveBeenCalled()
    })
  })
})
