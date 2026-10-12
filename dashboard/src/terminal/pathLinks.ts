/**
 * Absolute paths in terminal output are links.
 *
 * Agents print the file they changed, the test that failed, the log they
 * wrote, the screenshot they took. Activation hands the path to Files —
 * every path, pictures included, because the operator clicking one wants to
 * look at the file beside the terminal rather than over it. Files already
 * knows how to open one and how to say plainly when it is not there or not
 * readable; nothing is checked here, and nothing is asked of the server on
 * hover. A path is matched by shape alone: a slash that begins a word, then
 * segments. The slash inside a URL follows a letter or another slash, so URLs
 * stay with the web-links addon.
 */

import type { ILink, ILinkProvider, Terminal } from '@xterm/xterm'
import { openInFiles } from './openInFiles'

/** Where a path link goes: to Files, whatever kind of file it names. */
export function activatePath(path: string): void {
  openInFiles(path)
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

type Position = { x: number; y: number }
type PathMatch = { path: string; positions: Position[] }

// Bound hover work even for a terminal full of unbroken output. Nothing here
// rewrites the buffer: these are only text offsets and their original cells.
const ROW_RADIUS = 32
const ENCLOSED_WRAP = /([(["'])(\/[\w./@%+~-]+(?:\n +[\w./@%+~-]+)+)([)\]"'])/g
const CLOSING: Record<string, string> = { '(': ')', '[': ']', '"': '"', "'": "'" }

function bufferPaths(terminal: Terminal, requestedRow: number): PathMatch[] {
  const buffer = terminal.buffer.active
  const first = Math.max(0, requestedRow - 1 - ROW_RADIUS)
  const last = Math.min(buffer.length - 1, requestedRow - 1 + ROW_RADIUS)
  let text = ''
  const positions: Position[] = []
  const indents = new Map<number, string>()
  for (let y = first; y <= last; y++) {
    const line = buffer.getLine(y)!
    if (y > first && !line.isWrapped) {
      text += '\n'
      positions.push({ x: 0, y: y + 1 })
    }
    indents.set(y + 1, line.translateToString(true).match(/^ */)![0])
    const next = buffer.getLine(y + 1)
    let rowText = ''
    const rowPositions: Position[] = []
    for (let x = 0; x < Math.min(line.length, terminal.cols); x++) {
      const cell = line.getCell(x)!
      if (!cell.getWidth()) continue
      const chars = cell.getChars()
      // A wide character can wrap early, leaving an unused final cell.
      if (!chars && x === terminal.cols - 1 && next?.isWrapped && next.getCell(0)?.getWidth() === 2) continue
      const value = chars || ' '
      rowText += value
      for (let i = 0; i < value.length; i++) rowPositions.push({ x: x + 1, y: y + 1 })
    }
    // Spaces before a soft wrap are real separators; only trim hard rows.
    if (!next?.isWrapped) rowText = rowText.trimEnd()
    text += rowText
    positions.push(...rowPositions.slice(0, rowText.length))
  }

  const reconstructed: { start: number; end: number; match: PathMatch }[] = []
  for (const match of text.matchAll(ENCLOSED_WRAP)) {
    if (CLOSING[match[1]] !== match[3]) continue
    const start = match.index + 1
    const raw = match[2]
    const indent = indents.get(positions[start].y)
    // Hard breaks are inference, so require an enclosed, uninterrupted path,
    // right-margin breaks and the same nonempty indentation on every row.
    if (!indent) continue
    let valid = true
    for (const boundary of raw.matchAll(/\n( +)/g)) {
      if (boundary[1] !== indent || positions[start + boundary.index - 1].x !== terminal.cols) {
        valid = false
        break
      }
    }
    if (!valid) continue
    const joined = raw.replace(/\n +/g, '')
    const [found] = findPaths(joined)
    if (!found || joined.replace(TRAILING, '') !== found.path) continue
    const mapped = positions.slice(start, start + raw.length).filter((_, i) => !/\s/.test(raw[i]))
    reconstructed.push({ start, end: start + raw.length, match: { path: found.path, positions: mapped.slice(0, found.path.length) } })
  }

  const ordinary = findPaths(text)
    .filter(({ index, path }) => !reconstructed.some(({ start, end }) => index < end && index + path.length > start))
    // Do not offer a suffix or prefix cut off by the bounded scan window.
    .filter(({ index, path }) => !(index === 0 && buffer.getLine(first)?.isWrapped)
      && !(index + path.length === text.length && buffer.getLine(last + 1)?.isWrapped))
    .map(({ path, index }) => ({ path, positions: positions.slice(index, index + path.length) }))
  return [...ordinary, ...reconstructed.map(({ match }) => match)]
}

export function createPathLinkProvider(terminal: Terminal): ILinkProvider {
  return {
    provideLinks(bufferLineNumber, callback) {
      if (!terminal.buffer.active.getLine(bufferLineNumber - 1)) {
        callback(undefined)
        return
      }
      const links: ILink[] = []
      for (const { path, positions } of bufferPaths(terminal, bufferLineNumber)) {
        const cells = positions.filter(position => position.y === bufferLineNumber)
        if (!cells.length) continue
        // Each fragment activates the full path. Separate row ranges leave
        // application indentation and surrounding prose out of hit-testing.
        links.push({
          text: path,
          range: { start: cells[0], end: cells[cells.length - 1] },
          activate: () => activatePath(path),
        })
      }
      callback(links.length > 0 ? links : undefined)
    },
  }
}
