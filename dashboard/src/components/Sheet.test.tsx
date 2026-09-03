import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Sheet from './Sheet'
import { resetSurfacesForTest } from '../keys/dismiss'

afterEach(() => {
  resetSurfacesForTest()
})

describe('Sheet', () => {
  it('docks to the edge it is given, at the extent it is given', () => {
    const { container, rerender } = render(
      <Sheet open edge="left" extent="932px" label="Peek main" onClose={vi.fn()}>body</Sheet>,
    )

    const sheet = container.querySelector('.sheet') as HTMLElement
    expect(sheet).toHaveClass('sheet-left')
    expect(sheet.style.width).toBe('932px')

    rerender(<Sheet open edge="bottom" extent="40%" label="Peek main" onClose={vi.fn()}>body</Sheet>)
    const bottom = container.querySelector('.sheet') as HTMLElement
    expect(bottom).toHaveClass('sheet-bottom')
    // A bottom sheet takes height, not width: the extent means the same thing
    // whichever edge it is measured from.
    expect(bottom.style.height).toBe('40%')
    expect(bottom.style.width).toBe('')
  })

  it('stays through a press outside as a work surface, and closes on Escape', () => {
    const onClose = vi.fn()
    render(<Sheet open edge="right" extent="380px" label="Send" onClose={onClose}>body</Sheet>)

    fireEvent.pointerDown(document.body)
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('closes on a press outside as a glance, and not on one inside', () => {
    const onClose = vi.fn()
    render(<Sheet open edge="left" extent="50%" kind="glance" label="Peek main" onClose={onClose}>body</Sheet>)

    fireEvent.pointerDown(screen.getByText('body'))
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.pointerDown(document.body)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('draws nothing at all while it is closed', () => {
    const { container } = render(
      <Sheet open={false} edge="left" extent="50%" label="Peek main" onClose={vi.fn()}>body</Sheet>,
    )

    expect(container.querySelector('.sheet')).toBeNull()
    expect(screen.queryByText('body')).toBeNull()
  })
})
