/**
 * Bead ids in terminal output are links.
 *
 * An agent prints the id of what it is working on; the operator reads it and
 * wants the Bead, not a search. Activation opens the card beside the session
 * that named it — never a new tab, because the id is not a URL and there is
 * nowhere else to go.
 */

import type { ILink, ILinkProvider, Terminal } from '@xterm/xterm'
import { findBeadIds } from '../beads/beadIds'
import { openBeadCard } from '../beads/beadCard'

/**
 * The ids on one line of the buffer, as xterm ranges. Columns are 1-based and
 * the line is read as it is drawn, so what the operator clicks is what matched.
 */
export function beadLinksOnLine(line: string, bufferLineNumber: number): ILink[] {
  return findBeadIds(line).map(({ id, index }) => ({
    text: id,
    range: {
      start: { x: index + 1, y: bufferLineNumber },
      end: { x: index + id.length, y: bufferLineNumber },
    },
    activate: () => openBeadCard(id),
  }))
}

export function createBeadLinkProvider(terminal: Terminal): ILinkProvider {
  return {
    provideLinks(bufferLineNumber, callback) {
      const line = terminal.buffer.active.getLine(bufferLineNumber - 1)
      if (!line) {
        callback(undefined)
        return
      }
      const links = beadLinksOnLine(line.translateToString(true), bufferLineNumber)
      callback(links.length > 0 ? links : undefined)
    },
  }
}
