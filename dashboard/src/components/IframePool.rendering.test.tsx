import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useEffect, useRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IframePoolProvider, useIframePool } from './IframePool'

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { fontSize: 14 },
    sessions: [
      { name: 'smooth-scroll', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ],
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['alice:smooth-scroll'], activeSession: 'alice:smooth-scroll', colorIndex: 0 },
        ],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
  }),
}))

function ClaimedIframe() {
  const targetRef = useRef<HTMLDivElement>(null)
  const pool = useIframePool()

  useEffect(() => {
    if (!targetRef.current) return
    return pool.claimIframe('alice:smooth-scroll', targetRef.current)
  }, [pool])

  return (
    <>
      <div data-testid="iframe-target" ref={targetRef} />
      <button type="button" onClick={() => pool.triggerFit('alice:smooth-scroll')}>Fit terminal</button>
    </>
  )
}

describe('IframePool attached terminal rendering', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('uses an opaque non-scrolling ttyd iframe instead of the old transparent theme path', async () => {
    render(
      <IframePoolProvider>
        <ClaimedIframe />
      </IframePoolProvider>
    )

    const iframe = await waitFor(() => {
      const target = document.querySelector('[data-testid="iframe-target"]')
      const node = target?.querySelector('iframe') as HTMLIFrameElement | null
      expect(node).not.toBeNull()
      expect(node?.getAttribute('src')).toBe('/terminal/?arg=smooth-scroll&arg=alice')
      return node!
    })

    expect(iframe.getAttribute('src')).not.toContain('theme=')
    expect(iframe.getAttribute('scrolling')).toBe('no')
    expect(iframe.style.backgroundColor).toBe('rgb(10, 10, 10)')
    expect(iframe.style.overflow).toBe('hidden')
  })

  it('finishes a bounded three-pass fit after the visible iframe geometry settles', async () => {
    render(
      <IframePoolProvider>
        <ClaimedIframe />
      </IframePoolProvider>
    )

    const iframe = await waitFor(() => {
      const node = document.querySelector('[data-testid="iframe-target"] iframe') as HTMLIFrameElement | null
      expect(node).not.toBeNull()
      return node!
    })
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 800 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 600 })
    const dispatch = vi.spyOn(iframe.contentWindow!, 'dispatchEvent')
    vi.useFakeTimers()

    fireEvent.click(screen.getByRole('button', { name: 'Fit terminal' }))
    expect(dispatch).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(200)
    expect(dispatch).toHaveBeenCalledTimes(2)

    vi.advanceTimersByTime(300)
    expect(dispatch).toHaveBeenCalledTimes(3)
  })

  it('cancels pending fits when the document becomes hidden', async () => {
    const originalVisibility = Object.getOwnPropertyDescriptor(document, 'visibilityState')
    try {
      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
      render(
        <IframePoolProvider>
          <ClaimedIframe />
        </IframePoolProvider>
      )

      const iframe = await waitFor(() => {
        const node = document.querySelector('[data-testid="iframe-target"] iframe') as HTMLIFrameElement | null
        expect(node).not.toBeNull()
        return node!
      })
      Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 800 })
      Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 600 })
      const dispatch = vi.spyOn(iframe.contentWindow!, 'dispatchEvent')
      vi.useFakeTimers()

      fireEvent.click(screen.getByRole('button', { name: 'Fit terminal' }))
      expect(dispatch).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBe(2)

      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
      fireEvent(document, new Event('visibilitychange'))
      expect(vi.getTimerCount()).toBe(0)
      vi.advanceTimersByTime(500)
      expect(dispatch).toHaveBeenCalledTimes(1)

      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
      fireEvent(document, new Event('visibilitychange'))
      expect(dispatch).toHaveBeenCalledTimes(2)
      expect(vi.getTimerCount()).toBe(2)
      vi.advanceTimersByTime(500)
      expect(dispatch).toHaveBeenCalledTimes(4)
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      if (originalVisibility) {
        Object.defineProperty(document, 'visibilityState', originalVisibility)
      }
    }
  })
})
