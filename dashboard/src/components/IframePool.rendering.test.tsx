import { render, waitFor } from '@testing-library/react'
import { useEffect, useRef } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

  return <div data-testid="iframe-target" ref={targetRef} />
}

describe('IframePool attached terminal rendering', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
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
})
