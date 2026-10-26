/**
 * The Beads of a slice as a night sky, in the Brain's own terms.
 *
 * Each epic, or any Bead that has children, is a star system: the Bead at the
 * centre, the open work beneath it in orbit, and its finished work the nebula
 * around it, so how much of an epic is done shows as how much it glows. A Bead
 * filed under no parent is unfiled work, drifting loose around its project's
 * unfiled system. Parent and blocker links are the sky's lines, so work that
 * waits on other work sits with it.
 *
 * A system is named for its root Bead's title; its id is the project and the
 * root's id, so its place in the sky outlives a retitle.
 */

import type { LibraryGraph, LibraryGraphPage } from '../library/libraryApi'
import { beadTypeName } from './beadType'
import type { SliceIndex } from './beadsSlice'

const CLOSED = 4

/** The id under which an unfiled Bead's project gathers its loose work. */
const UNFILED = '~unfiled'

export interface BeadsSky {
  graph: LibraryGraph
  /** The sky's path for a row, and back. */
  pathOf: (row: number) => string
  rowOf: (path: string) => number
  /** A system's name: its root Bead's title, or Unfiled. */
  nameOf: (subject: string) => string | undefined
}

function iso(seconds: number | undefined): string {
  return seconds ? new Date(seconds * 1000).toISOString() : ''
}

/** The topmost Bead above a row, stopping at a cycle a store should not hold. */
function rootOf(index: SliceIndex, row: number): number {
  const seen = new Set<number>()
  let at = row
  while (index.parent[at] >= 0 && !seen.has(at)) {
    seen.add(at)
    at = index.parent[at]
  }
  return at
}

/** Every row the index holds, placed; `rows` are the ones a slice keeps. */
export function beadsSky(index: SliceIndex, rows: Iterable<number>): BeadsSky {
  const pathOf = (row: number) => `${index.projects[index.project[row]].key}:${index.rows[row].id}`
  const kept = [...rows]
  const placed = new Set(kept)
  const names = new Map<string, string>()
  const pages: LibraryGraphPage[] = kept.map(row => {
    const bead = index.rows[row]
    const root = rootOf(index, row)
    const project = index.projects[index.project[root]]
    const system = index.children[root].length > 0
    const subject = system ? `${project.key}/${index.rows[root].id}` : `${index.projects[index.project[row]].key}/${UNFILED}`
    names.set(subject, system ? index.rows[root].title : 'Unfiled')
    const centre = system && row === root
    return {
      path: pathOf(row),
      subject,
      shelf: index.projects[index.project[row]].label,
      slug: centre ? bead.id : undefined,
      // The centre is the system's topic; finished work is the dust it rests on.
      kind: centre ? 'topic' : index.status[row] === CLOSED ? 'source' : beadTypeName(bead.type),
      title: bead.title,
      words: bead.size ?? 0,
      created: iso(bead.created),
      updated: iso(bead.updated ?? bead.created),
      candidate: false,
    }
  })
  const links: [string, string][] = []
  for (const row of kept) {
    const parent = index.parent[row]
    if (parent >= 0 && placed.has(parent)) links.push([pathOf(row), pathOf(parent)])
    for (const blocker of index.blockers[row]) if (placed.has(blocker)) links.push([pathOf(row), pathOf(blocker)])
  }
  const rowByPath = new Map(kept.map(row => [pathOf(row), row]))
  return {
    graph: { pages, links, tags: [] },
    pathOf,
    rowOf: path => rowByPath.get(path) ?? -1,
    nameOf: subject => names.get(subject),
  }
}
