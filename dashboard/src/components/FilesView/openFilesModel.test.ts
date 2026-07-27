import { describe, expect, it } from 'vitest'
import {
  EMPTY_OPEN_FILES,
  applyRead,
  closeBuffer,
  describeBuffers,
  dirtyBuffersUnder,
  openBuffer,
  patchBuffer,
  pruneViewStates,
  remapBuffers,
  remapConflicts,
  remapViewStates,
  removeBuffersUnder,
  type OpenFilesState,
} from './openFilesModel'

// The workbench allocates read tokens from a counter that outlives the buffer
// set; the tests mirror that so token reuse is never accidentally simulated.
let tokenCounter = 0
function nextToken(): number {
  tokenCounter += 1
  return tokenCounter
}

function open(state: OpenFilesState, path: string, loading = true): OpenFilesState {
  return openBuffer(state, {
    path,
    name: path.split('/').pop() || path,
    size: 10,
    type: 'text/plain',
    kind: 'text',
    loading,
    error: null,
  }, nextToken()).state
}

function withDirty(state: OpenFilesState, path: string, content: string): OpenFilesState {
  return patchBuffer(state, path, { content, dirty: true, loading: false })
}

describe('openFilesModel', () => {
  it('focuses an already-open buffer instead of re-reading it', () => {
    const token = nextToken()
    const first = openBuffer(EMPTY_OPEN_FILES, {
      path: '/notes.txt', name: 'notes.txt', size: 10, type: 'text/plain', kind: 'text', loading: true, error: null,
    }, token)
    expect(first.readToken).toBe(token)

    const edited = withDirty(first.state, '/notes.txt', 'unsaved work')
    const reopened = openBuffer(edited, {
      path: '/notes.txt', name: 'notes.txt', size: 10, type: 'text/plain', kind: 'text', loading: true, error: null,
    }, nextToken())

    expect(reopened.readToken).toBeNull()
    expect(reopened.state.files).toHaveLength(1)
    expect(reopened.state.files[0].content).toBe('unsaved work')
    expect(reopened.state.files[0].dirty).toBe(true)
    expect(reopened.state.activePath).toBe('/notes.txt')
  })

  it('drops read results that lost their claim on the buffer', () => {
    const opened = openBuffer(EMPTY_OPEN_FILES, {
      path: '/a.txt', name: 'a.txt', size: 10, type: 'text/plain', kind: 'text', loading: true, error: null,
    }, nextToken())
    const token = opened.readToken as number

    // Edited while the read was in flight: disk content must not win.
    const dirty = withDirty(opened.state, '/a.txt', 'my edit')
    expect(applyRead(dirty, '/a.txt', token, { content: 'disk content' }).files[0].content).toBe('my edit')

    // Closed and reopened: the old token no longer matches.
    const reopened = open(closeBuffer(opened.state, '/a.txt'), '/a.txt')
    expect(applyRead(reopened, '/a.txt', token, { content: 'stale' }).files[0].content).toBe('')

    // Moved before the read landed: nothing at the old path to apply to.
    const moved = remapBuffers(opened.state, '/a.txt', '/sub/a.txt')
    expect(applyRead(moved, '/a.txt', token, { content: 'stale' })).toBe(moved)

    // The current token still applies normally.
    const applied = applyRead(opened.state, '/a.txt', token, { content: 'fresh' })
    expect(applied.files[0]).toMatchObject({ content: 'fresh', loading: false, error: null })
  })

  it('keeps each buffer bound to the token its read was issued under', () => {
    const first = openBuffer(EMPTY_OPEN_FILES, {
      path: '/a.txt', name: 'a.txt', size: 1, type: '', kind: 'text', loading: true, error: null,
    }, nextToken())
    const second = openBuffer(first.state, {
      path: '/b.txt', name: 'b.txt', size: 1, type: '', kind: 'text', loading: true, error: null,
    }, nextToken())

    expect(second.readToken).not.toBe(first.readToken)
    // A read for one buffer never lands on another.
    const applied = applyRead(second.state, '/b.txt', first.readToken as number, { content: 'wrong buffer' })
    expect(applied).toBe(second.state)
  })

  it('reports dirty buffers under a delete target, including descendants', () => {
    let state = open(EMPTY_OPEN_FILES, '/project/notes.txt')
    state = open(state, '/project/deep/draft.md')
    state = open(state, '/other/clean.txt')
    state = withDirty(state, '/project/deep/draft.md', 'unsaved')

    const dirty = dirtyBuffersUnder(state, ['/project'])
    expect(describeBuffers(dirty)).toBe('draft.md')
    expect(dirtyBuffersUnder(state, ['/other'])).toEqual([])
  })

  it('removes deleted buffers and their descendants, then re-picks an active tab', () => {
    let state = open(EMPTY_OPEN_FILES, '/keep.txt')
    state = open(state, '/project/a.txt')
    state = open(state, '/project/nested/b.txt')
    expect(state.activePath).toBe('/project/nested/b.txt')

    const next = removeBuffersUnder(state, ['/project'])
    expect(next.files.map(file => file.path)).toEqual(['/keep.txt'])
    expect(next.activePath).toBe('/keep.txt')
    expect(removeBuffersUnder(next, ['/keep.txt']).activePath).toBeNull()
  })

  it('remaps a moved folder across buffers, active path, and view states', () => {
    let state = open(EMPTY_OPEN_FILES, '/src/index.ts')
    state = open(state, '/src/deep/util.ts')
    state = withDirty(state, '/src/deep/util.ts', 'unsaved move test')

    const moved = remapBuffers(state, '/src', '/lib/src')
    expect(moved.files.map(file => file.path)).toEqual(['/lib/src/index.ts', '/lib/src/deep/util.ts'])
    expect(moved.activePath).toBe('/lib/src/deep/util.ts')
    // The edit rides along with the buffer.
    expect(moved.files[1]).toMatchObject({ dirty: true, content: 'unsaved move test', name: 'util.ts' })

    const viewStates = remapViewStates(
      { '/src/deep/util.ts': { scrollTop: 42 } as never, '/elsewhere.txt': { scrollTop: 1 } as never },
      '/src',
      '/lib/src',
    )
    expect(Object.keys(viewStates).sort()).toEqual(['/elsewhere.txt', '/lib/src/deep/util.ts'])
  })

  it('renames a single buffer and its tab label', () => {
    const state = withDirty(open(EMPTY_OPEN_FILES, '/notes.txt'), '/notes.txt', 'unsaved')
    const renamed = remapBuffers(state, '/notes.txt', '/notes-final.txt')
    expect(renamed.files[0]).toMatchObject({ path: '/notes-final.txt', name: 'notes-final.txt', dirty: true, content: 'unsaved' })
    expect(renamed.activePath).toBe('/notes-final.txt')
  })

  it('detects moves that would collide with another open buffer', () => {
    let state = open(EMPTY_OPEN_FILES, '/a/notes.txt')
    state = open(state, '/b/notes.txt')

    expect(remapConflicts(state, '/a/notes.txt', '/b/notes.txt')).toEqual(['/b/notes.txt'])
    expect(remapConflicts(state, '/a/notes.txt', '/c/notes.txt')).toEqual([])
    // Moving a folder onto a path only it occupies is not a conflict.
    expect(remapConflicts(state, '/a', '/archive/a')).toEqual([])
  })

  it('prunes view states for deleted paths', () => {
    const pruned = pruneViewStates(
      { '/project/a.txt': { scrollTop: 1 } as never, '/keep.txt': { scrollTop: 2 } as never },
      ['/project'],
    )
    expect(Object.keys(pruned)).toEqual(['/keep.txt'])
  })

  it('activates the neighbouring tab when the active buffer closes', () => {
    let state = open(EMPTY_OPEN_FILES, '/a.txt')
    state = open(state, '/b.txt')
    state = open(state, '/c.txt')

    const closed = closeBuffer({ ...state, activePath: '/b.txt' }, '/b.txt')
    expect(closed.activePath).toBe('/c.txt')
    expect(closeBuffer(closed, '/a.txt').activePath).toBe('/c.txt')
  })
})
