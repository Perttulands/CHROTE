import { describe, expect, it } from 'vitest'
import type { LibraryGraph, LibraryGraphPage } from '../../library/libraryApi'
import { neighboursOf } from './brainSubjects'

const page = (path: string) => ({ path, shelf: path.split('/')[0], title: path, words: 10, created: '', updated: '' }) as LibraryGraphPage

// The reader's Related list is this function's only reader, so what counts as
// one hop is worth pinning: a written link either way, a shared tag, each page once.
describe('neighboursOf', () => {
  it('gathers every page one written link or one shared tag away', () => {
    const corpus: LibraryGraph = {
      pages: ['knowledge/alpha.md', 'knowledge/beta.md', 'preferences/gamma.md', 'preferences/delta.md', 'telos/epsilon.md'].map(page),
      links: [['knowledge/alpha.md', 'preferences/gamma.md'], ['knowledge/beta.md', 'knowledge/alpha.md']],
      tags: [['preferences/gamma.md', 'preferences/delta.md', 'shared']],
    }
    expect(neighboursOf(corpus, 'knowledge/alpha.md')).toEqual(['preferences/gamma.md', 'knowledge/beta.md'])
    expect(neighboursOf(corpus, 'preferences/gamma.md')).toEqual(['knowledge/alpha.md', 'preferences/delta.md'])
    expect(neighboursOf(corpus, 'telos/epsilon.md')).toEqual([])
  })
})
