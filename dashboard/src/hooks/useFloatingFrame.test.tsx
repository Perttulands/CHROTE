import { fireEvent, render, screen } from '@testing-library/react'
import { useRef } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { useFloatingFrame } from './useFloatingFrame'
import { readFloatingWindowSize } from './floatingWindowSize'

/**
 * A window in a workspace of a known size. jsdom reports every element as
 * 0x0, so the workspace is given client dimensions directly; the pointer drag
 * itself is a browser journey, not a unit.
 */
function Subject({ content }: { content: { width: number; height: number } | null }) {
  const elementRef = useRef<HTMLDivElement>(null)
  const frame = useFloatingFrame({
    kind: 'image',
    elementRef,
    open: true,
    label: 'image',
    contentSize: () => content,
  })

  return (
    <div
      ref={node => {
        if (!node) return
        Object.defineProperty(node, 'clientWidth', { value: 1000, configurable: true })
        Object.defineProperty(node, 'clientHeight', { value: 800, configurable: true })
      }}
    >
      <div ref={elementRef} data-testid="window" style={frame.size ? { width: frame.size.width, height: frame.size.height } : undefined}>
        {frame.handles.map(handle => (
          <div key={handle.id} {...handle.props} data-testid={`handle-${handle.id}`} />
        ))}
        {frame.remembered && <button type="button" onClick={frame.resetSize}>Reset size</button>}
      </div>
    </div>
  )
}

afterEach(() => {
  localStorage.clear()
})

describe('useFloatingFrame', () => {
  it('steps 16px per arrow toward the handle, remembers the size, and gives the content the say back on reset', () => {
    render(<Subject content={{ width: 400, height: 300 }} />)
    const frame = screen.getByTestId('window')
    expect(frame).toHaveStyle({ width: '400px', height: '300px' })
    expect(screen.queryByRole('button', { name: 'Reset size' })).toBeNull()

    fireEvent.keyDown(screen.getByTestId('handle-se'), { key: 'ArrowRight' })
    expect(frame).toHaveStyle({ width: '416px', height: '300px' })

    fireEvent.keyDown(screen.getByTestId('handle-se'), { key: 'ArrowDown' })
    expect(frame).toHaveStyle({ width: '416px', height: '316px' })

    // The west handle grows to the west, so the same arrow shrinks there.
    fireEvent.keyDown(screen.getByTestId('handle-w'), { key: 'ArrowRight' })
    expect(frame).toHaveStyle({ width: '400px', height: '316px' })

    expect(readFloatingWindowSize('image')).toEqual({ width: 400, height: 316 })

    fireEvent.click(screen.getByRole('button', { name: 'Reset size' }))
    expect(frame).toHaveStyle({ width: '400px', height: '300px' })
    expect(readFloatingWindowSize('image')).toBeNull()
  })

  it('opens at the remembered size whatever the content asks for', () => {
    localStorage.setItem('chrote.floatingWindowSize.v1', JSON.stringify({
      version: 1,
      sizes: { image: { width: 640, height: 480 } },
    }))
    render(<Subject content={{ width: 40, height: 30 }} />)
    expect(screen.getByTestId('window')).toHaveStyle({ width: '640px', height: '480px' })
  })
})
