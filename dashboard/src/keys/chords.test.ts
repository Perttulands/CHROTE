import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  LEADER_LABEL,
  LEADER_WINDOW_MS,
  registerChords,
  resetChordsForTest,
  setActiveScopes,
  setKeysEnabled,
  terminalKeyEvent,
  useLeader,
  type Chord,
} from './chords'

const LEADER: KeyboardEventInit = { key: ' ', code: 'Space', ctrlKey: true, shiftKey: true }

function keydown(init: KeyboardEventInit): KeyboardEvent {
  return new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init })
}

/** Send a key the way the browser does: through the document, capture first. */
function press(init: KeyboardEventInit): KeyboardEvent {
  const event = keydown(init)
  act(() => { document.body.dispatchEvent(event) })
  return event
}

/** Offer a key to the xterm handler, as a focused terminal does. */
function offerToTerminal(init: KeyboardEventInit): boolean {
  let handled = false
  act(() => { handled = terminalKeyEvent(keydown(init)) })
  return handled
}

function chord(overrides: Partial<Chord> & Pick<Chord, 'id' | 'key' | 'scope'>): Chord {
  return { label: overrides.id, run: () => {}, ...overrides }
}

describe('chord registry', () => {
  beforeEach(() => {
    resetChordsForTest()
  })

  afterEach(() => {
    resetChordsForTest()
    vi.useRealTimers()
  })

  it('lists only what the active scopes offer, and every chord for the panel', () => {
    const { result } = renderHook(() => useLeader())
    act(() => {
      registerChords([
        chord({ id: 'anywhere', key: 'b', scope: 'global' }),
        chord({ id: 'in-workspace', key: '=', scope: 'workspace' }),
        chord({ id: 'on-tile', key: 'p', scope: 'tile' }),
      ])
    })

    expect(result.current.scopeChords.map(c => c.id)).toEqual(['anywhere'])
    expect(result.current.allChords.map(c => c.id)).toEqual(['anywhere', 'in-workspace', 'on-tile'])

    act(() => setActiveScopes({ workspace: true, tile: false }))
    expect(result.current.scopeChords.map(c => c.id)).toEqual(['anywhere', 'in-workspace'])

    act(() => setActiveScopes({ workspace: true, tile: true }))
    expect(result.current.scopeChords.map(c => c.id)).toEqual(['anywhere', 'in-workspace', 'on-tile'])
  })

  it('retires a registration when its owner unregisters', () => {
    const { result } = renderHook(() => useLeader())
    let retire = () => {}
    act(() => { retire = registerChords([chord({ id: 'anywhere', key: 'b', scope: 'global' })]) })
    expect(result.current.allChords).toHaveLength(1)

    act(() => retire())
    expect(result.current.allChords).toHaveLength(0)
  })

  it('runs a scoped chord only while its scope is active', () => {
    const run = vi.fn()
    act(() => { registerChords([chord({ id: 'tile-only', key: 'p', scope: 'tile', run })]) })

    press(LEADER)
    press({ key: 'p' })
    expect(run).not.toHaveBeenCalled()

    act(() => setActiveScopes({ workspace: true, tile: true }))
    press(LEADER)
    press({ key: 'p' })
    expect(run).toHaveBeenCalledTimes(1)
  })
})

describe('the leader window', () => {
  const run = vi.fn()

  beforeEach(() => {
    resetChordsForTest()
    run.mockClear()
    act(() => {
      registerChords([
        chord({ id: 'window-2', key: '2', scope: 'global', run }),
        chord({ id: 'keys-panel', key: '?', scope: 'global', run }),
      ])
    })
  })

  afterEach(() => {
    resetChordsForTest()
    vi.useRealTimers()
  })

  it('opens on the leader, echoes it, and takes the key from everyone else', () => {
    const { result } = renderHook(() => useLeader())

    const leader = press(LEADER)

    expect(result.current.leaderOpen).toBe(true)
    expect(result.current.pressed).toEqual([LEADER_LABEL])
    expect(leader.defaultPrevented).toBe(true)
  })

  it('closes on the next key and runs its chord', () => {
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    const next = press({ key: '2' })

    expect(run).toHaveBeenCalledTimes(1)
    expect(result.current.leaderOpen).toBe(false)
    expect(next.defaultPrevented).toBe(true)
  })

  it('cancels on Escape without running anything', () => {
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    press({ key: 'Escape' })

    expect(run).not.toHaveBeenCalled()
    expect(result.current.leaderOpen).toBe(false)
  })

  it('closes on its own after the window elapses', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    act(() => { vi.advanceTimersByTime(LEADER_WINDOW_MS - 1) })
    expect(result.current.leaderOpen).toBe(true)

    act(() => { vi.advanceTimersByTime(1) })
    expect(result.current.leaderOpen).toBe(false)
    expect(run).not.toHaveBeenCalled()
  })

  // Reaching ? or S means pressing Shift, whose own keydown arrives first.
  it('does not treat a modifier as the next key', () => {
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    const shift = press({ key: 'Shift', shiftKey: true })

    expect(result.current.leaderOpen).toBe(true)
    expect(shift.defaultPrevented).toBe(false)

    press({ key: '?', shiftKey: true })
    expect(run).toHaveBeenCalledTimes(1)
  })

  it('answers one event once, however many listeners see it', () => {
    const { result } = renderHook(() => useLeader())

    const leader = keydown(LEADER)
    act(() => { document.body.dispatchEvent(leader) })
    // The same event offered again, as the xterm handler would see it.
    act(() => { expect(terminalKeyEvent(leader)).toBe(false) })

    expect(result.current.leaderOpen).toBe(true)
    expect(result.current.pressed).toEqual([LEADER_LABEL])
  })
})

describe('the terminal handler', () => {
  const run = vi.fn()

  beforeEach(() => {
    resetChordsForTest()
    run.mockClear()
    act(() => { registerChords([chord({ id: 'window-2', key: '2', scope: 'global', run })]) })
  })

  afterEach(() => resetChordsForTest())

  it('keeps the leader and the chord that follows it out of the pty', () => {
    // false is xterm's "do not handle this key", so nothing is written and
    // nothing is sent on the socket.
    expect(offerToTerminal(LEADER)).toBe(false)
    expect(offerToTerminal({ key: '2' })).toBe(false)
    expect(run).toHaveBeenCalledTimes(1)
  })

  it('hands every ordinary key to the shell', () => {
    expect(offerToTerminal({ key: '2' })).toBe(true)
    expect(offerToTerminal({ key: 'a' })).toBe(true)
    expect(offerToTerminal({ key: ' ', code: 'Space', ctrlKey: true })).toBe(true)
    expect(run).not.toHaveBeenCalled()
  })

  it('intercepts nothing anywhere while keys are off', () => {
    const { result } = renderHook(() => useLeader())
    act(() => setKeysEnabled(false))

    expect(offerToTerminal(LEADER)).toBe(true)
    const leader = press(LEADER)

    expect(leader.defaultPrevented).toBe(false)
    expect(result.current.leaderOpen).toBe(false)
    expect(result.current.keysEnabled).toBe(false)

    act(() => setKeysEnabled(true))
    expect(offerToTerminal(LEADER)).toBe(false)
  })

  it('shuts an open window when keys are turned off underneath it', () => {
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    act(() => setKeysEnabled(false))

    expect(result.current.leaderOpen).toBe(false)
    expect(offerToTerminal({ key: '2' })).toBe(true)
    expect(run).not.toHaveBeenCalled()
  })
})
