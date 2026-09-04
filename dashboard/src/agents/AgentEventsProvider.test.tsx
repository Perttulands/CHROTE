import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentEventsProvider, useAgentEventMarks } from './AgentEventsProvider'
import type { AgentEvent, TmuxSession, UserSettings } from '../types'
import { DEFAULT_SETTINGS } from '../types'

/**
 * The provider's channels, driven from the session list the way the poll
 * drives them. The mark and its clearing on focus are proven in the browser,
 * where the tile is real; here it is what the device does at the moment an
 * event is news, and what it does not do for history.
 */

const mockState = vi.hoisted(() => ({
  sessions: [] as TmuxSession[],
  loading: true,
  settings: {} as UserSettings,
  focusedWindowKey: null as string | null,
  announce: vi.fn(),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    sessions: mockState.sessions,
    loading: mockState.loading,
    error: null,
    partialAnsweringUsers: null,
    focusedWindowKey: mockState.focusedWindowKey,
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions: ['chrote:builder'], activeSession: 'chrote:builder', colorIndex: 0 }],
      },
    },
    settings: mockState.settings,
  }),
}))

vi.mock('../context/StatusContext', () => ({
  useStatus: () => ({ announce: mockState.announce }),
}))

const playTone = vi.hoisted(() => vi.fn())
vi.mock('./tones', () => ({
  audioContext: () => ({ stub: true }),
  playTone,
}))

class StubNotification {
  static permission: NotificationPermission = 'granted'
  static made: Array<{ title: string; options?: NotificationOptions }> = []
  onclick: (() => void) | null = null
  constructor(title: string, options?: NotificationOptions) {
    StubNotification.made.push({ title, options })
  }
  close() {}
}

const builder = (lastEvent?: AgentEvent): TmuxSession => ({
  name: 'builder', windows: 1, attached: false, group: 'claude', unixUser: 'chrote', ...(lastEvent ? { lastEvent } : {}),
})

let marks: ReadonlySet<string> = new Set()
function Marks() {
  marks = useAgentEventMarks()
  return null
}

function renderProvider() {
  return render(<AgentEventsProvider><Marks /></AgentEventsProvider>)
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { value: hidden, configurable: true })
}

describe('AgentEventsProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockState.sessions = []
    mockState.loading = true
    mockState.settings = { ...DEFAULT_SETTINGS }
    mockState.focusedWindowKey = null
    StubNotification.made = []
    StubNotification.permission = 'granted'
    vi.stubGlobal('Notification', StubNotification)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, text: async () => '' }))
    setHidden(false)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('tells a new unseen event through the toast, the tone and a notification while the tab is hidden, and marks its session', () => {
    mockState.settings = { ...DEFAULT_SETTINGS, agentEventTones: true, agentEventNotifications: true }
    const view = renderProvider()

    // The first list is history: an unseen event in it tells nothing.
    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:00:00.000Z', seen: false, summary: 'Old news' })]
    mockState.loading = false
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(mockState.announce).not.toHaveBeenCalled()
    expect(marks.size).toBe(0)

    setHidden(true)
    mockState.sessions = [builder({ event: 'needs-input', time: '2026-09-03T10:05:00.000Z', seen: false, summary: 'Which branch?' })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    expect(mockState.announce).toHaveBeenCalledWith('builder needs input', 'success')
    expect(playTone).toHaveBeenCalledWith('needs-input', { stub: true })
    expect(StubNotification.made).toEqual([{ title: 'builder needs input', options: { body: 'Which branch?', tag: 'chrote-agent-event-chrote:builder' } }])
    expect([...marks]).toEqual(['chrote:builder'])
  })

  it('keeps the tone and the notification off by default, and the notification off while the tab is shown', () => {
    const view = renderProvider()
    mockState.sessions = [builder()]
    mockState.loading = false
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    setHidden(true)
    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:05:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(mockState.announce).toHaveBeenCalledWith('builder finished', 'success')
    expect(playTone).not.toHaveBeenCalled()
    expect(StubNotification.made).toEqual([])

    mockState.settings = { ...DEFAULT_SETTINGS, agentEventNotifications: true }
    setHidden(false)
    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:06:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(StubNotification.made).toEqual([])
  })

  it('marks nothing when the device has turned marks off, and marks again when it turns them back on', () => {
    mockState.settings = { ...DEFAULT_SETTINGS, agentEventMarks: false }
    const view = renderProvider()
    mockState.sessions = [builder()]
    mockState.loading = false
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:05:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(marks.size).toBe(0)
    // The setting silences the mark alone; the report still arrives.
    expect(mockState.announce).toHaveBeenCalledWith('builder finished', 'success')

    // Knowing the event and drawing it are separate, so the mark is there to
    // show the moment the operator turns it back on.
    mockState.settings = { ...DEFAULT_SETTINGS, agentEventMarks: true }
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect([...marks]).toEqual(['chrote:builder'])
  })

  it('keeps the toast off when the device has turned it off, leaving the status line the record', () => {
    mockState.settings = { ...DEFAULT_SETTINGS, agentEventToast: false }
    const view = renderProvider()
    mockState.sessions = [builder()]
    mockState.loading = false
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:05:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    // Severity is what the toast reads: information takes the line only.
    expect(mockState.announce).toHaveBeenCalledWith('builder finished', 'info')
    // The mark is a telling of its own and is untouched by this setting.
    expect([...marks]).toEqual(['chrote:builder'])
  })

  it('tells the server seen when the focused tile shows the session, once per event', () => {
    const view = renderProvider()
    mockState.sessions = [builder()]
    mockState.loading = false
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:05:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(marks.has('chrote:builder')).toBe(true)

    mockState.focusedWindowKey = 'terminal1-terminal1-window-0'
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))

    expect(marks.size).toBe(0)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith('/api/agent/event/seen', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ session: 'builder', unixUser: 'chrote' }),
    }))

    // The next poll has not caught up with the post yet: it is not posted again.
    mockState.sessions = [builder({ event: 'finished', time: '2026-09-03T10:05:00.000Z', seen: false })]
    act(() => view.rerender(<AgentEventsProvider><Marks /></AgentEventsProvider>))
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(marks.size).toBe(0)
  })
})
