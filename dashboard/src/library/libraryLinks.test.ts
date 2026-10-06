import { describe, expect, it } from 'vitest'
import { createLibraryResolver } from './libraryLinks'

const pages = [
  { path: 'Goals.md', title: 'Root target' },
  { path: 'knowledge/notes.md', title: 'Shared heading' },
  { path: 'knowledge/archive/notes.md', title: 'Archived heading' },
  { path: 'knowledge/by-title.md', title: 'notes' },
  { path: 'preferences/notes.md', title: ' Shared heading ' },
  { path: 'preferences/different-name.md', title: 'Unique title' },
  { path: 'telos/goals.md', title: 'Goals title' },
]

describe('Library wikilink resolution', () => {
  // These are the native libraryIndex.resolve rules: path wins over a local
  // basename, filenames win over titles, and ambiguities keep graph order.
  it.each([
    [' /TeLoS/GOALS.MD ', 'knowledge/source.md', 'telos/goals.md'],
    ['/telos/goals/', 'knowledge/source.md', 'telos/goals.md'],
    ['goals', 'telos/source.md', 'Goals.md'],
    ['missing/shelf/NOTES.md', 'preferences/source.md', 'preferences/notes.md'],
    ['notes', 'knowledge/archive/source.md', 'knowledge/notes.md'],
    ['notes', 'unknown/source.md', 'knowledge/notes.md'],
    ['SHARED HEADING', 'preferences/source.md', 'preferences/notes.md'],
    ['Shared heading', 'unknown/source.md', 'knowledge/notes.md'],
    [' unique TITLE ', 'knowledge/source.md', 'preferences/different-name.md'],
    ['telos/goals', 'telos/goals.md', 'telos/goals.md'],
    ['nowhere', 'knowledge/source.md', undefined],
    ['/', 'knowledge/source.md', undefined],
    ['', 'knowledge/source.md', undefined],
    // The native resolver removes the extension before trimming slashes.
    ['/telos/goals.md/', 'knowledge/source.md', undefined],
  ])('resolves %s from %s to %s', (target, from, expected) => {
    expect(createLibraryResolver(pages)(target, from)).toBe(expected)
  })
})
