/**
 * Bead ids, wherever they are written.
 *
 * An id is a project prefix, a short tail, and a dotted number for each level of
 * nesting: `chrote-5grx`, `chrote-5grx.15`, `ctx-t4ak`. Terminal output, a
 * Bead's own description and a commit message all spell them the same way, so
 * one matcher serves the terminal's link provider and the card's Markdown
 * alike.
 *
 * Which prefixes exist is a fact about the host, learned from the projects
 * route. Until it answers, the two prefixes this host has always had are
 * assumed: a link that opens the card is worth more than a link that waits.
 */

import { fetchBeadProjectIdentities, type BeadProject } from './beadsApi'

export const FALLBACK_BEAD_PREFIXES: readonly string[] = ['chrote', 'ctx']

let projects: BeadProject[] = []
let prefixes: readonly string[] = FALLBACK_BEAD_PREFIXES
let manualPaths: readonly string[] = []
let catalogKey: string | null = null
let generation = 0
let loading: { key: string; request: Promise<BeadProject[]> } | null = null

function pathKey(paths: readonly string[]): string {
  return JSON.stringify([...new Set(paths.map(path => path.trim()).filter(Boolean))].sort())
}

function escapeForPattern(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * The id shape for a set of prefixes, anchored so that a longer word ending in
 * an id — a path, a branch name — is not mistaken for one.
 */
export function beadIdPattern(known: readonly string[] = beadPrefixes()): RegExp {
  const alternatives = known.filter(prefix => prefix.trim() !== '').map(escapeForPattern).join('|')
  return new RegExp(`(?<![\\w-])(?:${alternatives})-[a-z0-9]{3,6}(?:\\.\\d+)*(?![\\w-])`, 'g')
}

export interface BeadIdMatch {
  id: string
  index: number
}

/** Every Bead id in a line of text, with where it starts. */
export function findBeadIds(text: string, known: readonly string[] = beadPrefixes()): BeadIdMatch[] {
  if (known.length === 0) return []
  const pattern = beadIdPattern(known)
  const found: BeadIdMatch[] = []
  for (const match of text.matchAll(pattern)) {
    if (match.index === undefined) continue
    found.push({ id: match[0], index: match.index })
  }
  return found
}

export function beadPrefixes(): readonly string[] {
  return prefixes
}

export function beadProjects(): readonly BeadProject[] {
  return projects
}

/** The project a Bead id belongs to, by its prefix; the longest one wins. */
export function beadProjectPath(id: string): string | null {
  const owning = projects
    .filter(project => project.prefix && id.startsWith(`${project.prefix}-`))
    .sort((a, b) => (b.prefix as string).length - (a.prefix as string).length)
  return owning[0]?.path ?? null
}

export function setBeadProjects(known: readonly BeadProject[]): void {
  generation += 1
  catalogKey = pathKey(manualPaths)
  projects = [...known]
  const found = projects.map(project => project.prefix).filter((prefix): prefix is string => !!prefix)
  prefixes = found.length > 0 ? found : FALLBACK_BEAD_PREFIXES
}

/**
 * The catalog owner refreshes on startup/configuration changes and Beads first
 * use/refresh. A card with an unknown store can retry a failed load.
 */
export function ensureBeadProjects(paths: readonly string[] = manualPaths): Promise<BeadProject[]> {
  if (catalogKey === pathKey(paths)) return Promise.resolve(projects)
  return refreshBeadProjects(paths).catch(() => [])
}

export function refreshBeadProjects(paths: readonly string[]): Promise<BeadProject[]> {
  const key = pathKey(paths)
  manualPaths = JSON.parse(key) as string[]
  if (loading?.key === key) return loading.request
  const current = ++generation
  const request = fetchBeadProjectIdentities(manualPaths).then(found => {
    // Missing prefixes can mean a failed one-issue read. Keep a known prefix
    // for a still-listed path; removed stores disappear and new prefixes win.
    if (generation === current) setBeadProjects(found.map(project => {
      const knownPrefix = projects.find(known => known.path === project.path)?.prefix
      return !project.prefix && knownPrefix ? { ...project, prefix: knownPrefix } : project
    }))
    return found
  })
  loading = { key, request }
  const settled = () => { if (loading?.request === request) loading = null }
  void request.then(settled, settled)
  return request
}

export function resetBeadProjectsForTest(): void {
  projects = []
  prefixes = FALLBACK_BEAD_PREFIXES
  manualPaths = []
  catalogKey = null
  generation += 1
  loading = null
}
