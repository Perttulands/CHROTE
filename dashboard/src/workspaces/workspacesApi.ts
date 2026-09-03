/**
 * The workspaces of this host, as the server lists them.
 *
 * One request answers three surfaces: the Agents rail asks it which folders an
 * agent could be started in, the Beads rail reads the ones holding a store,
 * and the launcher offers them as suggestions. The server computes the list on
 * request from the live sessions, the configured Beads projects and a walk of
 * the roots; nothing is cached here, so a folder that appeared is on the next
 * answer.
 */

import { apiErrorMessage } from '../apiErrors'

/** Why a folder is on the list. One folder can be there for several reasons. */
export type WorkspaceSource = 'session' | 'beads' | 'git' | 'store'

export interface Workspace {
  path: string
  sources: WorkspaceSource[]
  /** The live sessions running in this folder. */
  sessions: string[]
  /** The prefix the store's Bead ids carry, when the folder holds a store. */
  beadsPrefix?: string
  /** How many Beads in the store are not closed. Absent when bd could not say. */
  openBeads?: number
  /** How many instruction files the folder owns itself. */
  instructions: number
  /** When a session here last saw input or output. Absent without a session. */
  lastActivity?: string
}

const REQUEST_TIMEOUT_MS = 30000

export interface FetchWorkspacesOptions {
  /**
   * Ask bd about each store for its prefix and open count. One process per
   * store on the server, so only the surfaces that read Beads ask for it.
   */
  beads?: boolean
  signal?: AbortSignal
}

export async function fetchWorkspaces({ beads = false, signal }: FetchWorkspacesOptions = {}): Promise<Workspace[]> {
  const url = beads ? '/api/workspaces?beads=1' : '/api/workspaces'
  const response = await fetch(url, { signal: signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS) })
  if (!response.ok) throw new Error(apiErrorMessage(await response.text(), 'Could not list the workspaces'))
  const body = await response.json() as unknown
  return Array.isArray(body) ? body as Workspace[] : []
}

/** A workspace a live session runs in. */
export function isRunning(workspace: Workspace): boolean {
  return workspace.sessions.length > 0
}

/** A workspace holding a Beads store bd can read. */
export function holdsStore(workspace: Workspace): boolean {
  return workspace.sources.includes('store')
}

/** The last segment of a path, which is what tells two workspaces apart. */
export function workspaceName(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  return trimmed.slice(trimmed.lastIndexOf('/') + 1) || path
}
