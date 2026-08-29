import { getFileBaseName } from '../FileViewer'
import type { SavedPath } from './pinnedPaths'

export type SavedPathGroup = 'pinned' | 'recent'
export type SavedGroupsCollapsed = Record<SavedPathGroup, boolean>

export const RECENT_STORAGE_KEY = 'chrote.files.recentPaths'
const SAVED_GROUPS_COLLAPSED_STORAGE_KEY = 'chrote.files.savedGroupsCollapsed'
const DEFAULT_SAVED_GROUPS_COLLAPSED: SavedGroupsCollapsed = { pinned: false, recent: false }

export function savedPathLabel(path: string): string {
  const base = getFileBaseName(path)
  return base === '/' ? path : base
}

export function readSavedPaths(key: string): SavedPath[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return []
    return parsed
      .map((item): SavedPath | null => {
        if (typeof item === 'string') return { path: item, kind: 'file' }
        if (item && typeof item === 'object' && 'path' in item && typeof item.path === 'string') {
          return { path: item.path, kind: 'kind' in item && item.kind === 'directory' ? 'directory' : 'file' }
        }
        return null
      })
      .filter((item): item is SavedPath => item !== null)
  } catch {
    return []
  }
}

export function writeSavedPaths(key: string, paths: SavedPath[]): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(key, JSON.stringify(paths))
  } catch {
    // Storage failures do not block the file UI.
  }
}

export function readSavedGroupsCollapsed(): SavedGroupsCollapsed {
  if (typeof window === 'undefined') return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
  try {
    const raw = window.localStorage.getItem(SAVED_GROUPS_COLLAPSED_STORAGE_KEY)
    if (!raw) return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
    const record = parsed as Partial<Record<SavedPathGroup, unknown>>
    return { pinned: record.pinned === true, recent: record.recent === true }
  } catch {
    return { ...DEFAULT_SAVED_GROUPS_COLLAPSED }
  }
}

export function writeSavedGroupsCollapsed(collapsed: SavedGroupsCollapsed): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(SAVED_GROUPS_COLLAPSED_STORAGE_KEY, JSON.stringify(collapsed))
  } catch {
    // Storage failures do not block the file UI.
  }
}
