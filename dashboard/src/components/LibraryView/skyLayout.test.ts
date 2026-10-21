import { describe, expect, it } from 'vitest'
import type { LibraryGraphPage } from '../../library/libraryApi'
import { withHomes } from './brainSubjects'
import { placementOf, skyLayout, type SkyLayout } from './skyLayout'

const page = (path: string, kind = 'note'): LibraryGraphPage => ({
  path, title: path, shelf: path.split('/')[0], subject: '', kind, slug: path.split('/').pop()?.replace('.md', ''),
  words: 100, created: '', updated: '', candidate: false,
})

// Eight subjects, each with a topic, linked notes, evidence and one stray.
const pages = Array.from({ length: 8 }, (_, subject) => [
  page(`knowledge/s${subject}/s${subject}.md`, 'topic'),
  ...Array.from({ length: 6 }, (_, index) => page(`knowledge/s${subject}/n${index}.md`)),
  ...Array.from({ length: 10 }, (_, index) => page(`knowledge/s${subject}/sources/r${index}.md`, 'source')),
  page(`knowledge/s${subject}/stray.md`),
]).flat()
const links: [string, string][] = pages
  .filter(entry => /\/n\d\.md$/.test(entry.path))
  .flatMap(entry => {
    const subject = Number(/s(\d)/.exec(entry.path)?.[1])
    return [
      [entry.path, `knowledge/s${subject}/s${subject}.md`] as [string, string],
      [entry.path, `knowledge/s${(subject + 1) % 8}/s${(subject + 1) % 8}.md`] as [string, string],
    ]
  })

const positions = (layout: SkyLayout) => layout.stars.map(({ page: { path }, x, y }) => [path, x, y])

function expectSeparate(layout: SkyLayout) {
  for (const region of layout.regions) {
    for (const other of layout.regions.filter(candidate => candidate !== region)) {
      expect(Math.hypot(region.x - other.x, region.y - other.y)).toBeGreaterThan(region.radius + other.radius)
    }
  }
}

describe('the night sky', () => {
  it('keeps systems apart, its topic at the centre, the rest inside and strays just outside', () => {
    const layout = skyLayout({ pages, links })
    expect(layout.regions).toHaveLength(8)
    expectSeparate(layout)
    for (const region of layout.regions) {
      expect(region.core).toBe(`${region.id}/${region.id.split('/')[1]}.md`)
      for (const star of layout.stars.filter(candidate => candidate.subject === region.id)) {
        const distance = Math.hypot(star.x - region.x, star.y - region.y)
        if (star.role === 'stray') expect(distance).toBeGreaterThan(region.radius)
        else expect(distance).toBeLessThan(region.radius)
      }
    }
    expect(layout.stars.filter(star => star.role === 'dust')).toHaveLength(80)
  })

  it('is the same sky whatever order the corpus arrives in', () => {
    expect(positions(skyLayout({ pages: [...pages].reverse(), links: [...links].reverse() })))
      .toEqual(positions(skyLayout({ pages, links })))
  })

  it('keeps every system in place when a page arrives, given the sky before', () => {
    const before = skyLayout({ pages, links })
    const grown = [...pages, ...Array.from({ length: 30 }, (_, index) => page(`knowledge/s3/sources/new${index}.md`, 'source'))]
    const after = skyLayout({ pages: grown, links }, placementOf(before))
    expectSeparate(after)
    const moved = after.regions.map(region => {
      const was = before.regions.find(candidate => candidate.id === region.id)
      return was ? Math.hypot(region.x - was.x, region.y - was.y) : 0
    })
    // Only the grown system and its touching neighbours give way, and only by
    // as much as it grew.
    const grownBy = (after.regions.find(region => region.id === 'knowledge/s3')?.radius ?? 0)
      - (before.regions.find(region => region.id === 'knowledge/s3')?.radius ?? 0)
    expect(Math.max(...moved)).toBeLessThanOrEqual(grownBy + 1)
  })

  it('puts a new subject beside the subjects it links to', () => {
    const before = skyLayout({ pages, links })
    const newcomer = page('knowledge/fresh/fresh.md', 'topic')
    const after = skyLayout({ pages: [...pages, newcomer], links: [...links, [newcomer.path, 'knowledge/s5/s5.md']] }, placementOf(before))
    expectSeparate(after)
    const fresh = after.regions.find(region => region.id === 'knowledge/fresh')
    const anchor = after.regions.find(region => region.id === 'knowledge/s5')
    const distances = after.regions.filter(region => region !== fresh)
      .map(region => Math.hypot(region.x - (fresh?.x ?? 0), region.y - (fresh?.y ?? 0)))
    expect(Math.hypot((anchor?.x ?? 0) - (fresh?.x ?? 0), (anchor?.y ?? 0) - (fresh?.y ?? 0))).toBe(Math.min(...distances))
  })
})

describe('records find their homes', () => {
  it('draws a record with the subject it was filed for, and leaves an unknown one with its producer', () => {
    const graph = withHomes({
      pages: [
        page('knowledge/image-to-3d/image-to-3d.md', 'topic'),
        page('projects/korpi/README.md'),
        page('evidence/sources/image-to-3d/paper.md', 'source'),
        page('evidence/desk/korpi/ask.md'),
        page('evidence/desk/plutus/ask.md'),
      ],
      links: [], tags: [],
    })
    const subjects = Object.fromEntries(graph.pages.map(entry => [entry.path, entry.subject]))
    expect(subjects['evidence/sources/image-to-3d/paper.md']).toBe('knowledge/image-to-3d')
    expect(subjects['evidence/desk/korpi/ask.md']).toBe('projects/korpi')
    expect(subjects['evidence/desk/plutus/ask.md']).toBe('')
  })
})
