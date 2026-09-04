/**
 * Which resident's column is on screen, for the surfaces that are not its
 * parent.
 *
 * Alt+Enter is registered once, with the dashboard's other chords, and has to
 * reach whichever column is active; the Bead card has to know that Alt+S is
 * the resident's in this tab and not its own. Neither is a React ancestor of
 * the column, so the front column announces itself here and they read it.
 * Hidden tabs stay mounted for their UI state, but only the active column
 * announces itself.
 */

import { useSyncExternalStore } from 'react'
import type { ResidentTab } from './residentsApi'

export interface ResidentHandle {
  tab: ResidentTab
  /** Put the keyboard in the resident's terminal, or on the column when there is none. */
  focus: () => void
  /**
   * Put one line in the resident's prompt, unsubmitted. False when there is no
   * session to take it, which is the caller's cue to open the drawer instead.
   */
  paste: (line: string) => Promise<boolean>
}

let mounted: ResidentHandle | null = null
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

/** Announce a mounted column; the returned function withdraws it. */
export function mountResident(handle: ResidentHandle): () => void {
  mounted = handle
  publish()
  return () => {
    if (mounted !== handle) return
    mounted = null
    publish()
  }
}

/** Alt+Enter's target. False when no tab with a resident is in front. */
export function focusResident(): boolean {
  if (mounted === null) return false
  mounted.focus()
  return true
}

/**
 * Hand a line to the resident living in the tab in front. False when no column
 * is mounted or its session is not running, so a row's "Send to <resident>"
 * can fall back to the drawer rather than dropping the operator's request.
 */
export async function pasteToResident(line: string): Promise<boolean> {
  if (mounted === null) return false
  return mounted.paste(line)
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function readPresent(): boolean {
  return mounted !== null
}

/** Whether a resident's column is on screen, for a surface that shares its tab. */
export function useResidentPresent(): boolean {
  return useSyncExternalStore(subscribe, readPresent, readPresent)
}

export function resetResidentForTest(): void {
  mounted = null
  publish()
}
