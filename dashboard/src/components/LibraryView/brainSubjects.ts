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

/** Every page one link or one shared tag away, in the order the graph names them. */
export function neighboursOf(graph: LibraryGraph, path: string): string[] {
  const found: string[] = []
  const seen = new Set<string>([path])
  const add = (other: string) => {
    if (seen.has(other)) return
    seen.add(other)
    found.push(other)
  }
  graph.links.forEach(([from, to]) => {
    if (from === path) add(to)
    else if (to === path) add(from)
  })
  graph.tags.forEach(([from, to]) => {
    if (from === path) add(to)
    else if (to === path) add(from)
  })
  return found
}
