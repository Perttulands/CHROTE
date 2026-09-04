/**
 * Two short tones, made on the spot with the Web Audio API so no audio file
 * is shipped or fetched: finished is a two-note fall, needs-input a single
 * higher note, 120 ms a note.
 */

import type { AgentEventKind } from '../types'

export const TONE_NOTE_SECONDS = 0.12
const TONE_LEVEL = 0.2

export interface ToneNote {
  frequency: number
  /** Seconds after the tone starts. */
  at: number
}

export const TONE_SCHEDULES: Record<AgentEventKind, readonly ToneNote[]> = {
  // E5 falling to A4.
  finished: [{ frequency: 659.25, at: 0 }, { frequency: 440, at: TONE_NOTE_SECONDS }],
  // A5, above both.
  'needs-input': [{ frequency: 880, at: 0 }],
}

/** What playing a tone needs of an AudioContext, so a test can stand one in. */
export interface ToneContext {
  readonly currentTime: number
  readonly destination: AudioNode
  readonly state?: AudioContextState
  createOscillator(): OscillatorNode
  createGain(): GainNode
  resume?(): Promise<void>
}

export function playTone(event: AgentEventKind, context: ToneContext): void {
  // A context made before the operator's first gesture starts suspended, and
  // stays so until asked; after any gesture on the page the ask is answered.
  if (context.state === 'suspended') void context.resume?.()
  const start = context.currentTime
  for (const note of TONE_SCHEDULES[event]) {
    const oscillator = context.createOscillator()
    const gain = context.createGain()
    const at = start + note.at
    const end = at + TONE_NOTE_SECONDS
    oscillator.type = 'sine'
    oscillator.frequency.value = note.frequency
    // A 10 ms rise and a 20 ms fall keep the note from clicking at its edges.
    gain.gain.setValueAtTime(0, at)
    gain.gain.linearRampToValueAtTime(TONE_LEVEL, at + 0.01)
    gain.gain.setValueAtTime(TONE_LEVEL, end - 0.02)
    gain.gain.linearRampToValueAtTime(0, end)
    oscillator.connect(gain)
    gain.connect(context.destination)
    oscillator.start(at)
    oscillator.stop(end)
  }
}

let shared: AudioContext | null = null

/** The page's one AudioContext, made on first use; null where there is none. */
export function audioContext(): ToneContext | null {
  if (shared) return shared
  const Context = window.AudioContext
    ?? (window as Window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
  if (!Context) return null
  shared = new Context()
  return shared
}
