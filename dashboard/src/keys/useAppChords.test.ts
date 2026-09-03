import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useAppChords } from './useAppChords'
import { resetChordsForTest } from './chords'

const state = vi.hoisted(() => ({
  floatingSession: null as string | null,
  openFloatingModal: vi.fn(),
  closeFloatingModal: vi.fn(),
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
    floatingSession: state.floatingSession,
    openFloatingModal: state.openFloatingModal,
    closeFloatingModal: state.closeFloatingModal,
    updateSettings: vi.fn(),
    setFocusedWindowKey: vi.fn(),
    setWindowCount: vi.fn(),
    openSendToSession: vi.fn(),
  }),
}))

function altP() {
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', altKey: true, bubbles: true, cancelable: true }))
}

describe('Alt+P', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.floatingSession = null
    resetChordsForTest()
  })
  afterEach(() => resetChordsForTest())

  it('opens Peek on the focused tile, closes it when pressed again, and switches it from another tile', () => {
    const { rerender } = renderHook(() => useAppChords({
      activeTab: 'terminal1',
      onTabChange: vi.fn(),
      onToggleSessionsPanel: vi.fn(),
      onOpenSessionsPanel: vi.fn(),
      onToggleKeysPanel: vi.fn(),
    }))

    altP()
    expect(state.openFloatingModal).toHaveBeenCalledWith('alice:main')
    expect(state.closeFloatingModal).not.toHaveBeenCalled()

    // Peek now shows the focused tile's session: the same chord closes it.
    state.floatingSession = 'alice:main'
    rerender()
    altP()
    expect(state.closeFloatingModal).toHaveBeenCalledTimes(1)
    expect(state.openFloatingModal).toHaveBeenCalledTimes(1)

    // Peek shows another session: the chord switches it to this tile's.
    state.floatingSession = 'alice:jack'
    rerender()
    altP()
    expect(state.openFloatingModal).toHaveBeenLastCalledWith('alice:main')
    expect(state.closeFloatingModal).toHaveBeenCalledTimes(1)
  })
})
