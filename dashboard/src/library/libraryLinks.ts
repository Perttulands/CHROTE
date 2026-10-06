import { shelfOf, type LibraryGraphPage } from './libraryApi'

/** The native libraryIndex rules, using the same ordered graph page identities. */
export function createLibraryResolver(pages: readonly Pick<LibraryGraphPage, 'path' | 'title'>[]) {
  const byPath = new Map<string, string>()
  const byName = new Map<string, string[]>()
  const byTitle = new Map<string, string[]>()
  const add = (index: Map<string, string[]>, key: string, path: string) => {
    const candidates = index.get(key) ?? []
    candidates.push(path)
    index.set(key, candidates)
  }
  for (const page of pages) {
    const stem = page.path.replace(/\.[^/.]*$/, '').toLowerCase()
    byPath.set(stem, page.path)
    add(byName, stem.slice(stem.lastIndexOf('/') + 1), page.path)
    add(byTitle, page.title.trim().toLowerCase(), page.path)
  }
  return (target: string, from: string): string | undefined => {
    let key = target.trim().toLowerCase()
    if (key.endsWith('.md')) key = key.slice(0, -3)
    key = key.replace(/^\/+|\/+$/g, '')
    if (!key) return undefined
    const path = byPath.get(key)
    if (path !== undefined) return path
    const candidates = byName.get(key.slice(key.lastIndexOf('/') + 1)) ?? byTitle.get(key)
    return candidates?.find(candidate => shelfOf(candidate) === shelfOf(from)) ?? candidates?.[0]
  }
}
