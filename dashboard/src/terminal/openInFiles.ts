/**
 * Asking for a path in Files, from anywhere.
 *
 * A path is a link in a terminal, and the terminal is not React: the request
 * travels as a fact the app subscribes to, the way a Bead id's does, and the
 * app hands it to the Files panel of the active terminal workspace or, on any
 * other tab, to the Files tab.
 */

import { useSyncExternalStore } from 'react'

export interface OpenInFilesRequest {
  path: string
  /** Each request is its own, so the same path can be asked for twice. */
  nonce: number
}

let request: OpenInFilesRequest | null = null
let nonce = 0
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

export function openInFiles(path: string): void {
  nonce += 1
  request = { path, nonce }
  publish()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function read(): OpenInFilesRequest | null {
  return request
}

export function useOpenInFilesRequest(): OpenInFilesRequest | null {
  return useSyncExternalStore(subscribe, read, read)
}

export function resetOpenInFilesForTest(): void {
  request = null
  nonce = 0
}
