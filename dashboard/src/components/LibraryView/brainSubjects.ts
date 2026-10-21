import type { LibraryGraph, LibraryGraphPage } from '../../library/libraryApi'

export function subjectOf(page: LibraryGraphPage): string {
  if (page.subject) return page.subject
  const parts = page.path.split('/')
  return parts.length > 2 ? parts.slice(0, 2).join('/') : page.shelf
}

/**
 * The graph with each record placed in the subject it was filed for.
 *
 * Records are kept by who produced them (`evidence/sources/image-to-3d/…`,
 * `evidence/desk/korpi/…`), so the folder under the producer names the
 * subject the record supports. Where another collection has a subject of that
 * name, the record is drawn and counted with it; a record filed under a name
 * no subject has stays with its producer, where it shows as unfiled.
 */
export function withHomes(graph: LibraryGraph): LibraryGraph {
  const subjects = new Map<string, string>()
  for (const page of graph.pages) {
    const id = subjectOf(page)
    const [collection, name] = id.split('/')
    if (!name) continue
    const held = subjects.get(name)
    if (!held || HOME_ORDER(collection) < HOME_ORDER(held.split('/')[0])) subjects.set(name, id)
  }
  let moved = false
  const pages = graph.pages.map(page => {
    const parts = page.path.split('/')
    const home = parts.length >= 4 ? subjects.get(parts[2]) : undefined
    if (!home || home.split('/')[0] === parts[0]) return page
    moved = true
    return { ...page, subject: home }
  })
  return moved ? { ...graph, pages } : graph
}

/** Which collection a filed record joins when several share the name. */
const HOME_ORDER = (collection: string) => {
  const at = ['knowledge', 'projects', 'self'].indexOf(collection)
  return at === -1 ? 3 : at
}

export function subjectName(subject: string): string {
  const parts = subject.split('/')
  const name = parts[parts.length - 1] ?? subject
  return name.replace(/[-_]/g, ' ')
}

export function kindOf(page: LibraryGraphPage): string {
  return page.kind || 'note'
}

export interface BrainSubject {
  id: string
  name: string
  collection: string
  pages: LibraryGraphPage[]
  kinds: Map<string, number>
}

export function subjectsOf(pages: LibraryGraphPage[]): BrainSubject[] {
  const groups = new Map<string, BrainSubject>()
  for (const page of pages) {
    const id = subjectOf(page)
    if (!id) continue
    let group = groups.get(id)
    if (!group) {
      group = { id, name: subjectName(id), collection: id.includes('/') ? id.split('/')[0] : page.shelf, pages: [], kinds: new Map() }
      groups.set(id, group)
    }
    group.pages.push(page)
    const kind = kindOf(page)
    group.kinds.set(kind, (group.kinds.get(kind) ?? 0) + 1)
  }
  return [...groups.values()].sort((a, b) => a.id.localeCompare(b.id))
}

/** A subject keeps its colour when another subject is added or removed. */
export function subjectHue(id: string, palette: readonly string[]): string {
  let hash = 0
  for (const char of id) hash = (hash * 31 + char.charCodeAt(0)) >>> 0
  return palette[hash % palette.length] ?? 'var(--accent)'
}

/** Counts explicit cross-subject references; shared facets are not links. */
export function subjectConnections(graph: LibraryGraph, subject: string, selected: string | null) {
  const pages = new Map(graph.pages.map(page => [page.path, page]))
  const counts = new Map<string, number>()
  for (const [from, to] of graph.links) {
    if (selected && from !== selected && to !== selected) continue
    const a = pages.get(from)
    const b = pages.get(to)
    if (!a || !b) continue
    const own = subjectOf(a)
    const other = subjectOf(b)
    if (own === other) continue
    const id = own === subject ? other : other === subject ? own : null
    if (id) counts.set(id, (counts.get(id) ?? 0) + 1)
  }
  return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
}

/** Local pages and their real links; cross-subject references stay in the connection list. */
export function subjectGraph(graph: LibraryGraph, subject: string): LibraryGraph {
  const included = new Set(graph.pages.filter(page => subjectOf(page) === subject).map(page => page.path))
  return {
    pages: graph.pages.filter(page => included.has(page.path)).map(page => ({
      ...page,
      shelf: kindOf(page),
    })),
    links: graph.links.filter(([from, to]) => included.has(from) && included.has(to)),
    tags: [],
  }
}
