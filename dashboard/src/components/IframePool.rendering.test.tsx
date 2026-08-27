import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useEffect, useRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IframePoolProvider, useIframePool } from './IframePool'

const defaultMockSessions = [
  { name: 'smooth-scroll', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
]
const defaultMockWorkspaces = {
  terminal1: {
    windowCount: 1,
    windows: [
      { id: 'terminal1-window-0', boundSessions: ['alice:smooth-scroll'], activeSession: 'alice:smooth-scroll', colorIndex: 0 },
    ],
  },
  terminal2: { windowCount: 1, windows: [] },
  terminal3: { windowCount: 1, windows: [] },
}
const mockSessionState = {
  sessions: defaultMockSessions,
  workspaces: defaultMockWorkspaces as Record<string, unknown>,
}

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    settings: { fontSize: 14 },
    sessions: mockSessionState.sessions,
    workspaces: mockSessionState.workspaces,
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

interface TermStub {
  options: { fontSize: number }
  fit: ReturnType<typeof vi.fn>
  fontSizeAtFit: number[]
}

/** Plant a fake ttyd `window.term` inside the iframe, recording the fontSize seen by each fit(). */
function installTermStub(iframe: HTMLIFrameElement): TermStub {
  const term: TermStub = {
    options: { fontSize: 0 },
    fontSizeAtFit: [],
    fit: vi.fn(() => { term.fontSizeAtFit.push(term.options.fontSize) }),
  }
  ;(iframe.contentWindow as Window & { term?: TermStub }).term = term
  return term
}

type PoolApi = ReturnType<typeof useIframePool>

/** Render the provider with an imperative claim/release handle on a detached-from-React container. */
function renderPoolHarness() {
  const harness = {
    pool: null as PoolApi | null,
    container: document.createElement('div'),
    releaseFn: null as (() => void) | null,
    claim() {
      harness.releaseFn = harness.pool!.claimIframe('alice:smooth-scroll', harness.container)
    },
    release() {
      harness.releaseFn?.()
      harness.releaseFn = null
    },
    async claimedIframe(): Promise<HTMLIFrameElement> {
      await waitFor(() => {
        expect(harness.container.querySelector('iframe')).not.toBeNull()
      })
      return harness.container.querySelector('iframe')!
    },
  }

  function CaptureApi() {
    harness.pool = useIframePool()
    return null
  }

  render(
    <IframePoolProvider>
      <CaptureApi />
    </IframePoolProvider>
  )
  document.body.appendChild(harness.container)
  act(() => { harness.claim() })
  return harness
}

describe('IframePool attached terminal rendering', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    mockSessionState.sessions = defaultMockSessions
    mockSessionState.workspaces = defaultMockWorkspaces
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('pools iframes for sessions bound in any workspace present in state, not just the default tab set', async () => {
    // A visibility-derived enumeration would miss this workspace id and the
    // allSessions cleanup effect would then prune its live iframe.
    mockSessionState.sessions = [
      { name: 'hidden-session', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ]
    mockSessionState.workspaces = {
      terminal1: { windowCount: 1, windows: [] },
      terminal7: {
        windowCount: 1,
        windows: [
          { id: 'terminal7-window-0', boundSessions: ['alice:hidden-session'], activeSession: 'alice:hidden-session', colorIndex: 0 },
        ],
      },
    }

    render(
      <IframePoolProvider>
        <div />
      </IframePoolProvider>
    )

    await waitFor(() => {
      expect(document.querySelectorAll('iframe')).toHaveLength(1)
    })
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

  it('fits by calling the ttyd client term.fit() directly, without dispatching resize events or scheduling timers', async () => {
    // The ttyd client only listens for window resize AFTER its WebSocket
    // opens, so dispatched resize events are lost pre-open. term.fit exists
    // from xterm open() onward and is race-free. Evidence:
    // /srv/data/chrote/evidence/fit-probe-20260803/ttyd-client-excerpts.md
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
    const term = installTermStub(iframe)
    const dispatch = vi.spyOn(iframe.contentWindow!, 'dispatchEvent')
    vi.useFakeTimers()

    fireEvent.click(screen.getByRole('button', { name: 'Fit terminal' }))
    expect(term.fit).toHaveBeenCalledTimes(1)
    expect(dispatch).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'resize' }))
    expect(vi.getTimerCount()).toBe(0)
  })

  it('never fits an unclaimed (parked) iframe — a parked fit resizes the shared tmux window for other clients', async () => {
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 800 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 600 })

    act(() => { harness.release() })
    // jsdom recreates contentWindow on reparent (as real Chrome reloads on
    // appendChild moves), so plant the stub on the post-release window.
    const parkedTerm = installTermStub(iframe)
    act(() => { harness.pool!.triggerFit('alice:smooth-scroll') })
    expect(parkedTerm.fit).not.toHaveBeenCalled()

    act(() => { harness.claim() })
    const claimedTerm = installTermStub(iframe)
    act(() => { harness.pool!.triggerFit('alice:smooth-scroll') })
    expect(claimedTerm.fit).toHaveBeenCalledTimes(1)
  })

  it('applies font size and then fits, in that order, so the grid is computed with final cell metrics', async () => {
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 800 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 600 })
    const term = installTermStub(iframe)

    act(() => { harness.pool!.applyFontSize('alice:smooth-scroll', 17) })
    expect(term.options.fontSize).toBe(17)
    expect(term.fit).toHaveBeenCalledTimes(1)
    // The fit must observe the already-applied font size.
    expect(term.fontSizeAtFit).toEqual([17])
  })

  it('font application on a parked iframe sets the font but never fits', async () => {
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 800 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 600 })

    act(() => { harness.release() })
    // Stub planted post-release: jsdom recreates contentWindow on reparent.
    const term = installTermStub(iframe)
    act(() => { harness.pool!.applyFontSize('alice:smooth-scroll', 18) })
    expect(term.options.fontSize).toBe(18)
    expect(term.fit).not.toHaveBeenCalled()
  })

  it('refits claimed iframes when the document becomes visible again', async () => {
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
      const term = installTermStub(iframe)

      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
      fireEvent(document, new Event('visibilitychange'))
      expect(term.fit).not.toHaveBeenCalled()

      Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
      fireEvent(document, new Event('visibilitychange'))
      expect(term.fit).toHaveBeenCalledTimes(1)
    } finally {
      if (originalVisibility) {
        Object.defineProperty(document, 'visibilityState', originalVisibility)
      }
    }
  })

  it('reparents with moveBefore when the container supports it, moving BEFORE restyling', async () => {
    // appendChild reparenting reloads the iframe document in current Chrome
    // (drops the ttyd WebSocket); moveBefore is the state-preserving move.
    // Evidence: /srv/data/chrote/evidence/fit-probe-20260803/result2/3.txt
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()

    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 760 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 520 })

    act(() => { harness.release() })
    await waitFor(() => expect(iframe.style.width).toBe('760px'))

    const widthAtMove: string[] = []
    const target = harness.container
    ;(target as HTMLElement & { moveBefore?: (n: Node, ref: Node | null) => void }).moveBefore =
      vi.fn((node: Node, ref: Node | null) => {
        widthAtMove.push((node as HTMLIFrameElement).style.width)
        target.insertBefore(node, ref)
      })

    act(() => { harness.claim() })
    expect((target as HTMLElement & { moveBefore?: unknown }).moveBefore).toHaveBeenCalledTimes(1)
    expect(iframe.parentElement).toBe(target)
    // Park styles must still be in place at move time; restyle happens after
    // the move so a style change never lays out inside the pool.
    expect(widthAtMove).toEqual(['760px'])
    expect(iframe.style.width).toBe('100%')
  })

  it('parks a hidden iframe at its last fitted size instead of the fixed pool size', async () => {
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 760 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 520 })
    const term = installTermStub(iframe)

    act(() => { harness.pool!.triggerFit('alice:smooth-scroll') })
    expect(term.fit).toHaveBeenCalledTimes(1)

    iframe.style.display = 'none'
    Object.defineProperty(iframe, 'offsetWidth', { configurable: true, value: 0 })
    Object.defineProperty(iframe, 'offsetHeight', { configurable: true, value: 0 })
    act(() => { harness.release() })

    expect(iframe.style.width).toBe('760px')
    expect(iframe.style.height).toBe('520px')
  })

  it('falls back to appendChild when moveBefore is unavailable', async () => {
    const harness = renderPoolHarness()
    const iframe = await harness.claimedIframe()

    act(() => { harness.release() })
    act(() => { harness.claim() })
    expect(iframe.parentElement).toBe(harness.container)
    expect(iframe.style.width).toBe('100%')
  })

  it('routes the iframe load event through font-then-fit', async () => {
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
    const term = installTermStub(iframe)

    fireEvent(iframe, new Event('load'))
    await waitFor(() => expect(term.fit).toHaveBeenCalled())
    expect(term.options.fontSize).toBe(14)
    expect(term.fontSizeAtFit[term.fontSizeAtFit.length - 1]).toBe(14)
  })
})
