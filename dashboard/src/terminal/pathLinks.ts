/**
 * Absolute paths in terminal output are links.
 *
 * Agents print the file they changed, the test that failed, the log they
 * wrote, the screenshot they took. Activation hands the path to Files, which
 * already knows how to open one and how to say plainly when it is not there
 * or not readable — or, for a picture, to the image glance; nothing is
 * checked here, and nothing is asked of the server on hover. A path is
 * matched by shape alone: a slash that begins a word, then segments. The
 * slash inside a URL follows a letter or another slash, so URLs stay with the
 * web-links addon.
 */

import type { ILink, ILinkProvider, Terminal } from '@xterm/xterm'
import { openInFiles } from './openInFiles'
import { openImageGlance } from '../components/imageGlance'

/** The extensions a path link shows as a picture rather than opening in Files. */
const IMAGE_EXTENSIONS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'avif'])

export function isImagePath(path: string): boolean {
  const name = path.slice(path.lastIndexOf('/') + 1)
  const dot = name.lastIndexOf('.')
  return dot > 0 && IMAGE_EXTENSIONS.has(name.slice(dot + 1).toLowerCase())
}

/** Where a path link goes: a picture to the glance, everything else to Files. */
export function activatePath(path: string): void {
  if (isImagePath(path)) openImageGlance(path)
  else openInFiles(path)
}

// A leading slash that follows nothing path-like, then segments of the
// characters file names are made of. The characters that end a sentence or
// close a bracket are trimmed off the tail afterwards, as is a `:12` line.
const PATH_PATTERN = /(?<![\w./~:@%+-])\/(?:[\w.@%+~-]+\/)*[\w.@%+~-]*/g
const TRAILING = /[.,;:)\]}'"/]+$/

/** The absolute paths on one line, with where each starts. */
export function findPaths(line: string): { path: string; index: number }[] {
  const found: { path: string; index: number }[] = []
  for (const match of line.matchAll(PATH_PATTERN)) {
    const path = match[0].replace(TRAILING, '')
    // A slash on its own is not somewhere to go.
    if (path.length > 1) found.push({ path, index: match.index })
  }
  return found
}

/**
 * The paths on one line of the buffer, as xterm ranges. Columns are 1-based
 * and the line is read as it is drawn, so what the operator clicks is what
 * matched.
 */
export function pathLinksOnLine(line: string, bufferLineNumber: number): ILink[] {
  return findPaths(line).map(({ path, index }) => ({
    text: path,
    range: {
      start: { x: index + 1, y: bufferLineNumber },
      end: { x: index + path.length, y: bufferLineNumber },
    },
    activate: () => activatePath(path),
  }))
}

export function createPathLinkProvider(terminal: Terminal): ILinkProvider {
  return {
    provideLinks(bufferLineNumber, callback) {
      const line = terminal.buffer.active.getLine(bufferLineNumber - 1)
      if (!line) {
        callback(undefined)
        return
      }
      const links = pathLinksOnLine(line.translateToString(true), bufferLineNumber)
      callback(links.length > 0 ? links : undefined)
    },
  }
}
