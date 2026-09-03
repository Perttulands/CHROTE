/**
 * The chord registry: one keyboard model for the whole dashboard.
 *
 * The operator presses the leader, a strip lists what the current scope
 * offers, and the next key runs one of them. The state lives at module level
 * rather than in React because the terminal is the surface that has to give a
 * key up: `terminalSession.ts` builds an xterm outside React and hands its
 * `attachCustomKeyEventHandler` straight to `terminalKeyEvent` below. React
 * subscribes to the same singleton through `useLeader`.
 *
 * Two listeners take the leader, and they agree by construction: the document
 * capture listener runs first and marks the event taken, so the xterm handler
 * that would see the same event only confirms the verdict. Neither runs at all
 * while keys are off — that is the whole meaning of the toggle.
 */

import { useSyncExternalStore } from 'react'

export type ChordScope = 'global' | 'workspace' | 'tile'

export interface Chord {
  /** Stable identity, unique per registration site. */
  id: string
  /** `KeyboardEvent.key`. Single letters match either case. */
  key: string
  /** What running it does, in the operator's words. */
  label: string
  scope: ChordScope
  run: () => void
}

/** The leader, as the operator reads it and as the event arrives. */
export const LEADER_LABEL = 'Ctrl+Shift+Space'
/** How long the leader window stays open with nothing pressed. */
export const LEADER_WINDOW_MS = 1500

const SCOPE_RANK: Record<ChordScope, number> = { global: 0, workspace: 1, tile: 2 }

export const SCOPE_TITLES: Record<ChordScope, string> = {
  global: 'Anywhere',
  workspace: 'Terminal workspace',
  tile: 'Focused tile',
}

// The order the wave-1 contract lists the first chords in. It is presentation,
// but it belongs here: the strip and the keys panel must read the same, however
// many surfaces registered the chords.
const KEY_ORDER = ['1', '2', '3', '4', '[', ']', '=', '-', '/', 'n', 'p', 's', 'Tab', 'f', 'b', '?', 'k', 'Escape']

// A modifier's own keydown is not "the next key": holding Shift to reach ? or S
// would otherwise cancel the window before the chord ever arrived.
const MODIFIER_KEYS = new Set(['Shift', 'Control', 'Alt', 'Meta', 'CapsLock', 'AltGraph', 'OS', 'Dead'])

export interface KeysSnapshot {
  keysEnabled: boolean
  leaderOpen: boolean
  /** Echoed at the strip's left while the window is open. */
  pressed: readonly string[]
  /** What the current scope offers, in contract order. */
  scopeChords: readonly Chord[]
  /** Everything registered, whatever the scope, for the keys panel. */
  allChords: readonly Chord[]
}

const registrations = new Map<symbol, Chord>()
const listeners = new Set<() => void>()

let keysEnabled = true
let activeScopes: Record<ChordScope, boolean> = { global: true, workspace: false, tile: false }
let leaderOpen = false
let pressed: readonly string[] = []
let closeTimer: ReturnType<typeof setTimeout> | null = null

function orderChords(chords: Chord[]): Chord[] {
  return chords.sort((a, b) => {
    const byScope = SCOPE_RANK[a.scope] - SCOPE_RANK[b.scope]
    if (byScope !== 0) return byScope
    const rank = (chord: Chord) => {
      const index = KEY_ORDER.indexOf(chord.key.length === 1 ? chord.key.toLowerCase() : chord.key)
      return index === -1 ? KEY_ORDER.length : index
    }
    return rank(a) - rank(b)
  })
}

function computeSnapshot(): KeysSnapshot {
  const all = orderChords(Array.from(registrations.values()))
  return {
    keysEnabled,
    leaderOpen,
    pressed,
    scopeChords: all.filter(chord => activeScopes[chord.scope]),
    allChords: all,
  }
}

let snapshot: KeysSnapshot = computeSnapshot()

function publish() {
  snapshot = computeSnapshot()
  listeners.forEach(listener => listener())
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

/** Read the keyboard model in React. The strip, the panel and the toggle share it. */
export function useLeader(): KeysSnapshot {
  return useSyncExternalStore(subscribe, () => snapshot, () => snapshot)
}

/** Register one chord; the returned function retires it. */
export function registerChord(chord: Chord): () => void {
  const token = Symbol(chord.id)
  registrations.set(token, chord)
  publish()
  return () => {
    if (!registrations.delete(token)) return
    publish()
  }
}

/** Register a set of chords as one unit, retired together. */
export function registerChords(chords: readonly Chord[]): () => void {
  const tokens = chords.map(chord => {
    const token = Symbol(chord.id)
    registrations.set(token, chord)
    return token
  })
  publish()
  return () => {
    let changed = false
    tokens.forEach(token => { changed = registrations.delete(token) || changed })
    if (changed) publish()
  }
}

/**
 * Which scopes the strip lists and the next key can reach: global always,
 * workspace while a terminal tab is active, tile while a tile is focused.
 */
export function setActiveScopes(scopes: { workspace: boolean; tile: boolean }): void {
  if (activeScopes.workspace === scopes.workspace && activeScopes.tile === scopes.tile) return
  activeScopes = { global: true, workspace: scopes.workspace, tile: scopes.tile }
  publish()
}

/** The device-local toggle. Off means nothing is intercepted anywhere. */
export function setKeysEnabled(enabled: boolean): void {
  if (keysEnabled === enabled) return
  keysEnabled = enabled
  if (!enabled) cancelLeader(false)
  publish()
}

function cancelLeader(shouldPublish = true) {
  if (closeTimer !== null) {
    clearTimeout(closeTimer)
    closeTimer = null
  }
  if (!leaderOpen) return
  leaderOpen = false
  pressed = []
  if (shouldPublish) publish()
}

function openLeader() {
  if (closeTimer !== null) clearTimeout(closeTimer)
  leaderOpen = true
  pressed = [LEADER_LABEL]
  closeTimer = setTimeout(() => {
    closeTimer = null
    cancelLeader()
  }, LEADER_WINDOW_MS)
  publish()
}

export function isLeaderEvent(event: KeyboardEvent): boolean {
  if (!event.ctrlKey || !event.shiftKey || event.altKey || event.metaKey) return false
  return event.code === 'Space' || event.key === ' ' || event.key === 'Spacebar'
}

function chordFor(event: KeyboardEvent): Chord | null {
  const match = snapshot.scopeChords.filter(chord => (
    chord.key.length === 1 && /[a-z]/i.test(chord.key)
      ? event.key.length === 1 && event.key.toLowerCase() === chord.key.toLowerCase()
      : event.key === chord.key
  ))
  // Later registrations win, so a surface that is on screen can take a key
  // from one that registered the same key earlier.
  return match[match.length - 1] ?? null
}

function resolveLeaderKey(event: KeyboardEvent) {
  const chord = chordFor(event)
  cancelLeader()
  chord?.run()
}

// One event, one verdict. The document capture listener and the xterm handler
// both see the leader inside a focused terminal; the second one to look must
// not open the window a second time.
const taken = new WeakSet<KeyboardEvent>()

/**
 * Offer a keydown to the keyboard model. True means the model took it and no
 * one else — no pty, no browser default — may act on it.
 */
export function interceptKeyEvent(event: KeyboardEvent): boolean {
  if (!keysEnabled) return false
  if (event.type !== 'keydown') return false
  if (taken.has(event)) return true
  if (isLeaderEvent(event)) {
    taken.add(event)
    openLeader()
    return true
  }
  if (!leaderOpen) return false
  if (MODIFIER_KEYS.has(event.key)) return false
  taken.add(event)
  resolveLeaderKey(event)
  return true
}

/**
 * xterm's `attachCustomKeyEventHandler` contract: true lets xterm handle the
 * key and send it to the pty, false keeps it out of the terminal entirely.
 */
export function terminalKeyEvent(event: KeyboardEvent): boolean {
  return !interceptKeyEvent(event)
}

// The capture phase is what puts this ahead of everything else in the page,
// including the textarea listener xterm attaches for its own keys. Installed
// once, at import, so the model is live before any surface asks for it.
if (typeof document !== 'undefined') {
  document.addEventListener('keydown', event => {
    if (!interceptKeyEvent(event)) return
    event.preventDefault()
    event.stopPropagation()
  }, true)
}

/** Test seam: forget every registration and close the window. */
export function resetChordsForTest(): void {
  registrations.clear()
  keysEnabled = true
  activeScopes = { global: true, workspace: false, tile: false }
  cancelLeader(false)
  publish()
}
