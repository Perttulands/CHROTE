import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, waitFor } from '@testing-library/react'
import type { SendToSessionOutcome } from '../types'
import {
  renderSession,
  renderSessionWithStatus,
  sessionResponse,
} from './SessionContext.test.support'

describe('sendToSession', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('resolves exact pane identities for the selected Unix user', async () => {
    const pane = { sessionId: '$7', pane: '%42', panePid: '222', serverPid: '9001', windowName: 'logs', currentCommand: 'tail', currentPath: '/srv/app', active: true }
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/panes')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, session: 'shell1', unixUser: 'alice', panes: [pane] }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let panes = null
    await act(async () => {
      panes = await result.current.listSessionPanes('shell1', 'alice')
    })

    expect(panes).toEqual([pane])
    expect(fetchMock).toHaveBeenCalledWith('/api/tmux/sessions/shell1/panes?unixUser=alice', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('rejects pane discovery from a non-empty Unix user when default scope was requested', async () => {
    const pane = { sessionId: '$7', pane: '%42', panePid: '222', serverPid: '9001', active: true }
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/panes')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, session: 'shell1', unixUser: 'alice', panes: [pane] }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({ sessions: [], terminalUsers: [] }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    fetchMock.mockClear()

    let panes: Awaited<ReturnType<typeof result.current.listSessionPanes>> = []
    await act(async () => {
      panes = await result.current.listSessionPanes('shell1')
    })

    expect(panes).toBeNull()
  })

  it('rejects send success from a non-empty Unix user when default scope was requested', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: false,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: true,
            warning: '',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({ sessions: [], terminalUsers: [] }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithStatus()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', { text: 'wrong scope', files: [], submit: false })
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.status.status?.message).toContain('Unexpected send response'))
  })

  it('keeps a marker-confirmed send delivered when post-send verification observes target exit', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: true,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'target changed before post-send verification',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'failed'
    await act(async () => {
      delivered = await result.current.sendToSession('shell1', {
        text: 'inspect this',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('sent')
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toBe('/api/tmux/sessions/shell1/send?unixUser=alice')
    const form = init?.body as FormData
    expect(form.get('text')).toBe('inspect this')
    expect(form.get('submit')).toBe('true')
    expect(form.get('pane')).toBe('%42')
    expect(form.get('sessionId')).toBe('$7')
    expect(form.get('panePid')).toBe('222')
    expect(form.get('serverPid')).toBe('9001')
  })

  it('reports a confirmed paste without a submit-key receipt as unknown and non-retryable', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: true,
            transport: 'pasted',
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'target changed after paste; submit key was not dispatched',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithStatus()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'pasted but not submitted',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.status.status?.message).toContain('submit key was not dispatched'))
  })

  it('treats an explicit unknown delivery outcome as non-retryable and tells the user to inspect', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({
            success: false,
            transport: 'unknown',
            retryable: false,
            deliveryConfirmed: false,
            session: 'shell1',
            sessionId: '$7',
            pane: '%42',
            panePid: '222',
            serverPid: '9001',
            unixUser: 'alice',
            submissionRequested: true,
            submitKeyDispatched: false,
            bufferCleaned: true,
            targetVerified: false,
            warning: 'tmux did not confirm whether delivery occurred; inspect the exact pane before retrying',
          }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithStatus()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'do not duplicate',
        files: [],
        submit: true,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.status.status?.message).toContain('inspect the exact pane before retrying'))
  })

  it('labels browser transport failures as unknown rather than safe-to-retry failures', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.reject(new Error('connection reset after request write'))
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithStatus()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'could already be pasted',
        files: [],
        submit: false,
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
    await waitFor(() => expect(result.current.status.status?.message).toContain('inspect the exact pane before retrying'))
  })

  it.each([
    {
      title: 'shows a validated pre-dispatch 400 as an ordinary failure',
      status: 400,
      body: JSON.stringify({ success: false, error: { code: 'BAD_REQUEST', message: 'text or files are required' }, timestamp: '2026-07-18T18:00:00Z' }),
      expected: 'text or files are required',
      unknown: false,
    },
    {
      title: 'treats a valid-envelope 500 as an unknown delivery outcome',
      status: 500,
      body: JSON.stringify({ success: false, error: { code: 'TMUX_ERROR', message: 'internal tmux failure' }, timestamp: '2026-07-18T18:00:00Z' }),
      expected: 'inspect the exact pane before retrying',
      unknown: true,
    },
    {
      title: 'treats a malformed 409 envelope as an unknown delivery outcome',
      status: 409,
      body: JSON.stringify({ error: { code: 'TARGET_CHANGED', message: 'target changed' } }),
      expected: 'inspect the exact pane before retrying',
      unknown: true,
    },
  ])('$title', async ({ status, body, expected, unknown }) => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: false,
          status,
          text: () => Promise.resolve(body),
          json: () => Promise.reject(new Error('not a success response')),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSessionWithStatus()
    await waitFor(() => expect(result.current.session.terminalUsers).toEqual(['alice']))

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.session.sendToSession('shell1', {
        text: 'classify this response',
        files: [],
        submit: false,
      }, 'alice')
    })

    expect(delivered).toBe(unknown ? 'unknown' : 'failed')
    await waitFor(() => expect(result.current.status.status?.message).toContain(expected))
    if (!unknown) {
      expect(result.current.status.status?.message).not.toContain('outcome is unknown')
    }
  })

  it('rejects a malformed 2xx transport response instead of claiming delivery', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      if (String(input).includes('/api/tmux/sessions/shell1/send')) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ success: true, transport: 'queued', pane: '%42' }),
          text: () => Promise.resolve(''),
        })
      }
      return Promise.resolve(sessionResponse({
        sessions: [{ name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }],
        terminalUsers: ['alice'],
      }))
    })
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()
    await waitFor(() => expect(result.current.terminalUsers).toEqual(['alice']))
    fetchMock.mockClear()

    let delivered: SendToSessionOutcome = 'sent'
    await act(async () => {
      delivered = await result.current.sendToSession('shell1', {
        text: 'do not overclaim',
        files: [],
        submit: false,
        pane: '%42',
        sessionId: '$7',
        panePid: '222',
        serverPid: '9001',
      }, 'alice')
    })

    expect(delivered).toBe('unknown')
  })
})
