/**
 * Every Bead on the host, as the Beads tab reads it.
 *
 * One request answers for every store: the server keeps a projection per store
 * generation, and the browser names the generations it already holds, so a
 * poll over unchanged stores carries no rows at all. Nothing here runs per
 * interaction: filtering, sorting and the graph are the browser's own work, and
 * search sends only the query, answered from the server's retained text.
 */

import { useEffect, useSyncExternalStore } from 'react'

/** One Bead as the table, the strip and the graph draw it. Times are Unix seconds. */
export interface HostBead {
  id: string
  title: string
  status: string
  type?: string
  priority: number
  created?: number
  updated?: number
  closed?: number
  /** Characters of description, design, acceptance and notes together. */
  size?: number
  parent?: string
  /** Every Bead this one waits on, finished or not, in any store. */
  blockedBy?: string[]
  deferUntil?: string
}

export interface HostStore {
  path: string
  prefix?: string
  generation?: string
  readAt?: string
  /** A read of this store is in flight; its rows, if any, are the previous ones. */
  pending: boolean
  error?: string
  rows: readonly HostBead[]
}

export interface HostRead {
  stores: readonly HostStore[]
  /** When the server last answered. */
  checkedAt?: string
  /** True until the first answer. */
  loading: boolean
  /** The request itself failed; the stores are the last successful answer. */
  error?: string
  /** The stores as of their last row change: the same array until rows change,
   * so an index built from it is rebuilt only then. */
  rowStores: readonly HostStore[]
}

interface WireStore {
  path: string
  prefix?: string
  generation?: string
  readAt?: string
  pending: boolean
  error?: string
  unchanged?: boolean
  rows?: HostBead[]
}

interface Envelope<T> {
  success?: boolean
  data?: T
  error?: { message?: string }
}

const NO_ROWS: readonly HostBead[] = []
const POLL_MS = 3000
const PENDING_POLL_MS = 1000
const REQUEST_TIMEOUT_MS = 30000

const NO_STORES: readonly HostStore[] = []
let state: HostRead = { stores: NO_STORES, rowStores: NO_STORES, loading: true }
const listeners = new Set<() => void>()
let demand = 0
let manual: readonly string[] = []
let timer: ReturnType<typeof setTimeout> | undefined
let inFlight = false
let again = false
let epoch = 0

function publish(next: HostRead) {
  state = next
  listeners.forEach(listener => listener())
}

async function getJSON<T>(path: string, params: URLSearchParams, signal: AbortSignal): Promise<T> {
  const response = await fetch(`/api/beads/${path}?${params.toString()}`, { signal })
  const envelope = await response.json().catch(() => null) as Envelope<T> | null
  if (!envelope || envelope.success !== true || envelope.data === undefined) {
    throw new Error(envelope?.error?.message || `Beads request failed (${response.status})`)
  }
  return envelope.data
}

/**
 * Merge one answer into what the browser holds. Unchanged stores keep their
 * rows, and so does a store the server holds nothing for yet, as it does
 * after a restart: its last rows stand until the server has read it again.
 */
export function mergeHostAnswer(previous: HostRead, answer: { stores: WireStore[]; checkedAt?: string }): HostRead {
  const held = new Map(previous.stores.map(store => [store.path, store]))
  let rowsChanged = previous.stores.length !== answer.stores.length
  const stores = answer.stores.map(wire => {
    const prior = held.get(wire.path)
    const unread = !wire.generation && !wire.rows
    const kept = prior && (unread || wire.unchanged || prior.generation === wire.generation) ? prior.rows : NO_ROWS
    const rows = wire.rows ?? kept
    if (rows !== prior?.rows) rowsChanged = true
    return {
      path: wire.path,
      prefix: wire.prefix ?? (unread ? prior?.prefix : undefined),
      generation: unread ? prior?.generation : wire.generation,
      readAt: unread ? prior?.readAt : wire.readAt,
      pending: wire.pending,
      error: wire.error,
      rows,
    }
  }).sort((a, b) => a.path.localeCompare(b.path))
  if (!rowsChanged) rowsChanged = stores.some((store, i) => store.path !== previous.stores[i]?.path)
  return {
    stores,
    checkedAt: answer.checkedAt,
    loading: false,
    rowStores: rowsChanged ? stores : previous.rowStores,
  }
}

async function poll() {
  if (timer) clearTimeout(timer)
  timer = undefined
  if (demand === 0 || (typeof document !== 'undefined' && document.visibilityState === 'hidden')) return
  if (inFlight) { again = true; return }
  inFlight = true
  const started = epoch
  const params = new URLSearchParams()
  manual.forEach(path => params.append('path', path))
  state.stores.forEach(store => {
    if (store.generation) params.append('known', `${store.generation}:${store.path}`)
  })
  try {
    const answer = await getJSON<{ stores: WireStore[]; checkedAt?: string }>('host', params, AbortSignal.timeout(REQUEST_TIMEOUT_MS))
    if (started !== epoch) return
    publish(mergeHostAnswer(state, answer))
  } catch (cause) {
    if (started !== epoch) return
    publish({ ...state, loading: false, error: cause instanceof Error ? cause.message : 'Could not read Beads' })
  } finally {
    if (started === epoch) {
      inFlight = false
      if (again) { again = false; void poll() } else schedule()
    }
  }
}

function schedule() {
  if (timer) clearTimeout(timer)
  timer = undefined
  if (demand === 0) return
  const pending = state.loading || state.stores.some(store => store.pending)
  timer = setTimeout(() => { void poll() }, pending ? PENDING_POLL_MS : POLL_MS)
}

function onVisibility() {
  if (document.visibilityState !== 'hidden') void poll()
}

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
const read = () => state

/** Read every store while `active`; polling stops when nothing is visible. */
export function useHostBeads(active: boolean, manualPaths: readonly string[]): HostRead {
  const key = JSON.stringify([...new Set(manualPaths.map(path => path.trim()).filter(Boolean))].sort())
  useEffect(() => {
    if (!active) return
    manual = JSON.parse(key) as string[]
    demand += 1
    if (demand === 1) document.addEventListener('visibilitychange', onVisibility)
    void poll()
    return () => {
      demand -= 1
      if (demand === 0) {
        document.removeEventListener('visibilitychange', onVisibility)
        if (timer) clearTimeout(timer)
        timer = undefined
      }
    }
  }, [active, key])
  return useSyncExternalStore(subscribe, read, read)
}

/** The ids, per store path, whose text holds every term of the query. */
export async function searchHostBeads(query: string, signal: AbortSignal): Promise<Record<string, string[]>> {
  const data = await getJSON<{ matches: Record<string, string[]> }>('search', new URLSearchParams({ q: query }), signal)
  return data.matches ?? {}
}

export function resetHostBeadsForTest() {
  epoch += 1
  if (timer) clearTimeout(timer)
  timer = undefined
  if (demand > 0) document.removeEventListener('visibilitychange', onVisibility)
  demand = 0
  inFlight = false
  again = false
  manual = []
  listeners.clear()
  state = { stores: NO_STORES, rowStores: NO_STORES, loading: true }
}
