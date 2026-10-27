/**
 * The Files tab's home: the projects on this host, not the filesystem.
 *
 * A project is a workspace as the server lists it (a git root or a Beads store
 * under a root or a home, or any folder a session runs in), grouped by the
 * folder it lives in, so two homes and /srv read as three short lists. Within
 * a group the projects agents are working in come first, then the rest by
 * name. The roots close the list, so a folder that is nobody's project is
 * still a press away. The panel's search field filters them by name or path;
 * Enter or Down takes the operator into the first, and a press on a row opens
 * it.
 */

import { useMemo } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import { workspaceName, type Workspace } from '../../workspaces/workspacesApi'
import { libraryWhen } from '../../library/libraryApi'
import type { FileItem } from './types'

export interface ProjectGroup {
  /** The folder the projects live in. */
  parent: string
  projects: Workspace[]
}

function parentOf(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  const at = trimmed.lastIndexOf('/')
  return at <= 0 ? '/' : trimmed.slice(0, at)
}

/** The projects a query keeps, grouped by the folder they live in. */
export function projectGroups(workspaces: readonly Workspace[], query: string): ProjectGroup[] {
  const needle = query.trim().toLowerCase()
  const groups = new Map<string, Workspace[]>()
  for (const workspace of workspaces) {
    if (needle && !workspace.path.toLowerCase().includes(needle)) continue
    const parent = parentOf(workspace.path)
    const group = groups.get(parent)
    if (group) group.push(workspace)
    else groups.set(parent, [workspace])
  }
  const active = (workspace: Workspace) => workspace.lastActivity ?? ''
  return [...groups]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([parent, projects]) => ({
      parent,
      projects: projects.sort((a, b) => active(b).localeCompare(active(a)) || workspaceName(a.path).localeCompare(workspaceName(b.path))),
    }))
}

/** The project a path is in: the deepest workspace at or above it. */
export function projectOf(workspaces: readonly Workspace[], path: string): Workspace | null {
  let found: Workspace | null = null
  for (const workspace of workspaces) {
    const root = workspace.path.replace(/\/+$/, '')
    if ((path === root || path.startsWith(`${root}/`)) && (!found || root.length > found.path.length)) found = workspace
  }
  return found
}

/** What a project holds, in one quiet line. */
export function projectFacts(workspace: Workspace): string[] {
  const facts: string[] = []
  if (workspace.sessions.length > 0) facts.push(`${workspace.sessions.length} live`)
  if (workspace.openBeads !== undefined && workspace.openBeads > 0) facts.push(`${workspace.openBeads} open`)
  if (workspace.lastActivity) facts.push(libraryWhen(workspace.lastActivity))
  return facts
}

interface FilesProjectsProps {
  workspaces: readonly Workspace[] | null
  /** The folders at the top of the configured roots, for what is nobody's project. */
  roots: readonly FileItem[] | null
  error: string | null
  query: string
  onOpen: (path: string) => void
}

export default function FilesProjects({ workspaces, roots, error, query, onOpen }: FilesProjectsProps) {
  const groups = useMemo(() => projectGroups(workspaces ?? [], query), [query, workspaces])
  const needle = query.trim().toLowerCase()
  const shownRoots = (roots ?? []).filter(root => !needle || root.path.toLowerCase().includes(needle))

  // Up and Down walk the rows the way they walk find's results.
  const walk = (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    event.preventDefault()
    const rows = Array.from(event.currentTarget.closest('.files-projects')?.querySelectorAll<HTMLButtonElement>('.files-project') ?? [])
    rows[rows.indexOf(event.currentTarget) + (event.key === 'ArrowDown' ? 1 : -1)]?.focus()
  }

  if (error) return <p className="files-panel-note">{error}</p>
  if (!workspaces) return <p className="files-panel-note">Reading the projects…</p>
  return (
    <div className="files-projects" aria-label="Projects">
      {groups.map(group => (
        <section key={group.parent} className="files-project-group" role="list" aria-label={group.parent}>
          <h3 className="files-project-parent" role="presentation">{group.parent}</h3>
          {group.projects.map(project => (
            <button
              type="button"
              role="listitem"
              key={project.path}
              className={`files-project${project.sessions.length > 0 ? ' is-live' : ''}`}
              title={project.path}
              onClick={() => onOpen(project.path)}
              onKeyDown={walk}
            >
              <span className="files-project-name">{workspaceName(project.path)}</span>
              <span className="files-project-facts">{projectFacts(project).join(' · ')}</span>
            </button>
          ))}
        </section>
      ))}
      {groups.length === 0 && (
        <p className="files-panel-note">{needle ? 'No project matches' : 'No projects under the configured roots'}</p>
      )}
      {shownRoots.length > 0 && (
        <section className="files-project-group" role="list" aria-label="Roots">
          <h3 className="files-project-parent" role="presentation">Roots</h3>
          {shownRoots.map(root => (
            <button type="button" role="listitem" key={root.path} className="files-project" title={root.path} onClick={() => onOpen(root.path)} onKeyDown={walk}>
              <span className="files-project-name">{root.path}</span>
            </button>
          ))}
        </section>
      )}
    </div>
  )
}
