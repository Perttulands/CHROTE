/**
 * Which resident's column is on screen, for the surfaces that are not its
 * parent.
 *
 * Alt+Enter is registered once, with the dashboard's other chords, and has to
 * reach whichever column is mounted; the Bead card has to know that Alt+S is
 * the resident's in this tab and not its own. Neither is a React ancestor of
 * the column, so the mounted column announces itself here and they read it.
 * At most one column is mounted at a time, because a tab is one at a time.
 */

import { useSyncExternalStore } from 'react'
import type { ResidentTab } from './residentsApi'

export interface ResidentHandle {
  tab: ResidentTab
  /** Put the keyboard in the resident's terminal, or on the column when there is none. */
  focus: () => void
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
