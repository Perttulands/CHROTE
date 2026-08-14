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

export function readPinnedPaths(): SavedPath[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(PINNED_STORAGE_KEY)
    if (!raw) return []
    const paths = normalizePinnedPaths(JSON.parse(raw) as unknown)
    const serialized = JSON.stringify(paths)
    if (serialized !== raw) window.localStorage.setItem(PINNED_STORAGE_KEY, serialized)
    return paths
  } catch {
    return []
  }
}

function publishPinnedPaths(paths: SavedPath[]): SavedPath[] {
  const normalized = normalizePinnedPaths(paths)
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(PINNED_STORAGE_KEY, JSON.stringify(normalized))
    } catch {
      // localStorage quota/private mode failures should not break the file UI.
    }
    window.dispatchEvent(new CustomEvent<SavedPath[]>(PINNED_PATHS_EVENT, { detail: normalized }))
  }
  return normalized
}

export function togglePinnedPath(path: string, kind: SavedKind): SavedPath[] {
  const cleanPath = normalizeFilePath(path)
  const current = readPinnedPaths()
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
      setPaths(normalizePinnedPaths(customEvent.detail))
    }
    const handleStorage = (event: StorageEvent) => {
      if (event.key === PINNED_STORAGE_KEY) setPaths(readPinnedPaths())
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
