/**
 * The Beads tab's view state as a link: what it searches, which projects and
 * statuses it keeps, the brushed days, the order, the selected Bead and the
 * layout. Defaults are left out, so a plain link stays plain.
 */

import {
  NATURAL_DIRECTION,
  SLICE_SORT_KEYS,
  SLICE_STATUSES,
  dayISO,
  parseDayISO,
  type SliceSortKey,
  type SliceStatus,
  type SortDirection,
} from './beadsSlice'

export type BeadsMode = 'split' | 'table' | 'graph'
export const BEADS_MODES: readonly BeadsMode[] = ['split', 'table', 'graph']

export interface SliceSelection {
  id: string
  /** The project key, carried only when the id alone names more than one Bead. */
  project?: string
}

export interface SliceViewState {
  q: string
  /** Project keys kept; null keeps every project. */
  projects: readonly string[] | null
  statuses: readonly SliceStatus[] | null
  /** Inclusive local days. */
  range: { from: number; to: number } | null
  sort: SliceSortKey
  dir: SortDirection
  selected: SliceSelection | null
  mode: BeadsMode
}

export const DEFAULT_VIEW: SliceViewState = {
  q: '', projects: null, statuses: null, range: null, sort: 'updated', dir: 'desc', selected: null, mode: 'split',
}

/** Every query key the view owns, the tab included. */
export const VIEW_KEYS = ['tab', 'q', 'p', 's', 'from', 'to', 'sort', 'dir', 'sel', 'mode'] as const

function list(value: string | null): string[] | null {
  if (value === null) return null
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

export function isBeadsLink(search: string): boolean {
  return new URLSearchParams(search).get('tab') === 'beads'
}

export function readViewState(search: string): SliceViewState {
  const params = new URLSearchParams(search)
  if (params.get('tab') !== 'beads') return DEFAULT_VIEW
  const statuses = list(params.get('s'))?.filter((status): status is SliceStatus => (SLICE_STATUSES as readonly string[]).includes(status)) ?? null
  const sortValue = params.get('sort')
  const sort = (SLICE_SORT_KEYS as readonly string[]).includes(sortValue ?? '') ? sortValue as SliceSortKey : DEFAULT_VIEW.sort
  const dirValue = params.get('dir')
  const dir: SortDirection = dirValue === 'asc' || dirValue === 'desc' ? dirValue : NATURAL_DIRECTION[sort]
  const from = parseDayISO(params.get('from'))
  const to = parseDayISO(params.get('to')) ?? from
  const selection = params.get('sel')
  const at = selection?.indexOf('@') ?? -1
  const modeValue = params.get('mode')
  return {
    q: params.get('q') ?? '',
    projects: list(params.get('p')),
    statuses,
    range: from !== null && to !== null ? { from: Math.min(from, to), to: Math.max(from, to) } : null,
    sort,
    dir,
    selected: selection
      ? at > 0 ? { id: selection.slice(0, at), project: selection.slice(at + 1) } : { id: selection }
      : null,
    mode: (BEADS_MODES as readonly string[]).includes(modeValue ?? '') ? modeValue as BeadsMode : DEFAULT_VIEW.mode,
  }
}

/** Write the view into a URL's query, replacing whatever the view owned before. */
export function writeViewState(state: SliceViewState, url: URL): URL {
  const next = new URL(url.toString())
  VIEW_KEYS.forEach(key => next.searchParams.delete(key))
  next.searchParams.set('tab', 'beads')
  if (state.q.trim()) next.searchParams.set('q', state.q)
  if (state.projects) next.searchParams.set('p', state.projects.join(','))
  if (state.statuses) next.searchParams.set('s', state.statuses.join(','))
  if (state.range) {
    next.searchParams.set('from', dayISO(state.range.from))
    if (state.range.to !== state.range.from) next.searchParams.set('to', dayISO(state.range.to))
  }
  if (state.sort !== DEFAULT_VIEW.sort || state.dir !== DEFAULT_VIEW.dir) {
    next.searchParams.set('sort', state.sort)
    if (state.dir !== NATURAL_DIRECTION[state.sort]) next.searchParams.set('dir', state.dir)
  }
  if (state.selected) next.searchParams.set('sel', state.selected.project ? `${state.selected.id}@${state.selected.project}` : state.selected.id)
  if (state.mode !== DEFAULT_VIEW.mode) next.searchParams.set('mode', state.mode)
  return next
}

/** The URL with the view's keys removed, for when the tab is left. */
export function clearViewState(url: URL): URL {
  const next = new URL(url.toString())
  VIEW_KEYS.forEach(key => next.searchParams.delete(key))
  return next
}
