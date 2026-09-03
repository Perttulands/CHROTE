import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MenuTarget from './MenuTarget'
import { LONG_PRESS_MS } from './useContextMenu'

const open = vi.fn()

function renderRow() {
  render(
    <MenuTarget label="Actions for the row" groups={() => [{ id: 'g', rows: [{ id: 'open', label: 'Open', onSelect: open }] }]}>
      <div>The row</div>
    </MenuTarget>,
  )
  return screen.getByText('The row')
}

describe('MenuTarget', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.useRealTimers()
    open.mockReset()
  })

  it('opens the menu on right-click and runs the chosen row', () => {
    const row = renderRow()

    fireEvent.contextMenu(row, { clientX: 40, clientY: 50 })
    fireEvent.click(screen.getByRole('menuitem', { name: 'Open' }))

    expect(open).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('menu')).toBeNull()
  })

  // A finger held on the row is the same request; one that lifts or moves
  // first was a tap or a scroll.
  it('opens the menu from a long press, and not from a shorter one', () => {
    const row = renderRow()

    fireEvent.touchStart(row, { touches: [{ clientX: 40, clientY: 50 }] })
    act(() => { vi.advanceTimersByTime(LONG_PRESS_MS - 1) })
    fireEvent.touchEnd(row)
    act(() => { vi.advanceTimersByTime(LONG_PRESS_MS) })
    expect(screen.queryByRole('menu')).toBeNull()

    fireEvent.touchStart(row, { touches: [{ clientX: 40, clientY: 50 }] })
    act(() => { vi.advanceTimersByTime(LONG_PRESS_MS) })
    expect(screen.getByRole('menu', { name: 'Actions for the row' })).toBeInTheDocument()
  })
})
