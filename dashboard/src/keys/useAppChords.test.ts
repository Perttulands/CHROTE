import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useAppChords } from './useAppChords'
import { resetChordsForTest } from './chords'

const state = vi.hoisted(() => ({
  terminals: new Map<string, { claim: () => void }>(),
  connectionStates: new Map<string, string>(),
}))

vi.mock('../components/TerminalPool', () => ({
  useTerminalPool: () => ({ terminals: state.terminals, connectionStates: state.connectionStates }),
}))

vi.mock('../context/SessionContext', () => ({
  useSession: () => ({
    workspaceIds: ['terminal1', 'terminal2', 'terminal3'],
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['alice:main'], activeSession: 'alice:main', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['alice:jack'], activeSession: 'alice:jack', colorIndex: 1 },
        ],
      },
    },
    // The focus key is the workspace and the window's own id, which already
    // carries the workspace: this is what the tiles write.
    focusedWindowKey: 'terminal1-terminal1-window-0',
    settings: { keysEnabled: true },
    updateSettings: vi.fn(),
    setFocusedWindowKey: vi.fn(),
    setWindowCount: vi.fn(),
    openSendToSession: vi.fn(),
  }),
}))

describe('Beads chords', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetChordsForTest()
  })
  afterEach(() => resetChordsForTest())

  it('gives Alt+B to the global column and keeps leader then B on the tab', () => {
    const onTabChange = vi.fn()
    const onToggleBeadsColumn = vi.fn()
    renderHook(() => useAppChords({
      activeTab: 'terminal1',
      onTabChange,
      onToggleSessionsPanel: vi.fn(),
      onOpenSessionsPanel: vi.fn(),
      onToggleBeadsColumn,
      onToggleKeysPanel: vi.fn(),
      onOpenReport: vi.fn(),
    }))

    document.dispatchEvent(new KeyboardEvent('keydown', {
      key: 'b', altKey: true, bubbles: true, cancelable: true,
    }))
    expect(onToggleBeadsColumn).toHaveBeenCalledTimes(1)
    expect(onTabChange).not.toHaveBeenCalled()

    document.dispatchEvent(new KeyboardEvent('keydown', {
      key: ' ', code: 'Space', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true,
    }))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', bubbles: true, cancelable: true }))
    expect(onTabChange).toHaveBeenCalledWith('beads')
    expect(onToggleBeadsColumn).toHaveBeenCalledTimes(1)
  })
})

describe('Alt+C', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetChordsForTest()
  })
  afterEach(() => resetChordsForTest())

  it('claims every pooled terminal with an open connection and dials none of the others', () => {
    // A claim on a terminal without a connection would dial it at xterm's
    // default grid and size its session wrong.
    const open = { claim: vi.fn() }
    const openOnAnotherTab = { claim: vi.fn() }
    const neverShown = { claim: vi.fn() }
    const dropped = { claim: vi.fn() }
    state.terminals = new Map([
      ['alice:main', open],
      ['alice:other-tab', openOnAnotherTab],
      ['alice:never-shown', neverShown],
      ['alice:dropped', dropped],
    ])
    state.connectionStates = new Map([
      ['alice:main', 'open'],
      ['alice:other-tab', 'open'],
      ['alice:never-shown', 'idle'],
      ['alice:dropped', 'dropped'],
    ])
    renderHook(() => useAppChords({
      activeTab: 'beads',
      onTabChange: vi.fn(),
      onToggleSessionsPanel: vi.fn(),
      onOpenSessionsPanel: vi.fn(),
      onToggleBeadsColumn: vi.fn(),
      onToggleKeysPanel: vi.fn(),
      onOpenReport: vi.fn(),
    }))

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'c', altKey: true, bubbles: true, cancelable: true }))

    expect(open.claim).toHaveBeenCalledTimes(1)
    expect(openOnAnotherTab.claim).toHaveBeenCalledTimes(1)
    expect(neverShown.claim).not.toHaveBeenCalled()
    expect(dropped.claim).not.toHaveBeenCalled()
  })
})
