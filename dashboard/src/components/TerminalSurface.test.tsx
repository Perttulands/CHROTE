import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalSurface from './TerminalSurface'
import type { TerminalSession } from '../terminal/terminalSession'

const observers: { callback: () => void; target: Element }[] = []

class StubResizeObserver {
  constructor(private readonly callback: () => void) {}
  observe(target: Element) { observers.push({ callback: this.callback, target }) }
  disconnect() { observers.length = 0 }
}

function stubSession() {
  const element = document.createElement('div')
  element.textContent = 'terminal'
  const calls = { attach: 0, detach: 0, fit: 0 }
  const session: TerminalSession = {
    attach(container) { calls.attach += 1; container.appendChild(element) },
    detach() { calls.detach += 1; element.remove() },
    fit() { calls.fit += 1 },
    focus: vi.fn(),
    setFontSize: vi.fn(),
    setScrollbarHidden: vi.fn(),
    reconnect: vi.fn(),
    dispose: vi.fn(),
  }
  return { session, element, calls }
}

describe('TerminalSurface', () => {
  beforeEach(() => {
    observers.length = 0
    vi.stubGlobal('ResizeObserver', StubResizeObserver)
  })

  it('puts the terminal on screen and takes it off again when the tile goes away', () => {
    const { session, element, calls } = stubSession()
    const { container, unmount } = render(<TerminalSurface session={session} />)

    expect(container.querySelector('.terminal-surface-host')?.contains(element)).toBe(true)

    unmount()

    expect(calls.detach).toBe(1)
    expect(element.isConnected).toBe(false)
  })

  it('refits the grid when the frame is resized', () => {
    vi.useFakeTimers()
    const { session, calls } = stubSession()
    render(<TerminalSurface session={session} />)
    const fitsAfterMount = calls.fit

    observers.forEach(entry => entry.callback())
    vi.runAllTimers()

    expect(calls.fit).toBeGreaterThan(fitsAfterMount)
    vi.useRealTimers()
  })

  it('leaves a hidden terminal connected but never resizes it, because the tmux window is shared', () => {
    const { session, element, calls } = stubSession()
    const { container } = render(<TerminalSurface session={session} hidden />)

    expect(element.isConnected).toBe(true)
    expect(container.querySelector<HTMLElement>('.terminal-surface-host')?.style.display).toBe('none')
    expect(calls.fit).toBe(0)
    expect(observers).toHaveLength(0)
  })

  it('renders an empty slot until the pool has a terminal for this session', () => {
    const { container } = render(<TerminalSurface session={null} />)

    expect(container.querySelector('.terminal-surface-host')?.childElementCount).toBe(0)
  })
})
