/**
 * Asking for the Bead card, from anywhere.
 *
 * A Bead id is a link in a terminal, in a row of the map, and inside another
 * Bead's text. None of those places is a React parent of the card, and the
 * terminal is not React at all, so the request travels as a fact the card
 * subscribes to rather than as a prop passed down a tree.
 */

import { useSyncExternalStore } from 'react'

export interface BeadCardRequest {
  id: string
  /** The store the id belongs to, when the caller already knows it. */
  projectPath?: string
  /** Each request is its own, so the same id can be asked for twice. */
  nonce: number
}

let request: BeadCardRequest | null = null
let nonce = 0
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

export function openBeadCard(id: string, projectPath?: string): void {
  nonce += 1
  request = { id, projectPath, nonce }
  publish()
}

export function closeBeadCard(): void {
  if (request === null) return
  request = null
  publish()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function read(): BeadCardRequest | null {
  return request
}

export function useBeadCardRequest(): BeadCardRequest | null {
  return useSyncExternalStore(subscribe, read, read)
}

export function resetBeadCardForTest(): void {
  request = null
  nonce = 0
}
