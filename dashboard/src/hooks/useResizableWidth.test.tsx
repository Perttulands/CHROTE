import { fireEvent, render, screen } from '@testing-library/react'
import { useCallback, useRef } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useResizableWidth, type ResizeEdge } from './useResizableWidth'

interface SubjectProps {
  edge: ResizeEdge
  width: number
  minWidth?: number
  maxWidth?: number
  onCommit: (width: number) => void
}

function Subject({ edge, width, minWidth = 120, maxWidth = 400, onCommit }: SubjectProps) {
  const elementRef = useRef<HTMLDivElement>(null)
  const widest = useCallback(() => maxWidth, [maxWidth])
  const resize = useResizableWidth({
    elementRef,
    width,
    minWidth,
    maxWidth: widest,
    edge,
    onCommit,
  })

  return (
    <div ref={elementRef}>
      <div role="separator" tabIndex={0} {...resize.handleProps} />
    </div>
  )
}

describe('useResizableWidth', () => {
  it('steps toward the handle by 16px and reverses direction between edges', () => {
    const commit = vi.fn()
    const { rerender } = render(<Subject edge="right" width={240} onCommit={commit} />)
    const handle = screen.getByRole('separator')

    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(commit).toHaveBeenLastCalledWith(256)
    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(commit).toHaveBeenLastCalledWith(224)

    rerender(<Subject edge="left" width={240} onCommit={commit} />)
    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(commit).toHaveBeenLastCalledWith(256)
    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(commit).toHaveBeenLastCalledWith(224)
  })

  it('clamps keyboard changes to the caller-owned limits', () => {
    const commit = vi.fn()
    const { rerender } = render(
      <Subject edge="right" width={128} minWidth={120} maxWidth={132} onCommit={commit} />,
    )
    const handle = screen.getByRole('separator')

    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(commit).toHaveBeenLastCalledWith(132)

    rerender(<Subject edge="right" width={120} minWidth={120} maxWidth={132} onCommit={commit} />)
    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(commit).toHaveBeenLastCalledWith(120)
  })

  it('previews a drag and commits its clamped width on pointer up', () => {
    const commit = vi.fn()
    render(<Subject edge="right" width={240} minWidth={120} maxWidth={260} onCommit={commit} />)
    const handle = screen.getByRole('separator')
    const rail = handle.parentElement as HTMLDivElement
    Object.defineProperty(rail, 'getBoundingClientRect', {
      value: () => ({ width: 240 }),
    })
    Object.defineProperties(handle, {
      setPointerCapture: { value: vi.fn() },
      hasPointerCapture: { value: () => true },
      releasePointerCapture: { value: vi.fn() },
    })

    fireEvent.pointerDown(handle, { button: 0, pointerId: 7, clientX: 240 })
    fireEvent.pointerMove(handle, { pointerId: 7, clientX: 300 })
    expect(commit).not.toHaveBeenCalled()
    fireEvent.pointerUp(handle, { pointerId: 7, clientX: 300 })
    expect(commit).toHaveBeenCalledWith(260)
  })
})
