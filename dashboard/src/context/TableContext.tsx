/**
 * The table: the one selected object, global across tabs.
 *
 * A Bead opened from the map, a terminal link or another Bead's text; what an
 * agent sees, asked from a session's menu; a file. Whatever the operator last
 * picked up is on the table, and every tab shows it in the same column at its
 * right edge, so switching tabs never loses the thing in hand. Putting a new
 * object down replaces the old one: there is one table and it holds one thing.
 *
 * The selection lives at module level rather than in React state because the
 * terminal's link provider is not React and has to put a Bead down too; React
 * subscribes through `useTableObject`. The column's width is device-local and
 * lives with the other UserSettings. The tab-level actions the contents need
 * (Open in Beads) come through a React context that App provides.
 */

import { createContext, useContext, useSyncExternalStore } from 'react'
import type { ReactNode } from 'react'
import type { AgentHarness } from '../agents/agentContextApi'
import { beadReference } from '../beads/beadReference'
import { getSessionNameFromKey } from '../types'

export interface BeadOnTable {
  kind: 'bead'
  id: string
  /** The store the id belongs to, when the caller already knows it. */
  projectPath?: string
  /** The Bead's title, from the row that put it down or the card that read it. */
  title?: string
  /** The ids read before this one in the same sitting, oldest first. */
  trail: readonly string[]
  /** Each request is its own, so the same id can be asked for twice. */
  nonce: number
}

export interface AgentContextOnTable {
  kind: 'agent-context'
  /** The session the operator asked about, as the list keys it. */
  sessionKey: string
  /** The folder the session runs in; the stack is resolved for this. */
  folder: string
  /** The harness the pane is running, as the command names it. */
  harness: AgentHarness
  /** The Unix user the session belongs to, whose home the stack starts in. */
  user: string
  /** True when the pane is a shell rather than an agent. */
  shell: boolean
  nonce: number
}

export interface FileOnTable {
  kind: 'file'
  path: string
  nonce: number
}

export type TableObject = BeadOnTable | AgentContextOnTable | FileOnTable

/** What a caller puts down: the object without the nonce the store assigns. */
export type TableRequest =
  | (Omit<BeadOnTable, 'nonce' | 'trail'> & { trail?: readonly string[] })
  | Omit<AgentContextOnTable, 'nonce'>
  | Omit<FileOnTable, 'nonce'>

/** The column's width with nothing remembered, and the least it can be. */
export const TABLE_WIDTH_DEFAULT = 400
export const TABLE_WIDTH_MIN = 320
/** What the content beside the table keeps before the table starts to give. */
export const TABLE_CONTENT_MIN = 480
/** Below this the tab has no column to give, so the table overlays it. */
export const TABLE_DOCK_MIN_VIEWPORT = 900

let selected: TableObject | null = null
let nonce = 0
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

export function putOnTable(request: TableRequest): void {
  nonce += 1
  selected = request.kind === 'bead'
    ? { ...request, trail: request.trail ?? [], nonce }
    : { ...request, nonce }
  publish()
}

export function clearTable(): void {
  if (selected === null) return
  selected = null
  publish()
}

/**
 * Give the Bead on the table its title once the card has read it. A Bead put
 * down from a terminal link or an id in another Bead's text arrives with its
 * id alone, and the reference an agent is handed should carry the title too.
 */
export function nameBeadOnTable(id: string, title: string): void {
  if (selected?.kind !== 'bead' || selected.id !== id || selected.title === title) return
  selected = { ...selected, title }
  publish()
}

/** Back along a Bead's trail. False when there is nothing behind to go to. */
export function stepBackOnTable(): boolean {
  if (selected?.kind !== 'bead' || selected.trail.length === 0) return false
  const trail = selected.trail.slice(0, -1)
  putOnTable({ kind: 'bead', id: selected.trail[selected.trail.length - 1], projectPath: selected.projectPath, trail })
  return true
}

/**
 * Escape's meaning for the table: a Bead reached from another Bead returns to
 * it, and only a Bead with no trail behind it, or anything else, is put away.
 */
export function dismissTable(): void {
  if (!stepBackOnTable()) clearTable()
}

export function readTable(): TableObject | null {
  return selected
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

export function useTableObject(): TableObject | null {
  return useSyncExternalStore(subscribe, readTable, readTable)
}

export function resetTableForTest(): void {
  selected = null
  nonce = 0
  publish()
}

/** How the column names what is on it, for the reader who cannot see it. */
export function tableLabel(object: TableObject): string {
  switch (object.kind) {
    case 'bead': return `Bead ${object.id}`
    case 'agent-context': return `What ${getSessionNameFromKey(object.sessionKey)} sees`
    case 'file': return object.path
  }
}

/**
 * The one line that names what is on the table to an agent: the same words
 * the drawer puts first in a message, so a resident handed the table's object
 * reads it the way any other session would.
 */
export function tableReference(object: TableObject): string {
  switch (object.kind) {
    case 'bead': return beadReference(object)
    case 'agent-context': return `agents ${object.folder} ${object.harness}`
    case 'file': return `path ${object.path}`
  }
}

/**
 * A remembered width is trusted only as far as it is a width: anything else
 * is the default, and nothing narrower than the minimum is honoured.
 */
export function clampTableWidth(width: unknown): number {
  if (typeof width !== 'number' || !Number.isFinite(width)) return TABLE_WIDTH_DEFAULT
  return Math.max(TABLE_WIDTH_MIN, Math.round(width))
}

export interface TableActions {
  /** Show a Bead in the Beads tab, where its project and its map are. */
  openInBeads?: (projectPath: string, id: string) => void
}

const TableActionsContext = createContext<TableActions>({})

export function TableProvider({ openInBeads, children }: TableActions & { children: ReactNode }) {
  return <TableActionsContext.Provider value={{ openInBeads }}>{children}</TableActionsContext.Provider>
}

export function useTableActions(): TableActions {
  return useContext(TableActionsContext)
}
