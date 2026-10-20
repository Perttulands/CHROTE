import type { MapTransform } from '../../hooks/useMapTransform'

export type BrainViewMode = 'subjects' | 'map'

export interface BrainViewSnapshot {
  mode: BrainViewMode
  subject: string | null
  picked: string | null
  facet: string
  collection: string
  hiddenKinds: string[]
  hiddenShelves: string[]
  scrub: number
  cameras: { map: MapTransform | null; subjects: MapTransform | null }
}

export interface BrainSavedView {
  name: string
  snapshot: BrainViewSnapshot
}

export const BRAIN_VIEWS_KEY = 'chrote.brain.views.v1'
export const BRAIN_VIEW_SLOTS = 4
export type BrainViewSlots = (BrainSavedView | null)[]
type ViewStorage = Pick<Storage, 'getItem' | 'setItem'>

export function emptyBrainViews(): BrainViewSlots {
  return Array.from({ length: BRAIN_VIEW_SLOTS }, () => null)
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function camera(value: unknown): boolean {
  return value === null || (record(value) && typeof value.x === 'number' && Number.isFinite(value.x)
    && typeof value.y === 'number' && Number.isFinite(value.y)
    && typeof value.scale === 'number' && Number.isFinite(value.scale) && value.scale > 0)
}

function strings(value: unknown): value is string[] {
  return Array.isArray(value) && value.every(item => typeof item === 'string')
}

export function isBrainViewSnapshot(value: unknown): value is BrainViewSnapshot {
  return record(value) && (value.mode === 'subjects' || value.mode === 'map')
    && (value.subject === null || typeof value.subject === 'string')
    && (value.picked === null || typeof value.picked === 'string')
    && typeof value.facet === 'string' && typeof value.collection === 'string'
    && strings(value.hiddenKinds) && strings(value.hiddenShelves)
    && typeof value.scrub === 'number' && Number.isFinite(value.scrub) && value.scrub >= 0 && value.scrub <= 1000
    && record(value.cameras) && camera(value.cameras.map) && camera(value.cameras.subjects)
}

export function copyBrainView(snapshot: BrainViewSnapshot): BrainViewSnapshot {
  return { ...snapshot, hiddenKinds: [...snapshot.hiddenKinds], hiddenShelves: [...snapshot.hiddenShelves],
    cameras: { map: snapshot.cameras.map ? { ...snapshot.cameras.map } : null,
      subjects: snapshot.cameras.subjects ? { ...snapshot.cameras.subjects } : null } }
}

export function readBrainViews(storage: ViewStorage): BrainViewSlots {
  const raw = storage.getItem(BRAIN_VIEWS_KEY)
  if (raw === null) return emptyBrainViews()
  let value: unknown
  try { value = JSON.parse(raw) } catch { throw new Error('Saved Brain views could not be read: invalid stored data.') }
  if (!record(value) || value.version !== 1 || !Array.isArray(value.slots) || value.slots.length !== BRAIN_VIEW_SLOTS
    || !value.slots.every(slot => slot === null || (record(slot) && typeof slot.name === 'string'
      && slot.name.trim().length > 0 && isBrainViewSnapshot(slot.snapshot)))) {
    throw new Error('Saved Brain views could not be read: unsupported stored data.')
  }
  return value.slots as BrainViewSlots
}

export function writeBrainViews(storage: ViewStorage, slots: BrainViewSlots): void {
  storage.setItem(BRAIN_VIEWS_KEY, JSON.stringify({ version: 1, slots }))
}
