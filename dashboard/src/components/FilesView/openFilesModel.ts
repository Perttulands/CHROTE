// Authoritative owner of open-buffer state for the Files workbench.
//
// Every command that can destroy an edit — open, read completion, close,
// delete, rename, move — is a pure transition here, so dirty-buffer rules live
// in one place instead of being re-derived at each button. The view keeps React
// state; it just never mutates the buffer set directly.

import type { FileViewState } from '../workspaceFilesState'
import type { StoredOpenFile } from './filesWorkbenchState'

export interface OpenFile extends StoredOpenFile {
  content: string
  dirty: boolean
  loading: boolean
  error: string | null
  // Monotonic per-buffer read token. A read result only applies while the token
  // it was issued under is still current, which is what makes out-of-order or
  // superseded reads harmless.
  readToken: number
}

export interface OpenFilesState {
  files: OpenFile[]
  activePath: string | null
}

export const EMPTY_OPEN_FILES: OpenFilesState = { files: [], activePath: null }

export interface NewBuffer {
  path: string
  name: string
  size: number
  type: string
  kind: StoredOpenFile['kind']
  loading: boolean
  error: string | null
}

export interface OpenResult {
  state: OpenFilesState
  /** Read token to issue the content fetch under, or null when no read is needed. */
  readToken: number | null
}

function isPathOrDescendant(path: string, root: string): boolean {
  return path === root || path.startsWith(`${root}/`)
}

function remapPath(path: string, from: string, to: string): string {
  if (path === from) return to
  if (path.startsWith(`${from}/`)) return `${to}${path.slice(from.length)}`
  return path
}

/**
 * Open a buffer, or focus it when it is already open.
 *
 * An already-open buffer is never re-read: a re-open is a focus request, and
 * re-reading would overwrite unsaved edits with disk content.
 *
 * The read token is supplied by the caller from a counter that outlives the
 * buffer set. Deriving it from the live buffers would recycle tokens once tabs
 * close, letting a stale read land on a reopened tab.
 */
export function openBuffer(state: OpenFilesState, buffer: NewBuffer, readToken: number): OpenResult {
  const existing = state.files.find(file => file.path === buffer.path)
  if (existing) {
    return { state: { ...state, activePath: buffer.path }, readToken: null }
  }

  const opened: OpenFile = {
    path: buffer.path,
    name: buffer.name,
    size: buffer.size,
    type: buffer.type,
    kind: buffer.kind,
    content: '',
    dirty: false,
    loading: buffer.loading,
    error: buffer.error,
    readToken,
  }
  return {
    state: { files: [...state.files, opened], activePath: buffer.path },
    readToken: buffer.loading ? readToken : null,
  }
}

/**
 * Apply a completed read. The result is dropped unless the buffer still exists
 * at the same path with the same token, so a slow read cannot overwrite newer
 * edits, a moved path, or a reopened tab.
 */
export function applyRead(
  state: OpenFilesState,
  path: string,
  readToken: number,
  result: { content: string } | { error: string },
): OpenFilesState {
  const target = state.files.find(file => file.path === path)
  if (!target || target.readToken !== readToken) return state
  if (target.dirty) return state

  return {
    ...state,
    files: state.files.map(file => {
      if (file.path !== path) return file
      if ('error' in result) return { ...file, loading: false, error: result.error }
      return { ...file, content: result.content, loading: false, error: null }
    }),
  }
}

export function patchBuffer(state: OpenFilesState, path: string, patch: Partial<OpenFile>): OpenFilesState {
  return {
    ...state,
    files: state.files.map(file => (file.path === path ? { ...file, ...patch } : file)),
  }
}

/** Close one buffer, choosing the neighbouring tab as the next active one. */
export function closeBuffer(state: OpenFilesState, path: string): OpenFilesState {
  const index = state.files.findIndex(file => file.path === path)
  if (index === -1) return state
  const files = state.files.filter(file => file.path !== path)
  if (state.activePath !== path) return { ...state, files }
  const fallback = files[index] || files[index - 1] || files[0] || null
  return { ...state, files, activePath: fallback?.path ?? null }
}

export function closeAllBuffers(): OpenFilesState {
  return EMPTY_OPEN_FILES
}

export function closeOtherBuffers(state: OpenFilesState, path: string): OpenFilesState {
  const keep = state.files.find(file => file.path === path)
  if (!keep) return state
  return { ...state, files: [keep], activePath: keep.path }
}

/** Drop every buffer at or under the given roots, e.g. after a delete. */
export function removeBuffersUnder(state: OpenFilesState, roots: string[]): OpenFilesState {
  const files = state.files.filter(file => !roots.some(root => isPathOrDescendant(file.path, root)))
  if (files.length === state.files.length) return state
  const activeStillOpen = state.activePath !== null && files.some(file => file.path === state.activePath)
  return { ...state, files, activePath: activeStillOpen ? state.activePath : files[0]?.path ?? null }
}

/**
 * Rewrite a moved or renamed path across every buffer, including descendants,
 * so buffers stay attached to the file they were opened from.
 */
export function remapBuffers(state: OpenFilesState, from: string, to: string): OpenFilesState {
  if (from === to) return state
  const files = state.files.map(file => {
    const path = remapPath(file.path, from, to)
    if (path === file.path) return file
    return { ...file, path, name: path.split('/').pop() || path }
  })
  return {
    ...state,
    files,
    activePath: state.activePath === null ? null : remapPath(state.activePath, from, to),
  }
}

/**
 * A move collides when it would land a buffer on a path another *different*
 * buffer already occupies. Silently merging two tabs would drop one buffer's
 * edits, so callers block instead.
 */
export function remapConflicts(state: OpenFilesState, from: string, to: string): string[] {
  if (from === to) return []
  const moving = state.files.filter(file => isPathOrDescendant(file.path, from))
  const staying = state.files.filter(file => !isPathOrDescendant(file.path, from))
  return moving
    .map(file => remapPath(file.path, from, to))
    .filter(destination => staying.some(file => file.path === destination))
}

/** Dirty buffers at or under the given roots, in tab order. */
export function dirtyBuffersUnder(state: OpenFilesState, roots: string[]): OpenFile[] {
  return state.files.filter(file => file.dirty && roots.some(root => isPathOrDescendant(file.path, root)))
}

export function dirtyBuffers(state: OpenFilesState): OpenFile[] {
  return state.files.filter(file => file.dirty)
}

export function findBuffer(state: OpenFilesState, path: string | null): OpenFile | null {
  if (!path) return null
  return state.files.find(file => file.path === path) ?? null
}

/** Remap persisted per-file view state alongside a rename or move. */
export function remapViewStates(
  viewStates: Record<string, FileViewState>,
  from: string,
  to: string,
): Record<string, FileViewState> {
  if (from === to) return viewStates
  return Object.entries(viewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
    next[remapPath(path, from, to)] = value
    return next
  }, {})
}

/** Drop per-file view state for deleted paths so it cannot resurrect later. */
export function pruneViewStates(
  viewStates: Record<string, FileViewState>,
  roots: string[],
): Record<string, FileViewState> {
  return Object.entries(viewStates).reduce<Record<string, FileViewState>>((next, [path, value]) => {
    if (!roots.some(root => isPathOrDescendant(path, root))) next[path] = value
    return next
  }, {})
}

/** Human-readable list used by the dirty-buffer guards. */
export function describeBuffers(files: OpenFile[]): string {
  return files.map(file => file.name).join(', ')
}
