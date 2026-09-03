import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Toast, { TOAST_FADE_IN_MS, TOAST_FADE_OUT_MS, TOAST_HOLD_MS } from './Toast'
import { StatusProvider, useStatus, type StatusSeverity } from '../context/StatusContext'

let announce: (message: string, severity: StatusSeverity) => void

function Announcer() {
  announce = useStatus().announce
  return null
}

function renderToast() {
  const rendered = render(
    <StatusProvider>
      <Announcer />
      <Toast />
    </StatusProvider>,
  )
  return { ...rendered, toast: () => rendered.container.querySelector('.toast') }
}

describe('Toast', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('raises an announcement, holds it, and takes it down again', () => {
    const { toast } = renderToast()
    expect(toast()).toBeNull()

    act(() => announce('Copied chrote-5grx.49', 'success'))
    expect(toast()).toHaveTextContent('Copied chrote-5grx.49')
    // The raised class is the only proxy for opacity a DOM test can read: it
    // is what the fade-in transitions to, and what the fade-out leaves.
    expect(toast()).toHaveClass('toast-raised')

    act(() => { vi.advanceTimersByTime(TOAST_FADE_IN_MS + TOAST_HOLD_MS) })
    expect(toast()).not.toHaveClass('toast-raised')
    expect(toast()).toHaveTextContent('Copied chrote-5grx.49')

    act(() => { vi.advanceTimersByTime(TOAST_FADE_OUT_MS) })
    expect(toast()).toBeNull()
  })

  it('shows the newest of two announcements and starts the hold over', () => {
    const { toast } = renderToast()

    act(() => announce('Layout saved', 'success'))
    act(() => { vi.advanceTimersByTime(1000) })
    act(() => announce('Copied chrote-5grx.49', 'success'))

    expect(toast()).toHaveTextContent('Copied chrote-5grx.49')
    expect(toast()).not.toHaveTextContent('Layout saved')

    // Past the first announcement's hold, the second is still up on its own.
    act(() => { vi.advanceTimersByTime(TOAST_FADE_IN_MS + TOAST_HOLD_MS - 500) })
    expect(toast()).toHaveClass('toast-raised')

    act(() => { vi.advanceTimersByTime(500) })
    expect(toast()).not.toHaveClass('toast-raised')
    act(() => { vi.advanceTimersByTime(TOAST_FADE_OUT_MS) })
    expect(toast()).toBeNull()
  })

  it('comes back up when an announcement lands during the fade-out', () => {
    const { toast } = renderToast()

    act(() => announce('Sent to claude-chrote-ui', 'success'))
    act(() => { vi.advanceTimersByTime(TOAST_FADE_IN_MS + TOAST_HOLD_MS + 50) })
    expect(toast()).not.toHaveClass('toast-raised')

    act(() => announce('Could not copy', 'error'))
    expect(toast()).toHaveClass('toast-raised')
    expect(toast()).toHaveTextContent('Could not copy')

    act(() => { vi.advanceTimersByTime(TOAST_FADE_OUT_MS) })
    expect(toast()).not.toBeNull()
  })

  it('colours a failure and nothing else', () => {
    const { toast } = renderToast()

    act(() => announce('Could not copy chrote-5grx.49: the browser refused', 'error'))
    expect(toast()).toHaveClass('toast-failure')

    act(() => announce('Harness did not start', 'warning'))
    expect(toast()).not.toHaveClass('toast-failure')
  })

  it('leaves information on the status line and shows no toast for it', () => {
    const { toast } = renderToast()

    act(() => announce('Library loaded · 72 pages on 8 shelves', 'info'))
    act(() => { vi.advanceTimersByTime(TOAST_FADE_IN_MS) })

    expect(toast()).toBeNull()
  })
})
