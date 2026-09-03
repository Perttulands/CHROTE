/**
 * The chord registry: one keyboard model for the whole dashboard.
 *
 * The operator presses the leader, the keys panel lists what the current scope
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
 *
 * Alt is the CHROTE key. Most chords also carry a direct form the operator
 * reaches without the leader, and the leader is then discovery: it opens the
 * keys panel, which lists the same chords. Only a registered direct chord is
 * swallowed; every other Alt combination is the shell's.
 */

import { useSyncExternalStore } from 'react'

export type ChordScope = 'global' | 'workspace' | 'tile'

/**
 * The Alt form of a chord: how the operator actually runs it, leader or no
 * leader, terminal focused or not.
 */
export interface DirectChord {
  /** Alt is the CHROTE key, so every direct chord holds it. */
  alt: true
  /**
   * Whether Shift is held. Leave it out where the character in
   * `KeyboardEvent.key` already answers the question: the layout decides
   * whether Plus needs Shift, and CHROTE has no business insisting.
   */
  shift?: boolean
  /** The canonical `KeyboardEvent.key`, and what the strip reads. */
  key: string
  /**
   * The same physical key as another layout spells it, by `KeyboardEvent.key`.
   * A Finnish keyboard types `+` unshifted where a US one types `=`.
   */
  layoutKeys?: readonly string[]
}

export interface Chord {
  /** Stable identity, unique per registration site. */
  id: string
  /** `KeyboardEvent.key`. Single letters match either case. */
  key: string
  /** The Alt chord that runs this without the leader, where one exists. */
  direct?: DirectChord
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

// The order the keyboard map lists the chords in. It is presentation, but it
// belongs here: the keys panel must read the same however many surfaces
// registered the chords. Case matters, so `w` and `W` can both appear.
const KEY_ORDER = [
  '1', '2', '3', '4', '5', '6', 'b', 'l', 'g', 'd', '?', 'k', 'Escape',
  'w', 'W', '=', '-', '/', 'n', 'Tab', 'f', 'p', 's', 'r',
]

/** How a key reads once it is a chord: `Alt+Plus`, never `Alt++`. */
const KEY_NAMES: Record<string, string> = { '+': 'Plus', '-': 'Minus' }

// A modifier's own keydown is not "the next key": holding Shift to reach ? or S
// would otherwise cancel the window before the chord ever arrived.
const MODIFIER_KEYS = new Set(['Shift', 'Control', 'Alt', 'Meta', 'CapsLock', 'AltGraph', 'OS', 'Dead'])

/** One key cap in the echo badge. A modifier cap is filled; a key cap is outlined. */
export interface KeyCap {
  label: string
  modifier: boolean
}

/** The last registered chord to fire, for the badge that echoes it. */
export interface KeyEcho {
  /** Rises on every fire, so the same chord twice is two echoes. */
  nonce: number
  caps: readonly KeyCap[]
}

export interface KeysSnapshot {
  keysEnabled: boolean
  leaderOpen: boolean
  /** The keys pressed so far in the leader window. */
  pressed: readonly string[]
  /** What the current scope offers, in contract order. */
  scopeChords: readonly Chord[]
  /** Everything registered, whatever the scope, for the keys panel. */
  allChords: readonly Chord[]
  /** What to echo, or null before any chord has fired. */
  echo: KeyEcho | null
}

const registrations = new Map<symbol, Chord>()
const listeners = new Set<() => void>()

let keysEnabled = true
let activeScopes: Record<ChordScope, boolean> = { global: true, workspace: false, tile: false }
let leaderOpen = false
let pressed: readonly string[] = []
let closeTimer: ReturnType<typeof setTimeout> | null = null
let echo: KeyEcho | null = null
let echoSeq = 0

/** How a key reads on a cap or in a chord: `Plus`, `S`, `Escape`. */
function keyName(key: string): string {
  return KEY_NAMES[key] ?? (key.length === 1 ? key.toUpperCase() : key)
}

/** How the operator reads a direct chord: `Alt+S`, `Alt+Shift+W`, `Alt+Plus`. */
export function directChordLabel(direct: DirectChord): string {
  return `Alt+${direct.shift ? 'Shift+' : ''}${keyName(direct.key)}`
}

/** The caps the badge shows for a chord: the modifiers it holds, then the key. */
export function chordCaps(chord: Chord): readonly KeyCap[] {
  if (chord.direct === undefined) return [{ label: keyName(chord.key), modifier: false }]
  return [
    { label: 'ALT', modifier: true },
    ...(chord.direct.shift === true ? [{ label: 'SHIFT', modifier: true }] : []),
    { label: keyName(chord.direct.key), modifier: false },
  ]
}

function echoChord(chord: Chord) {
  echoSeq += 1
  echo = { nonce: echoSeq, caps: chordCaps(chord) }
}

function orderChords(chords: Chord[]): Chord[] {
  return chords.sort((a, b) => {
    const byScope = SCOPE_RANK[a.scope] - SCOPE_RANK[b.scope]
    if (byScope !== 0) return byScope
    const rank = (chord: Chord) => {
      const exact = KEY_ORDER.indexOf(chord.key)
      const index = exact !== -1 ? exact : KEY_ORDER.indexOf(chord.key.toLowerCase())
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
    echo,
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

function keyMatches(wanted: string, event: KeyboardEvent): boolean {
  return wanted.length === 1 && /[a-z]/i.test(wanted)
    ? event.key.length === 1 && event.key.toLowerCase() === wanted.toLowerCase()
    : event.key === wanted
}

// Later registrations win, so a surface that is on screen can take a key from
// one that registered the same key earlier.
function last(chords: readonly Chord[]): Chord | null {
  return chords[chords.length - 1] ?? null
}

function chordFor(event: KeyboardEvent): Chord | null {
  // An exact key beats a case-insensitive one, which is how `w` and `W` can
  // both be leader keys for the two directions of the window cycle.
  const exact = snapshot.scopeChords.filter(chord => chord.key === event.key)
  if (exact.length > 0) return last(exact)
  return last(snapshot.scopeChords.filter(chord => keyMatches(chord.key, event)))
}

/**
 * Whether this keydown is a candidate for a direct chord at all. AltGr arrives
 * as Ctrl+Alt on Windows and a Finnish layout needs it for @, $, { and }: those
 * keys belong to the program in the terminal, never to CHROTE.
 */
function isDirectEvent(event: KeyboardEvent): boolean {
  return event.altKey && !event.ctrlKey && !event.metaKey
}

function directMatches(direct: DirectChord, event: KeyboardEvent): boolean {
  if (direct.shift !== undefined && direct.shift !== event.shiftKey) return false
  return [direct.key, ...(direct.layoutKeys ?? [])].some(key => keyMatches(key, event))
}

function directChordFor(event: KeyboardEvent): Chord | null {
  if (!isDirectEvent(event)) return null
  return last(snapshot.scopeChords.filter(
    chord => chord.direct !== undefined && directMatches(chord.direct, event),
  ))
}

/**
 * Shut the leader window from outside the model. The keys panel needs it: the
 * leader opens the panel, and the panel's search must get the next key rather
 * than lose it to a window that is still listening.
 */
export function closeLeaderWindow(): void {
  cancelLeader()
}

function resolveLeaderKey(event: KeyboardEvent) {
  const chord = chordFor(event)
  cancelLeader(false)
  if (chord) echoChord(chord)
  publish()
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
  // Registered Alt chords, and nothing else with Alt in it: an unregistered
  // combination is the pty's, which is what makes Alt safe to take. This is
  // asked before the leader window's next key, because an operator holding Alt
  // means the direct chord — Alt+K is the keys panel even while the window that
  // reads a bare `k` as "keys off" is open.
  const direct = directChordFor(event)
  if (direct !== null) {
    taken.add(event)
    cancelLeader(false)
    echoChord(direct)
    publish()
    direct.run()
    return true
  }
  if (leaderOpen) {
    if (MODIFIER_KEYS.has(event.key)) return false
    taken.add(event)
    resolveLeaderKey(event)
    return true
  }
  return false
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
  echo = null
  echoSeq = 0
  cancelLeader(false)
  publish()
}
