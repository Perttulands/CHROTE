import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createElement, StrictMode } from 'react'
import { DEFAULT_SETTINGS } from '../types'
import { useSession } from './SessionContext'
import {
  Wrapper,
  deferred,
  flushPromises,
  renderSession,
  sessionResponse,
  setViewportWidth,
  stubDeferredSessionFetch,
} from './SessionContext.test.support'

describe('refreshSessions', () => {
  beforeEach(() => {
    localStorage.clear()
    setViewportWidth(1280)
    vi.useRealTimers()
  })

  it('runs one request at a time and coalesces interval, manual, create, delete, and rename triggers', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result } = renderSession()

    expect(requests).toHaveLength(1)
    const refreshSessions = result.current.refreshSessions
    let coalescedSettled = false

    act(() => {
      void result.current.refreshSessions().then(() => { coalescedSettled = true })
      void result.current.refreshSessions()
      vi.advanceTimersByTime(DEFAULT_SETTINGS.autoRefreshInterval)
    })
    await act(async () => {
      await Promise.all([
        result.current.createSession({ name: 'created' }),
        result.current.deleteSession('deleted'),
        result.current.renameSession('old', 'renamed'),
      ])
    })

    expect(result.current.refreshSessions).toBe(refreshSessions)
    expect(requests).toHaveLength(1)

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)
    expect(coalescedSettled).toBe(false)

    act(() => {
      void result.current.refreshSessions()
      void result.current.refreshSessions()
      vi.advanceTimersByTime(DEFAULT_SETTINGS.autoRefreshInterval)
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    expect(coalescedSettled).toBe(true)

    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    vi.useRealTimers()
  })

  it.each([
    ['createSession', 'POST', '/api/tmux/sessions'],
    ['deleteSession', 'DELETE', '/api/tmux/sessions/deleted'],
    ['renameSession', 'PATCH', '/api/tmux/sessions/old'],
  ] as const)('%s alone requests one trailing GET while the initial GET is unresolved', async (actionName, method, url) => {
    vi.useFakeTimers()
    const { fetchMock, requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()

    expect(requests).toHaveLength(1)

    await act(async () => {
      if (actionName === 'createSession') {
        expect(await result.current.createSession({ name: 'created' })).toBe('created')
      } else if (actionName === 'deleteSession') {
        expect(await result.current.deleteSession('deleted')).toBe(true)
      } else {
        expect(await result.current.renameSession('old', 'renamed')).toBe(true)
      }
    })

    expect(fetchMock.mock.calls.filter(([input, init]) => String(input) === url && init?.method === method)).toHaveLength(1)
    expect(requests).toHaveLength(1)

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('keeps a refresh requested during the trailing flight pending through one further coalesced GET', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()

    expect(requests).toHaveLength(1)
    act(() => {
      void result.current.refreshSessions()
    })

    await act(async () => {
      requests[0].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(2)

    let refreshSettled = false
    let refreshDuringTrailing!: Promise<void>
    act(() => {
      refreshDuringTrailing = result.current.refreshSessions()
      void refreshDuringTrailing.then(() => { refreshSettled = true })
    })
    expect(requests).toHaveLength(2)
    expect(refreshSettled).toBe(false)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(3)
    expect(refreshSettled).toBe(false)

    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await refreshDuringTrailing
    })
    expect(refreshSettled).toBe(true)
    expect(requests).toHaveLength(3)

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it.each(['fetch', 'response.json'] as const)(
    'times out when %s ignores abort, starts the queued trailing refresh, and remains reusable',
    async stalledStage => {
      vi.useFakeTimers()
      const { requests } = stubDeferredSessionFetch()
      const { result, unmount } = renderSession()

      expect(requests).toHaveLength(1)
      if (stalledStage === 'response.json') {
        const neverSettlingBody = new Promise<never>(() => {})
        const stalledJson = vi.fn(() => neverSettlingBody)
        await act(async () => {
          requests[0].response.resolve({
            ok: true,
            json: stalledJson,
            text: vi.fn(() => Promise.resolve('')),
          })
          await flushPromises()
        })
        expect(stalledJson).toHaveBeenCalledOnce()
      }

      let queuedSettled = false
      let queuedRefresh!: Promise<void>
      act(() => {
        queuedRefresh = result.current.refreshSessions()
        void queuedRefresh.then(() => { queuedSettled = true })
      })

      await act(async () => {
        vi.advanceTimersByTime(10000)
        await Promise.resolve()
      })

      expect(requests[0].signal?.aborted).toBe(true)
      expect(requests).toHaveLength(2)
      expect(queuedSettled).toBe(false)
      expect(result.current.loading).toBe(false)
      // One polling interval plus the trailing flight timeout: the abandoned flight timer is gone.
      expect(vi.getTimerCount()).toBe(2)

      await act(async () => {
        requests[1].response.resolve(sessionResponse({
          sessions: [{ name: 'trailing' }],
          grouped: {},
          terminalUsers: [],
        }))
        await queuedRefresh
      })
      expect(queuedSettled).toBe(true)
      expect(result.current.sessions).toEqual([{ name: 'trailing' }])
      expect(vi.getTimerCount()).toBe(1)

      const laterRefresh = result.current.refreshSessions()
      expect(requests).toHaveLength(3)
      await act(async () => {
        requests[2].response.resolve(sessionResponse({
          sessions: [{ name: 'later' }],
          grouped: {},
          terminalUsers: [],
        }))
        await laterRefresh
      })
      expect(result.current.sessions).toEqual([{ name: 'later' }])
      expect(result.current.loading).toBe(false)
      expect(vi.getTimerCount()).toBe(1)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      vi.useRealTimers()
    },
  )

  it('ignores an eventual rejection from an abandoned response body after a newer refresh succeeds', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {},
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const abandonedBody = deferred<Record<string, unknown>>()
    const abandonedJson = vi.fn(() => abandonedBody.promise)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    await act(async () => {
      requests[0].response.resolve({
        ok: true,
        json: abandonedJson,
        text: vi.fn(() => Promise.resolve('')),
      })
      await flushPromises()
    })
    expect(abandonedJson).toHaveBeenCalledOnce()
    await act(async () => {
      vi.advanceTimersByTime(10000)
      await flushPromises()
    })

    let newerSettled = false
    const newerRefresh = result.current.refreshSessions()
    void newerRefresh.then(() => { newerSettled = true })
    expect(requests).toHaveLength(2)
    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        sessions: [{ name: 'newer' }],
        grouped: {},
        terminalUsers: [],
      }))
      await flushPromises()
    })
    expect(newerSettled).toBe(true)

    await act(async () => {
      abandonedBody.reject(new Error('late body failure'))
      await Promise.resolve()
    })
    expect(result.current.sessions).toEqual([{ name: 'newer' }])
    expect(result.current.error).toBeNull()
    expect(consoleError).not.toHaveBeenCalled()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('settles coalesced callers on unmount without resolving an abort-ignoring fetch or honoring trailing intent', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    let settledCallers = 0

    act(() => {
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
    })
    expect(requests).toHaveLength(1)

    unmount()
    expect(requests[0].signal?.aborted).toBe(true)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(settledCallers).toBe(2)
    expect(requests).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('keeps StrictMode generation cancellation silent and ignores its eventual completion', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = ({ children }: { children: React.ReactNode }) => createElement(
      StrictMode,
      null,
      createElement(Wrapper, null, children),
    )
    const { result, unmount } = renderHook(() => useSession(), { wrapper })

    expect(requests).toHaveLength(2)
    expect(requests[0].signal?.aborted).toBe(true)
    expect(requests[1].signal?.aborted).toBe(false)

    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        sessions: [{ name: 'current-generation' }],
        grouped: {},
        terminalUsers: [],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([{ name: 'current-generation' }])
    expect(result.current.error).toBeNull()

    const abandonedResponse = sessionResponse({ sessions: [{ name: 'abandoned-generation' }] })
    await act(async () => {
      requests[0].response.resolve(abandonedResponse)
      await flushPromises()
    })
    expect(abandonedResponse.json).not.toHaveBeenCalled()
    expect(result.current.sessions).toEqual([{ name: 'current-generation' }])
    expect(result.current.error).toBeNull()
    expect(consoleError).not.toHaveBeenCalled()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it('reports a mounted timeout while preserving known state, bindings, modals, and stale confirmation', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [{
                id: 'terminal1-window-0',
                boundSessions: ['alice:known'],
                activeSession: 'alice:known',
                colorIndex: 0,
              }],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const knownGrouped = { shell: [knownSession] }

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [knownSession],
        grouped: knownGrouped,
        terminalUsers: ['alice'],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([knownSession])
    act(() => {
      result.current.openFloatingModal('alice:known')
      result.current.openSendToSession('alice:known')
    })

    let timeoutSettled = false
    let timedOutRefresh!: Promise<void>
    act(() => {
      timedOutRefresh = result.current.refreshSessions()
      void timedOutRefresh.then(() => { timeoutSettled = true })
    })
    expect(requests).toHaveLength(2)

    await act(async () => {
      vi.advanceTimersByTime(10000)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(requests[1].signal?.aborted).toBe(true)
    expect(timeoutSettled).toBe(true)
    await timedOutRefresh
    expect(requests).toHaveLength(2)
    expect(result.current).toMatchObject({
      sessions: [knownSession],
      groupedSessions: knownGrouped,
      terminalUsers: ['alice'],
      floatingSession: 'alice:known',
      sendToSessionTarget: 'alice:known',
      loading: false,
      error: 'Failed to fetch sessions (request timed out)',
    })
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:known'],
      activeSession: 'alice:known',
    })
    expect(vi.getTimerCount()).toBe(1)

    const firstAuthoritativeMissing = result.current.refreshSessions()
    expect(requests).toHaveLength(3)
    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await firstAuthoritativeMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:known'])
    expect(result.current.floatingSession).toBe('alice:known')
    expect(result.current.sendToSessionTarget).toBe('alice:known')

    const secondAuthoritativeMissing = result.current.refreshSessions()
    await act(async () => {
      requests[3].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
      await secondAuthoritativeMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])
    expect(result.current.floatingSession).toBeNull()
    expect(result.current.sendToSessionTarget).toBeNull()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('replaces last-known-good state with healthy sessions from an authoritative partial refresh and keeps the per-user error visible', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()
    const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const healthySession = { name: 'healthy', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [knownSession],
        grouped: { shell: [knownSession] },
        terminalUsers: ['alice'],
      }))
      await flushPromises()
    })
    expect(result.current.sessions).toEqual([knownSession])

    const partialRefresh = result.current.refreshSessions()
    await act(async () => {
      requests[1].response.resolve(sessionResponse({
        partial: true,
        error: 'build: error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)',
        sessions: [healthySession],
        grouped: { shell: [healthySession] },
        terminalUsers: ['alice', 'build'],
      }))
      await partialRefresh
    })

    expect(result.current.sessions).toEqual([healthySession])
    expect(result.current.groupedSessions).toEqual({ shell: [healthySession] })
    expect(result.current.terminalUsers).toEqual(['alice', 'build'])
    expect(result.current.error).toBe('build: error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)')

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('preserves authoritative state and stale confirmation across failed, non-ok, and aborted refreshes', async () => {
    vi.useFakeTimers()
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [{
                id: 'terminal1-window-0',
                boundSessions: ['alice:stale'],
                activeSession: 'alice:stale',
                colorIndex: 0,
              }],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
    }))
    const { requests } = stubDeferredSessionFetch()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = renderSession()
    const staleSession = { name: 'stale', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [staleSession],
        grouped: { shell: [staleSession] },
        terminalUsers: ['alice'],
      }))
      await Promise.resolve()
    })
    act(() => {
      result.current.openFloatingModal('alice:stale')
      result.current.openSendToSession('alice:stale')
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'protected', 'alice')
    })

    const knownState = {
      sessions: result.current.sessions,
      groupedSessions: result.current.groupedSessions,
      terminalUsers: result.current.terminalUsers,
      floatingSession: result.current.floatingSession,
      sendToSessionTarget: result.current.sendToSessionTarget,
    }
    const failedPayload = {
      error: 'tmux unavailable',
      sessions: [{ name: 'not-authoritative' }],
      grouped: { bad: [{ name: 'not-authoritative' }] },
      terminalUsers: ['not-authoritative'],
    }

    const nonOk = result.current.refreshSessions()
    await act(async () => {
      requests[1].response.resolve(sessionResponse(failedPayload, false))
      await nonOk
    })
    expect(result.current).toMatchObject(knownState)

    const failed = result.current.refreshSessions()
    await act(async () => {
      requests[2].response.resolve(sessionResponse(failedPayload))
      await failed
    })
    expect(result.current).toMatchObject(knownState)

    const aborted = result.current.refreshSessions()
    expect(requests[3].signal?.aborted).toBe(false)
    act(() => vi.advanceTimersByTime(10000))
    expect(requests[3].signal?.aborted).toBe(true)
    await act(async () => {
      requests[3].response.reject(new DOMException('Aborted', 'AbortError'))
      await aborted
    })
    expect(result.current).toMatchObject(knownState)
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])

    const firstMissing = result.current.refreshSessions()
    await act(async () => {
      requests[4].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await firstMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])
    expect(result.current.floatingSession).toBe('alice:stale')
    expect(result.current.sendToSessionTarget).toBe('alice:stale')

    const secondMissing = result.current.refreshSessions()
    await act(async () => {
      requests[5].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await secondMissing
    })
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:protected'])
    expect(result.current.floatingSession).toBeNull()
    expect(result.current.sendToSessionTarget).toBeNull()

    for (const requestIndex of [6, 7]) {
      const refresh = result.current.refreshSessions()
      await act(async () => {
        requests[requestIndex].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
        await refresh
      })
    }
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual([])

    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it.each(['invalid JSON', 'a non-aborted network rejection'] as const)(
    'preserves all authoritative state and stale confirmation after %s',
    async failureMode => {
      vi.useFakeTimers()
      localStorage.setItem('chrote-dashboard-state', JSON.stringify({
        version: 3,
        settingsSchemaVersion: 2,
        layoutsByViewport: {
          desktop: {
            workspaces: {
              terminal1: {
                windows: [{
                  id: 'terminal1-window-0',
                  boundSessions: ['alice:known'],
                  activeSession: 'alice:known',
                  colorIndex: 0,
                }],
                windowCount: 1,
              },
            },
          },
        },
        sidebarCollapsed: false,
        settings: { ...DEFAULT_SETTINGS, autoRefreshInterval: 60000 },
      }))
      const { requests } = stubDeferredSessionFetch()
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
      const { result, unmount } = renderSession()
      const knownSession = { name: 'known', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
      const knownGrouped = { shell: [knownSession] }

      await act(async () => {
        requests[0].response.resolve(sessionResponse({
          sessions: [knownSession],
          grouped: knownGrouped,
          terminalUsers: ['alice'],
        }))
        await Promise.resolve()
      })
      act(() => {
        result.current.openFloatingModal('alice:known')
        result.current.openSendToSession('alice:known')
      })

      const failedRefresh = result.current.refreshSessions()
      expect(requests).toHaveLength(2)
      expect(requests[1].signal?.aborted).toBe(false)
      await act(async () => {
        if (failureMode === 'invalid JSON') {
          requests[1].response.resolve({
            ok: true,
            json: vi.fn(() => Promise.reject(new SyntaxError('Unexpected token'))),
            text: vi.fn(() => Promise.resolve('not json')),
          })
        } else {
          requests[1].response.reject(new TypeError('network unavailable'))
        }
        await failedRefresh
      })

      expect(result.current).toMatchObject({
        sessions: [knownSession],
        groupedSessions: knownGrouped,
        terminalUsers: ['alice'],
        floatingSession: 'alice:known',
        sendToSessionTarget: 'alice:known',
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })

      const firstMissing = result.current.refreshSessions()
      await act(async () => {
        requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
        await firstMissing
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })
      expect(result.current.floatingSession).toBe('alice:known')
      expect(result.current.sendToSessionTarget).toBe('alice:known')

      const secondMissing = result.current.refreshSessions()
      await act(async () => {
        requests[3].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
        await secondMissing
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: [],
        activeSession: null,
      })
      expect(result.current.floatingSession).toBeNull()
      expect(result.current.sendToSessionTarget).toBeNull()

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      consoleError.mockRestore()
      vi.useRealTimers()
    },
  )

  it('auto-removes stale terminal bindings after repeated successful session refreshes', async () => {
    const liveSessions = [
      { name: 'alive', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
      { name: 'legacy-live', windows: 1, attached: false, group: 'shell' },
      { name: 'agent-live', windows: 1, attached: false, group: 'agents', unixUser: 'build' },
    ]
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                {
                  id: 'terminal1-window-0',
                  boundSessions: ['alice:alive', 'alice:gone', 'legacy-live', 'legacy-gone'],
                  activeSession: 'alice:gone',
                  colorIndex: 0,
                },
              ],
              windowCount: 1,
            },
            terminal2: {
              windows: [
                {
                  id: 'terminal2-window-0',
                  boundSessions: ['build:agent-live', 'build:agent-dead'],
                  activeSession: 'build:agent-dead',
                  colorIndex: 0,
                },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))
    const fetchMock = vi.fn((): Promise<any> => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: liveSessions,
        grouped: { shell: liveSessions.slice(0, 2), agents: liveSessions.slice(2) },
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderSession()

    await waitFor(() => expect(result.current.sessions).toEqual(liveSessions))
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toContain('alice:gone')

    await act(async () => {
      await result.current.refreshSessions()
    })

    await waitFor(() => {
      expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:alive', 'legacy-live'])
      expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('alice:alive')
      expect(result.current.workspaces.terminal2.windows[0].boundSessions).toEqual(['build:agent-live'])
      expect(result.current.workspaces.terminal2.windows[0].activeSession).toBe('build:agent-live')
    })
  })

  it('preserves terminal bindings when a refresh fails instead of sweeping on uncertainty', async () => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify({
      version: 3,
      settingsSchemaVersion: 2,
      layoutsByViewport: {
        desktop: {
          workspaces: {
            terminal1: {
              windows: [
                { id: 'terminal1-window-0', boundSessions: ['probably-live'], activeSession: 'probably-live', colorIndex: 0 },
              ],
              windowCount: 1,
            },
          },
        },
      },
      sidebarCollapsed: false,
      settings: DEFAULT_SETTINGS,
    }))
    vi.stubGlobal('fetch', vi.fn((): Promise<any> => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux server not running' }),
      text: () => Promise.resolve('{"error":"tmux server not running"}'),
    })))

    const { result } = renderSession()

    await waitFor(() => expect(result.current.error).toBe('tmux server not running'))
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['probably-live'])
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('probably-live')
  })

  it('preserves existing sessions and groups when a poll returns a non-ok response', async () => {
    const existingSessions = [
      { name: 'shell1', windows: 1, attached: false, group: 'shell', unixUser: 'alice' },
    ]
    const existingGrouped = { shell: existingSessions }
    const fetchMock = vi.fn((): Promise<any> => Promise.resolve({
      ok: true,
      json: () => Promise.resolve({
        sessions: existingSessions,
        grouped: existingGrouped,
        terminalUsers: ['alice'],
        timestamp: new Date().toISOString(),
      }),
      text: () => Promise.resolve(''),
    }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderSession()

    await waitFor(() => expect(result.current.sessions).toEqual(existingSessions))
    fetchMock.mockImplementationOnce(() => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux server not running' }),
      text: () => Promise.resolve('{"error":"tmux server not running"}'),
    }))

    await act(async () => {
      await result.current.refreshSessions()
    })

    expect(result.current.error).toBe('tmux server not running')
    expect(result.current.sessions).toEqual(existingSessions)
    expect(result.current.groupedSessions).toEqual(existingGrouped)
  })
})
