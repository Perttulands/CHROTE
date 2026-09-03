import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import {
  LEADER_LABEL,
  LEADER_WINDOW_MS,
  directChordLabel,
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

  // The window cycle needs both directions on one letter, so the bare leader
  // keys are `w` and `W` and the exact case has to win.
  it('prefers an exact key over a case-insensitive one', () => {
    const next = vi.fn()
    const previous = vi.fn()
    act(() => {
      registerChords([
        chord({ id: 'next', key: 'w', scope: 'global', run: next }),
        chord({ id: 'previous', key: 'W', scope: 'global', run: previous }),
      ])
    })

    press(LEADER)
    press({ key: 'w' })
    expect(next).toHaveBeenCalledTimes(1)
    expect(previous).not.toHaveBeenCalled()

    press(LEADER)
    press({ key: 'W', shiftKey: true })
    expect(previous).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledTimes(1)
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

  it('shuts without running anything on Escape, and on its own once the window elapses', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useLeader())

    press(LEADER)
    press({ key: 'Escape' })

    expect(run).not.toHaveBeenCalled()
    expect(result.current.leaderOpen).toBe(false)

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

  it('takes the leader and its chord out of the pty, and hands every other key to the shell', () => {
    // true is xterm's "xterm handles this key", so an ordinary key reaches the
    // shell; false is "do not handle this", so nothing is written or sent.
    expect(offerToTerminal({ key: '2' })).toBe(true)
    expect(offerToTerminal({ key: 'a' })).toBe(true)
    expect(offerToTerminal({ key: ' ', code: 'Space', ctrlKey: true })).toBe(true)
    expect(run).not.toHaveBeenCalled()

    expect(offerToTerminal(LEADER)).toBe(false)
    expect(offerToTerminal({ key: '2' })).toBe(false)
    expect(run).toHaveBeenCalledTimes(1)
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

describe('direct chords', () => {
  const run = vi.fn()
  const other = vi.fn()

  beforeEach(() => {
    resetChordsForTest()
    run.mockClear()
    other.mockClear()
    act(() => {
      registerChords([
        chord({ id: 'send', key: 's', direct: { alt: true, shift: false, key: 's' }, scope: 'global', run }),
        chord({ id: 'next-window', key: 'w', direct: { alt: true, shift: false, key: 'w' }, scope: 'global', run }),
        chord({ id: 'prev-window', key: 'W', direct: { alt: true, shift: true, key: 'w' }, scope: 'global', run: other }),
        chord({ id: 'add-window', key: '=', direct: { alt: true, key: '+', layoutKeys: ['='] }, scope: 'global', run }),
        chord({ id: 'remove-window', key: '-', direct: { alt: true, key: '-' }, scope: 'global', run: other }),
        chord({ id: 'peek', key: 'p', direct: { alt: true, shift: false, key: 'p' }, scope: 'tile', run: other }),
      ])
    })
  })

  afterEach(() => resetChordsForTest())

  it('runs without the leader, taking the key from both the browser and the pty', () => {
    const event = press({ key: 's', altKey: true })

    expect(run).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)

    expect(offerToTerminal({ key: 's', altKey: true })).toBe(false)
    expect(run).toHaveBeenCalledTimes(2)

    // Off is off here too: the browser keeps the key and the pty gets it.
    act(() => setKeysEnabled(false))
    expect(press({ key: 's', altKey: true }).defaultPrevented).toBe(false)
    expect(offerToTerminal({ key: 's', altKey: true })).toBe(true)
    expect(run).toHaveBeenCalledTimes(2)
    act(() => setKeysEnabled(true))
  })

  it('leaves an unregistered Alt key to the pty', () => {
    const event = press({ key: 'x', altKey: true })

    expect(run).not.toHaveBeenCalled()
    expect(other).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
    // The xterm handler agrees: true is "xterm handles this key", which is how
    // Alt+X leaves as the escape sequence the shell expects.
    expect(offerToTerminal({ key: 'x', altKey: true })).toBe(true)
  })

  it('tells Alt+W from Alt+Shift+W', () => {
    press({ key: 'w', altKey: true })
    expect(run).toHaveBeenCalledTimes(1)
    expect(other).not.toHaveBeenCalled()

    press({ key: 'W', altKey: true, shiftKey: true })
    expect(other).toHaveBeenCalledTimes(1)
    expect(run).toHaveBeenCalledTimes(1)
  })

  // A Finnish keyboard types "+" with no Shift; a US one types "=" unshifted
  // and "+" with Shift. All three are one chord to the operator.
  it('matches Plus on either layout, and Minus on both', () => {
    press({ key: '+', altKey: true })
    press({ key: '=', altKey: true })
    press({ key: '+', altKey: true, shiftKey: true })
    expect(run).toHaveBeenCalledTimes(3)

    press({ key: '-', altKey: true })
    expect(other).toHaveBeenCalledTimes(1)
  })

  // AltGr arrives as Ctrl+Alt, and a Finnish layout needs it for @ and {.
  it('ignores AltGr and Alt held with another modifier', () => {
    expect(press({ key: 's', altKey: true, ctrlKey: true }).defaultPrevented).toBe(false)
    expect(press({ key: 's', altKey: true, metaKey: true }).defaultPrevented).toBe(false)
    expect(offerToTerminal({ key: 's', altKey: true, ctrlKey: true })).toBe(true)
    expect(run).not.toHaveBeenCalled()
  })

  it('runs a scoped direct chord only while its scope is active', () => {
    press({ key: 'p', altKey: true })
    expect(other).not.toHaveBeenCalled()

    act(() => setActiveScopes({ workspace: true, tile: true }))
    press({ key: 'p', altKey: true })
    expect(other).toHaveBeenCalledTimes(1)
  })

  // Holding Alt says which chord is meant, so the open window does not get to
  // read the key as its bare self instead.
  it('beats the leader window’s next key, and shuts the window', () => {
    press(LEADER)
    const event = press({ key: 's', altKey: true })

    expect(run).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)

    // The window is shut, so the next bare key belongs to the pty again.
    expect(offerToTerminal({ key: 's' })).toBe(true)
    expect(run).toHaveBeenCalledTimes(1)
  })

  it('reads the way the operator says it', () => {
    expect(directChordLabel({ alt: true, shift: false, key: 's' })).toBe('Alt+S')
    expect(directChordLabel({ alt: true, shift: true, key: 'w' })).toBe('Alt+Shift+W')
    expect(directChordLabel({ alt: true, key: '+', layoutKeys: ['='] })).toBe('Alt+Plus')
    expect(directChordLabel({ alt: true, key: '-' })).toBe('Alt+Minus')
    expect(directChordLabel({ alt: true, shift: false, key: '1' })).toBe('Alt+1')
  })
})
