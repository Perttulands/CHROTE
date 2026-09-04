/**
 * The layout, off the thread the operator is on.
 *
 * Laying a corpus out is a fixed number of passes over every page, and past a
 * few hundred pages one run costs long enough that doing it where the
 * interface lives would stop the interface. So above that size the map asks
 * this worker instead: the same function, the same seeded arithmetic, the same
 * picture, computed while the tab stays answerable.
 *
 * The map keeps the last answer it was given until a new one arrives, so
 * nothing flickers between them.
 */

import { layoutMap, type MapLayout } from './mapLayout'
import type { LibraryGraph } from '../../library/libraryApi'

export interface MapLayoutRequest {
  token: number
  graph: LibraryGraph
  width: number
  height: number
  now: number
}

export interface MapLayoutAnswer {
  token: number
  layout: MapLayout
}

self.onmessage = (event: MessageEvent<MapLayoutRequest>) => {
  const { token, graph, width, height, now } = event.data
  const answer: MapLayoutAnswer = { token, layout: layoutMap(graph, width, height, now) }
  ;(self as unknown as Worker).postMessage(answer)
}
