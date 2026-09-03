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

import { fetchBeadProjects, type BeadProject } from './beadsApi'

export const FALLBACK_BEAD_PREFIXES: readonly string[] = ['chrote', 'ctx']

let projects: BeadProject[] = []
let prefixes: readonly string[] = FALLBACK_BEAD_PREFIXES
let loading: Promise<BeadProject[]> | null = null

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
  projects = [...known]
  const found = projects.map(project => project.prefix).filter((prefix): prefix is string => !!prefix)
  prefixes = found.length > 0 ? found : FALLBACK_BEAD_PREFIXES
}

/**
 * Learn the projects once. The card needs them to know which store to ask, and
 * the terminal's links sharpen from the fallback prefixes to the real ones as
 * soon as they land. A refusal leaves the fallback in place rather than
 * retrying: the operator opening a Bead is the retry.
 */
export function ensureBeadProjects(manualPaths: readonly string[] = []): Promise<BeadProject[]> {
  if (projects.length > 0) return Promise.resolve(projects)
  if (!loading) {
    loading = fetchBeadProjects(manualPaths)
      .then(found => {
        setBeadProjects(found)
        return found
      })
      .catch(() => {
        loading = null
        return []
      })
  }
  return loading
}

export function resetBeadProjectsForTest(): void {
  projects = []
  prefixes = FALLBACK_BEAD_PREFIXES
  loading = null
}
