import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import Sheet from './Sheet'

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

  it('lays no backdrop over the work behind it, and a click outside is just a click', () => {
    const onClose = vi.fn()
    render(<Sheet open edge="left" extent="50%" label="Peek main" onClose={onClose}>body</Sheet>)

    expect(document.querySelector('.floating-panel-dismiss-layer')).toBeNull()

    fireEvent.pointerDown(document.body)
    fireEvent.click(document.body)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes on Escape, from wherever the cursor is', () => {
    const onClose = vi.fn()
    render(<Sheet open edge="right" extent="380px" label="Send" onClose={onClose}>body</Sheet>)

    fireEvent.keyDown(document, { key: 'Escape' })
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
