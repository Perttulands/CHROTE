/**
 * Asking what an agent sees, from anywhere.
 *
 * The way in is a session's own menu — its row in the Sessions panel and its
 * tag in a tile — and neither is a React parent of the panel that answers. The
 * question goes on the table, the way a Bead does, so a menu row stays one
 * call with no props threaded through the tree.
 */

import {
  clearTable,
  putOnTable,
  readTable,
  resetTableForTest,
  useTableObject,
  type AgentContextOnTable,
} from '../context/TableContext'
import type { AgentHarness } from './agentContextApi'

export type AgentContextRequest = AgentContextOnTable

export function openAgentContext(next: Omit<AgentContextRequest, 'nonce' | 'kind'>): void {
  putOnTable({ kind: 'agent-context', ...next })
}

export function closeAgentContext(): void {
  if (readTable()?.kind !== 'agent-context') return
  clearTable()
}

export function useAgentContextRequest(): AgentContextRequest | null {
  const object = useTableObject()
  return object?.kind === 'agent-context' ? object : null
}

export function resetAgentContextForTest(): void {
  resetTableForTest()
}

/**
 * Which harness a pane's command is, and whether it is one at all.
 *
 * A session running a shell has no stack of its own to show; the panel says so
 * and shows the folder's stack under the default harness, because the operator
 * asked about the folder as much as about the process.
 */
export function harnessOfCommand(command: string | undefined): { harness: AgentHarness; shell: boolean } {
  const name = (command ?? '').toLowerCase()
  if (name.includes('codex')) return { harness: 'codex', shell: false }
  if (name.includes('claude')) return { harness: 'claude-code', shell: false }
  return { harness: 'claude-code', shell: true }
}
