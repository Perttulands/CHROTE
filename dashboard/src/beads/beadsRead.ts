/** One visible-demand owner for disposable Beads projections. Source truth stays in bd. */
import { useEffect, useSyncExternalStore } from 'react'
import {
  fetchBeadStates, fetchBeadWork, fetchClosedBeadWork, fetchBeadSnapshot,
  type BeadDetail, type BeadProject, type BeadStoreState, type BeadWork,
} from './beadsApi'
import { currentBead } from './beadStatus'
import type { WorkRow } from './beadsTree'
import { forgetBeadDetail, invalidateKnownBeads, rememberBeadDetail, rememberBeadRows } from './knownBeads'

export interface Projection<T> {
  data?: T
  generation?: string
  loading: boolean
  error?: string
  retryAt: number
  dirty?: boolean
  refreshVersion?: number
  appliedVersion?: number
}
export interface StoreRead {
  state?: BeadStoreState
  work: Projection<BeadWork>
  closed: Projection<BeadWork>
  cards: Map<string, Projection<BeadDetail>>
  touched: number
  waitForRefresh?: boolean
  rowCache?: Partial<Record<'work' | 'closed', { source: BeadWork; label: string; expired: number; rows: WorkRow[] }>>
}
interface Demand {
  paths: readonly string[]
  foreground?: readonly string[]
  work?: boolean
  workPaths?: readonly string[]
  closed?: boolean
  card?: { path: string; id: string }
}
const POLL_MS = 2000
const PENDING_POLL_MS = 250
const MAX_STORES = 64
const MAX_CARDS = 128
const IDLE_MS = 10 * 60 * 1000
const stores = new Map<string, StoreRead>()
const demands = new Map<symbol, Demand>()
const listeners = new Set<() => void>()
let revision = 0
let timer: ReturnType<typeof setTimeout> | undefined
let checking = false
let queued = false
let visibilityInstalled = false
let projects: BeadProject[] = []
let epoch = 0
const forcedPaths = new Set<string>()
const freshProjection = <T>(): Projection<T> => ({ loading: false, retryAt: 0 })

function publish() {
  revision += 1
  listeners.forEach(listener => listener())
}
function entry(path: string): StoreRead {
  let result = stores.get(path)
  if (!result) {
    result = { work: freshProjection(), closed: freshProjection(), cards: new Map(), touched: Date.now() }
    stores.set(path, result)
  }
  result.touched = Date.now()
  return result
}
export function beadStoreRead(path: string): StoreRead | undefined { return stores.get(path) }
const EMPTY_ROWS: WorkRow[] = []
/** State-only polls do not change row/graph identity; dated expiry still does. */
export function beadRows(path: string, label: string, kind: 'work' | 'closed'): WorkRow[] {
  const store = stores.get(path)
  const source = store?.[kind].data
  if (!store || !source) return EMPTY_ROWS
  const expired = source.beads.filter(row => row.status === 'deferred' && currentBead(row).status === 'open').length
  const prior = store.rowCache?.[kind]
  if (prior?.source === source && prior.label === label && prior.expired === expired) return prior.rows
  const rows = source.beads.map(row => ({ ...currentBead(row), projectPath: path, projectName: label }))
  store.rowCache ??= {}
  store.rowCache[kind] = { source, label, expired, rows }
  return rows
}
/** Saved owning-store paths need no host catalog before they can be demanded. */
export function beadProjectSkeletons(known: readonly BeadProject[], manualPaths: readonly string[]): BeadProject[] {
  const result = [...known]
  const paths = new Set(result.map(project => project.path))
  manualPaths.forEach(value => {
    const path = value.trim().replace(/\/+$/, '') || (value.trim() === '/' ? '/' : '')
    if (!path || paths.has(path)) return
    paths.add(path)
    result.push({ path, name: path.split('/').filter(Boolean).slice(-1)[0] || path,
      beadsPath: `${path === '/' ? '' : path}/.beads`, source: 'manual', summaryPending: true })
  })
  return result
}
export function rememberedBeadProjects(): BeadProject[] { return projects }
export function rememberBeadProjects(found: BeadProject[]) { projects = found }

function visible() { return typeof document !== 'undefined' && document.visibilityState !== 'hidden' }
function currentDemand() {
  const paths = new Set<string>()
  const foreground = new Set<string>()
  const work = new Set<string>()
  const closed = new Set<string>()
  const cards = new Map<string, Set<string>>()
  demands.forEach(demand => {
    demand.paths.forEach(path => paths.add(path))
    demand.foreground?.forEach(path => foreground.add(path))
    if (demand.work) (demand.workPaths ?? demand.paths).forEach(path => work.add(path))
    if (demand.closed) demand.paths.forEach(path => closed.add(path))
    if (demand.card) {
      const { path, id } = demand.card
      paths.add(path)
      foreground.add(path)
      const ids = cards.get(path) ?? new Set<string>()
      ids.add(id)
      cards.set(path, ids)
    }
  })
  return { paths, foreground, work, closed, cards }
}
function acceptState(state: BeadStoreState) {
  const store = entry(state.path)
  const generation = state.availableGeneration
  if (generation && generation !== store.state?.availableGeneration) invalidateKnownBeads(state.path, generation)
  store.state = state
  if (store.waitForRefresh && !state.pending && !forcedPaths.has(state.path)) store.waitForRefresh = false
}
function shouldRead<T>(projection: Projection<T>, generation: string | undefined) {
  return !projection.loading && Date.now() >= projection.retryAt &&
    (!projection.data || projection.dirty || !!projection.error || (!!generation && projection.generation !== generation))
}
function read<T>(path: string, projection: Projection<T>, fetch: () => Promise<{ data: T; state?: BeadStoreState }>, remember: (data: T, generation?: string) => void) {
  const store = entry(path)
  if (store.waitForRefresh || !shouldRead(projection, store.state?.availableGeneration)) return
  projection.loading = true
  const startedEpoch = epoch
  const refreshVersion = projection.refreshVersion
  const appliedVersion = projection.appliedVersion ?? 0
  publish()
  void fetch().then(result => {
    if (startedEpoch !== epoch || (projection.appliedVersion ?? 0) !== appliedVersion) return
    // A successful response alone advances the applied projection generation.
    // Its metadata describes its actual snapshot, even if a newer check raced it.
    projection.data = result.data
    projection.appliedVersion = appliedVersion + 1
    projection.generation = result.state?.availableGeneration ?? store.state?.availableGeneration
    projection.error = undefined
    if (projection.refreshVersion === refreshVersion) projection.dirty = false
    projection.retryAt = 0
    if (result.state && (!store.state?.availableGeneration || store.state.availableGeneration === result.state.availableGeneration && (!store.state.checkedAt || (result.state.checkedAt ?? '') >= store.state.checkedAt))) {
      acceptState(result.state)
    }
    remember(result.data, projection.generation)
  }).catch((cause: unknown) => {
    if (startedEpoch !== epoch || (projection.appliedVersion ?? 0) !== appliedVersion) return
    projection.error = cause instanceof Error ? cause.message : 'Could not read Beads'
    projection.retryAt = Date.now() + POLL_MS
  }).finally(() => {
    if (startedEpoch !== epoch) return
    projection.loading = false
    publish()
    if (projection.dirty && !store.waitForRefresh) readWanted()
  })
}
function readWanted() {
  if (!visible()) return
  const wanted = currentDemand()
  wanted.paths.forEach(path => {
    const store = entry(path)
    // A cold host checks state and fills independently. Only selected/card
    // demand waits on a promoted direct projection read.
    if (!store.state?.availableGeneration && !wanted.foreground.has(path)) return
    if (wanted.work.has(path) && store.state?.openBeads === 0 && !wanted.foreground.has(path)) {
      if (store.work.generation !== store.state.availableGeneration || !store.work.data) {
        store.work.data = { beads: [], prefix: '', projectPath: path, state: store.state }
        store.work.generation = store.state.availableGeneration
        store.work.appliedVersion = (store.work.appliedVersion ?? 0) + 1
        store.work.error = undefined
        rememberBeadRows(path, [], store.work.generation, 'work')
        publish()
      }
    } else if (wanted.work.has(path)) read(path, store.work, async () => {
      const data = await fetchBeadWork(path)
      return { data, state: data.state }
    }, (data, generation) => rememberBeadRows(path, data.beads, generation, 'work'))
    if (wanted.closed.has(path)) read(path, store.closed, async () => {
      const data = await fetchClosedBeadWork(path)
      return { data, state: data.state }
    }, (data, generation) => rememberBeadRows(path, data.beads, generation, 'closed'))
    wanted.cards.get(path)?.forEach(id => {
      let card = store.cards.get(id)
      if (!card) { card = freshProjection(); store.cards.set(id, card) }
      read(path, card, async () => {
        const result = await fetchBeadSnapshot(path, id)
        return { data: result.bead, state: result.state }
      }, (data, generation) => rememberBeadDetail(path, data, generation))
    })
  })
}
function evict() {
  const wanted = currentDemand()
  const candidates = [...stores.entries()].filter(([path, store]) =>
    !wanted.paths.has(path) && !store.work.loading && !store.closed.loading && ![...store.cards.values()].some(card => card.loading),
  ).sort((a, b) => a[1].touched - b[1].touched)
  candidates.forEach(([path, store]) => {
    if (stores.size > MAX_STORES || Date.now() - store.touched > IDLE_MS) {
      stores.delete(path)
      invalidateKnownBeads(path)
    }
  })
  let count = [...stores.values()].reduce((sum, store) => sum + store.cards.size, 0)
  for (const [path, store] of stores) {
    for (const [id, card] of store.cards) {
      if (count <= MAX_CARDS) return
      if (!card.loading && !wanted.cards.get(path)?.has(id)) { store.cards.delete(id); forgetBeadDetail(path, id); count -= 1 }
    }
  }
}
function schedule() {
  if (timer) clearTimeout(timer)
  timer = undefined
  if (demands.size && visible()) {
    // A foreground job is already running. Check its completion promptly;
    // settled and background-only demand keep the ordinary cheap cadence.
    const pending = [...currentDemand().foreground].some(path => stores.get(path)?.state?.pending)
    timer = setTimeout(() => { void check() }, pending ? PENDING_POLL_MS : POLL_MS)
  }
}
async function check(refreshPaths?: readonly string[]) {
  if (!visible() || !demands.size) { schedule(); return }
  if (checking) { queued = true; return }
  if (!refreshPaths && forcedPaths.size) { refreshPaths = [...forcedPaths]; forcedPaths.clear() }
  checking = true
  const startedEpoch = epoch
  const wanted = currentDemand()
  const paths = refreshPaths ?? [...wanted.paths]
  try {
    const states = await fetchBeadStates(paths, [...wanted.foreground], !!refreshPaths)
    if (startedEpoch !== epoch) return
    states.forEach(acceptState)
    publish()
  } catch (cause: unknown) {
    if (startedEpoch !== epoch) return
    const error = cause instanceof Error ? cause.message : 'Could not check Beads freshness'
    paths.forEach(path => {
      const store = entry(path)
      store.state = { ...store.state, path, pending: false, error }
      store.waitForRefresh = false
    })
    publish()
  } finally {
    if (startedEpoch === epoch) {
      checking = false
      evict()
      if (forcedPaths.size) {
        const pending = [...forcedPaths]
        forcedPaths.clear()
        pending.forEach(path => { entry(path).waitForRefresh = true })
        void check(pending)
      } else {
        readWanted()
        if (queued) { queued = false; poke() } else schedule()
      }
    }
  }
}
function onVisibility() {
  if (visible()) poke()
  else { if (timer) clearTimeout(timer); timer = undefined }
}
function poke() {
  readWanted()
  if (queued) return
  queued = true
  queueMicrotask(() => { queued = false; void check() })
}
export function demandBeads(demand: Demand): () => void {
  const key = Symbol('beads demand')
  demands.set(key, demand)
  demand.paths.forEach(entry)
  if (!visibilityInstalled) { document.addEventListener('visibilitychange', onVisibility); visibilityInstalled = true }
  poke()
  return () => { demands.delete(key); schedule() }
}
/** An explicit relationship navigation shares the same retained card read. */
export function readBeadDetail(path: string, id: string): Promise<BeadDetail> {
  const store = entry(path)
  let projection = store.cards.get(id)
  if (!projection) { projection = freshProjection(); store.cards.set(id, projection) }
  const stop = demandBeads({ paths: [path], card: { path, id } })
  const current = projection
  return new Promise((resolve, reject) => {
    const finish = () => {
      if (current.loading) return
      if (current.data && !current.error) { listeners.delete(finish); stop(); resolve(current.data) }
      else if (current.error) { listeners.delete(finish); stop(); reject(new Error(current.error)) }
    }
    listeners.add(finish)
    finish()
  })
}

export function refreshBeads(paths: readonly string[]) {
  paths.forEach(path => {
    const store = entry(path)
    store.waitForRefresh = true
    const mark = <T>(projection: Projection<T>) => {
      projection.dirty = true
      projection.refreshVersion = (projection.refreshVersion ?? 0) + 1
    }
    mark(store.work)
    mark(store.closed)
    store.cards.forEach(mark)
    store.work.retryAt = store.closed.retryAt = 0
    store.cards.forEach(card => { card.retryAt = 0 })
  })
  // Coalesce repeated clicks without a second loop or a lost force request.
  if (checking || !visible()) paths.forEach(path => forcedPaths.add(path))
  else void check(paths)
}

const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }
const snapshot = () => revision
export function useBeadsRead(demand: Demand, active: boolean): number {
  const key = JSON.stringify(demand)
  useEffect(() => {
    if (active) return demandBeads(JSON.parse(key) as Demand)
  }, [active, key])
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}
export function resetBeadsReadForTest() {
  if (timer) clearTimeout(timer)
  timer = undefined
  epoch += 1
  forcedPaths.clear()
  stores.clear(); demands.clear(); projects = []; listeners.clear()
  checking = false; queued = false; revision = 0
  if (visibilityInstalled) document.removeEventListener('visibilitychange', onVisibility)
  visibilityInstalled = false
}
