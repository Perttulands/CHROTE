/**
 * Asking to look at an image, from anywhere.
 *
 * An image path is a link in a terminal and a picture in the Files panel or
 * tab, and none of those is a React parent of the glance that shows it. The
 * request travels as a fact the glance subscribes to, the way the Bead card's
 * does.
 */

import { useSyncExternalStore } from 'react'

export interface ImageGlanceRequest {
  path: string
  /** Each request is its own, so the same image can be asked for twice. */
  nonce: number
}

let request: ImageGlanceRequest | null = null
let nonce = 0
const listeners = new Set<() => void>()

function publish(): void {
  listeners.forEach(listener => listener())
}

export function openImageGlance(path: string): void {
  nonce += 1
  request = { path, nonce }
  publish()
}

export function closeImageGlance(): void {
  if (request === null) return
  request = null
  publish()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function read(): ImageGlanceRequest | null {
  return request
}

export function useImageGlanceRequest(): ImageGlanceRequest | null {
  return useSyncExternalStore(subscribe, read, read)
}

export function resetImageGlanceForTest(): void {
  request = null
  nonce = 0
}
