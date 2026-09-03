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

  it('runs one request at a time, coalescing every trigger and settling every caller', async () => {
    vi.useFakeTimers()
    const { requests } = stubDeferredSessionFetch()
    const { result, unmount } = renderSession()

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

    // A refresh asked for while the trailing flight is out cannot be answered by
    // it: only the GET after that one can have seen the change.
    let duringTrailingSettled = false
    let duringTrailing!: Promise<void>
    act(() => {
      duringTrailing = result.current.refreshSessions()
      void duringTrailing.then(() => { duringTrailingSettled = true })
    })
    expect(requests).toHaveLength(3)
    expect(duringTrailingSettled).toBe(false)

    await act(async () => {
      requests[2].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await Promise.resolve()
    })
    expect(requests).toHaveLength(4)
    expect(duringTrailingSettled).toBe(false)

    await act(async () => {
      requests[3].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
      await duringTrailing
    })
    expect(duringTrailingSettled).toBe(true)
    expect(requests).toHaveLength(4)

    // Unmounting aborts the flight in the air, and every caller still waiting on
    // it is settled rather than left holding a promise nothing will resolve.
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    let settledCallers = 0
    act(() => {
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
      void result.current.refreshSessions().then(() => { settledCallers += 1 })
    })
    expect(requests).toHaveLength(5)

    unmount()
    expect(requests[4].signal?.aborted).toBe(true)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(settledCallers).toBe(2)
    expect(requests).toHaveLength(5)
    expect(vi.getTimerCount()).toBe(0)
    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
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

  it('reports a mounted timeout while preserving known state, bindings, and modals', async () => {
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
      result.current.openSendToSession({ targetSessionKey: 'alice:known' })
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
      sendToSessionRequest: { targetSessionKey: 'alice:known' },
      loading: false,
      error: 'Failed to fetch sessions (request timed out)',
    })
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:known'],
      activeSession: 'alice:known',
    })
    expect(vi.getTimerCount()).toBe(1)

    // The session is gone for good, and the binding still holds: a poll reports
    // what tmux lists, it does not revoke what the operator asked for.
    for (const requestIndex of [2, 3]) {
      const authoritativeMissing = result.current.refreshSessions()
      await act(async () => {
        requests[requestIndex].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
        await authoritativeMissing
      })
    }
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:known'],
      activeSession: 'alice:known',
    })
    expect(result.current.floatingSession).toBe('alice:known')
    expect(result.current.sendToSessionRequest?.targetSessionKey).toBe('alice:known')

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('preserves a bound failed-user session while applying healthy-user changes across repeated partial refreshes', async () => {
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
                boundSessions: ['alice:old', 'build:worker'],
                activeSession: 'build:worker',
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
    const oldHealthySession = { name: 'old', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }
    const failedUserSession = { name: 'worker', windows: 1, attached: false, group: 'agents', unixUser: 'build' }
    const newHealthySession = { name: 'new', windows: 1, attached: false, group: 'shell', unixUser: 'alice' }

    await act(async () => {
      requests[0].response.resolve(sessionResponse({
        sessions: [oldHealthySession, failedUserSession],
        grouped: { shell: [oldHealthySession], agents: [failedUserSession] },
        terminalUsers: ['alice', 'build'],
      }))
      await flushPromises()
    })
    act(() => {
      result.current.openFloatingModal('build:worker')
      result.current.openSendToSession({ targetSessionKey: 'build:worker' })
    })

    for (const requestIndex of [1, 2]) {
      const partialRefresh = result.current.refreshSessions()
      await act(async () => {
        requests[requestIndex].response.resolve(sessionResponse({
          partial: true,
          successfulUsers: ['alice'],
          failedUsers: ['build'],
          error: 'build: tmux source permission denied',
          sessions: [newHealthySession],
          grouped: { shell: [newHealthySession] },
          terminalUsers: ['alice', 'build'],
        }))
        await partialRefresh
      })
    }

    expect(result.current.sessions).toEqual([newHealthySession, failedUserSession])
    expect(result.current.groupedSessions).toEqual({ shell: [newHealthySession], agents: [failedUserSession] })
    expect(result.current.terminalUsers).toEqual(['alice', 'build'])
    expect(result.current.error).toBe('build: tmux source permission denied')
    // The half of the response that is trustworthy is named, so a consumer can
    // still join 'alice' bindings instead of discarding the whole list.
    expect(result.current.partialAnsweringUsers).toEqual(['alice'])
    expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
      boundSessions: ['alice:old', 'build:worker'],
      activeSession: 'build:worker',
    })
    expect(result.current.floatingSession).toBe('build:worker')
    expect(result.current.sendToSessionRequest?.targetSessionKey).toBe('build:worker')

    // A poll that fails outright says nothing about any user, so the partial
    // verdict must not outlive it and be joined against again.
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const failed = result.current.refreshSessions()
    await act(async () => {
      requests[3].response.reject(new Error('network down'))
      await failed
    })
    expect(result.current.partialAnsweringUsers).toBeNull()
    expect(result.current.sessions).toEqual([newHealthySession, failedUserSession])
    consoleError.mockRestore()

    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it('preserves authoritative state across failed, non-ok, and aborted refreshes', async () => {
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
      result.current.openSendToSession({ targetSessionKey: 'alice:stale' })
      result.current.addSessionToWindow('terminal1', 'terminal1-window-0', 'protected', 'alice')
    })

    const knownState = {
      sessions: result.current.sessions,
      groupedSessions: result.current.groupedSessions,
      terminalUsers: result.current.terminalUsers,
      floatingSession: result.current.floatingSession,
      sendToSessionRequest: result.current.sendToSessionRequest,
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

    // Four clean polls that list neither binding, and both still hold.
    for (const requestIndex of [4, 5, 6, 7]) {
      const missing = result.current.refreshSessions()
      await act(async () => {
        requests[requestIndex].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: ['alice'] }))
        await missing
      })
    }
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['alice:stale', 'alice:protected'])
    expect(result.current.floatingSession).toBe('alice:stale')
    expect(result.current.sendToSessionRequest?.targetSessionKey).toBe('alice:stale')

    consoleError.mockRestore()
    vi.useRealTimers()
  })

  it.each(['invalid JSON', 'a non-aborted network rejection'] as const)(
    'preserves all authoritative state after %s',
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
        result.current.openSendToSession({ targetSessionKey: 'alice:known' })
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
        sendToSessionRequest: { targetSessionKey: 'alice:known' },
      })
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })

      for (const requestIndex of [2, 3]) {
        const missing = result.current.refreshSessions()
        await act(async () => {
          requests[requestIndex].response.resolve(sessionResponse({ sessions: [], grouped: {}, terminalUsers: [] }))
          await missing
        })
      }
      expect(result.current.workspaces.terminal1.windows[0]).toMatchObject({
        boundSessions: ['alice:known'],
        activeSession: 'alice:known',
      })
      expect(result.current.floatingSession).toBe('alice:known')
      expect(result.current.sendToSessionRequest?.targetSessionKey).toBe('alice:known')

      unmount()
      expect(vi.getTimerCount()).toBe(0)
      consoleError.mockRestore()
      vi.useRealTimers()
    },
  )

  it('keeps every binding, and the tile the operator was reading, across repeated successful refreshes', async () => {
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

    await act(async () => {
      await result.current.refreshSessions()
    })

    // The headline regression: nothing is unbound, and no other session is
    // promoted into the frame the operator was reading.
    expect(result.current.workspaces.terminal1.windows[0].boundSessions)
      .toEqual(['alice:alive', 'alice:gone', 'legacy-live', 'legacy-gone'])
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('alice:gone')
    expect(result.current.workspaces.terminal2.windows[0].boundSessions)
      .toEqual(['build:agent-live', 'build:agent-dead'])
    expect(result.current.workspaces.terminal2.windows[0].activeSession).toBe('build:agent-dead')
  })

  it('forgets nothing on a non-ok poll, whether or not one has ever succeeded', async () => {
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

    const { result, unmount } = renderSession()

    // Nothing has ever succeeded here, so there is no live list to reconcile
    // against. Sweeping on that would unbind a session that is very likely alive.
    await waitFor(() => expect(result.current.error).toBe('tmux server not running'))
    expect(result.current.workspaces.terminal1.windows[0].boundSessions).toEqual(['probably-live'])
    expect(result.current.workspaces.terminal1.windows[0].activeSession).toBe('probably-live')
    unmount()

    // And once a poll has succeeded, a later non-ok one does not undo it.
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
    const settled = renderSession()

    await waitFor(() => expect(settled.result.current.sessions).toEqual(existingSessions))
    fetchMock.mockImplementationOnce(() => Promise.resolve({
      ok: false,
      json: () => Promise.resolve({ error: 'tmux server not running' }),
      text: () => Promise.resolve('{"error":"tmux server not running"}'),
    }))

    await act(async () => {
      await settled.result.current.refreshSessions()
    })

    expect(settled.result.current.error).toBe('tmux server not running')
    expect(settled.result.current.sessions).toEqual(existingSessions)
    expect(settled.result.current.groupedSessions).toEqual(existingGrouped)
  })
})
