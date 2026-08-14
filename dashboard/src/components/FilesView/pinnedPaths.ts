import { useCallback, useEffect, useState } from 'react'
import { normalizeFilePath } from '../FileViewer'

export type SavedKind = 'file' | 'directory'

export interface SavedPath {
  path: string
  kind: SavedKind
}

export const PINNED_STORAGE_KEY = 'chrote.files.pinnedPaths'
const PINNED_PATHS_EVENT = 'chrote:pinned-paths-change'
const MAX_PINNED_PATHS = 20

let pinnedPathsSnapshot: SavedPath[] | null = null

function normalizePinnedPaths(value: unknown): SavedPath[] {
  if (!Array.isArray(value)) return []

  const normalized: SavedPath[] = []
  const seen = new Set<string>()
  value.forEach(item => {
    let path: string | null = null
    let kind: SavedKind = 'file'
    if (typeof item === 'string') {
      path = item
    } else if (item && typeof item === 'object' && 'path' in item && typeof item.path === 'string') {
      path = item.path
      kind = 'kind' in item && item.kind === 'directory' ? 'directory' : 'file'
    }
    if (path === null) return

    const cleanPath = normalizeFilePath(path)
    if (seen.has(cleanPath)) return
    seen.add(cleanPath)
    normalized.push({ path: cleanPath, kind })
  })

  return normalized.slice(0, MAX_PINNED_PATHS)
}

function parsePinnedPaths(raw: string | null): SavedPath[] {
  if (!raw) return []
  try {
    return normalizePinnedPaths(JSON.parse(raw) as unknown)
  } catch {
    return []
  }
}

function adoptPinnedPaths(paths: SavedPath[]): SavedPath[] {
  pinnedPathsSnapshot = normalizePinnedPaths(paths)
  return pinnedPathsSnapshot
}

export function readPinnedPaths(): SavedPath[] {
  if (typeof window === 'undefined') return pinnedPathsSnapshot ?? []

  let raw: string | null
  try {
    raw = window.localStorage.getItem(PINNED_STORAGE_KEY)
  } catch {
    return pinnedPathsSnapshot ?? []
  }

  const paths = adoptPinnedPaths(parsePinnedPaths(raw))
  const serialized = JSON.stringify(paths)
  if (raw && serialized !== raw) {
    try {
      window.localStorage.setItem(PINNED_STORAGE_KEY, serialized)
    } catch {
      // Canonical persistence is best-effort; the valid parsed snapshot remains authoritative.
    }
  }
  return paths
}

function publishPinnedPaths(paths: SavedPath[]): SavedPath[] {
  const normalized = adoptPinnedPaths(paths)
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(PINNED_STORAGE_KEY, JSON.stringify(normalized))
    } catch {
      // The mounted UI continues from the authoritative in-memory snapshot.
    }
    window.dispatchEvent(new CustomEvent<SavedPath[]>(PINNED_PATHS_EVENT, { detail: normalized }))
  }
  return normalized
}

export function togglePinnedPath(path: string, kind: SavedKind): SavedPath[] {
  const cleanPath = normalizeFilePath(path)
  const current = pinnedPathsSnapshot ?? readPinnedPaths()
  const next = current.some(item => item.path === cleanPath)
    ? current.filter(item => item.path !== cleanPath)
    : [{ path: cleanPath, kind }, ...current]
  return publishPinnedPaths(next)
}

export function usePinnedPaths(): [SavedPath[], (path: string, kind: SavedKind) => void] {
  const [paths, setPaths] = useState<SavedPath[]>(readPinnedPaths)

  useEffect(() => {
    const handlePinnedPaths = (event: Event) => {
      const customEvent = event as CustomEvent<SavedPath[]>
      setPaths(adoptPinnedPaths(customEvent.detail))
    }
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== PINNED_STORAGE_KEY && event.key !== null) return
      const next = event.key === null ? [] : parsePinnedPaths(event.newValue)
      setPaths(adoptPinnedPaths(next))
    }
    window.addEventListener(PINNED_PATHS_EVENT, handlePinnedPaths)
    window.addEventListener('storage', handleStorage)
    return () => {
      window.removeEventListener(PINNED_PATHS_EVENT, handlePinnedPaths)
      window.removeEventListener('storage', handleStorage)
    }
  }, [])

  const toggle = useCallback((path: string, kind: SavedKind) => {
    setPaths(togglePinnedPath(path, kind))
  }, [])

  return [paths, toggle]
}
