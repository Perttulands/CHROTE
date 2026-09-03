/**
 * Asking what an agent sees, from anywhere.
 *
 * The way in is a session's own menu — its row in the Sessions panel and its
 * tag in a tile — and neither is a React parent of the panel that answers. The
 * request travels as a fact the panel subscribes to, the way the Bead card's
 * does, so a menu row stays one call with no props threaded through the tree.
 */

import { useSyncExternalStore } from 'react'
import type { AgentHarness } from './agentContextApi'

export interface AgentContextRequest {
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
  /** Each request is its own, so the same session can be asked about twice. */
  nonce: number
}

let request: AgentContextRequest | null = null
let nonce = 0
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

export function openAgentContext(next: Omit<AgentContextRequest, 'nonce'>): void {
  nonce += 1
  request = { ...next, nonce }
  publish()
}

export function closeAgentContext(): void {
  if (request === null) return
  request = null
  publish()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function read(): AgentContextRequest | null {
  return request
}

export function useAgentContextRequest(): AgentContextRequest | null {
  return useSyncExternalStore(subscribe, read, read)
}

export function resetAgentContextForTest(): void {
  request = null
  nonce = 0
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
