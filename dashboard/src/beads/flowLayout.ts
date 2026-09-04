/**
 * The flow of an epic: waves left to right, parallel work stacked in a column.
 *
 * An epic is a process. What can start now stands in the first column; what
 * waits for it stands in the next; two pieces of work that wait for nothing in
 * common stand side by side, which is the whole point — the operator reads how
 * many lanes the epic really has before he hands it to agents. A column is a
 * topological level of the blocking edges, and nothing else moves a Bead: the
 * parent says where a Bead belongs, not when it can start, so it is drawn as a
 * band behind its children rather than as a position.
 *
 * The arithmetic is here, deterministic and without a DOM: the same rows lay
 * out the same way whatever order they arrive in, so the flow is a place the
 * operator learns rather than a picture that reshuffles under him. `bd` cannot
 * hold a cycle, but a store read mid-write could look like one, so a blocking
 * edge that closes a loop is broken rather than obeyed, and it is reported as
 * broken instead of being drawn as a wave.
 */

import { beadRowKey, type WorkRow } from './beadsTree'

/** A node's box, and the room a column and a lane take. */
export const NODE_WIDTH = 200
export const NODE_HEIGHT = 44
export const COLUMN_GAP = 64
export const LANE_GAP = 16
export const PADDING = 28

const COLUMN_STRIDE = NODE_WIDTH + COLUMN_GAP
const LANE_STRIDE = NODE_HEIGHT + LANE_GAP

/** How far a band is drawn outside the Beads it holds. */
const BAND_INSET = 7

export interface FlowNode {
  /** The row's identity across stores, which is what every edge names. */
  key: string
  row: WorkRow
  /** Which column: how many Beads must finish before this one can start. */
  wave: number
  /** Which row of that column. */
  lane: number
  x: number
  y: number
}

export interface FlowEdge {
  key: string
  from: string
  to: string
  x1: number
  y1: number
  x2: number
  y2: number
  /**
   * True when the edge closed a loop and was not obeyed. The line is still
   * drawn, because the operator should see the contradiction his store holds.
   */
  back: boolean
}

/** A parent's children, held together by a light band behind them. */
export interface FlowBand {
  /** The parent's id, written small above the band. */
  key: string
  x: number
  y: number
  width: number
  height: number
}

export interface FlowGraph {
  nodes: FlowNode[]
  edges: FlowEdge[]
  bands: FlowBand[]
  /** How many columns and how tall the tallest column is, for the fit. */
  waves: number
  width: number
  height: number
}

export const EMPTY_FLOW: FlowGraph = { nodes: [], edges: [], bands: [], waves: 0, width: 0, height: 0 }

function byPriorityThenId(a: WorkRow, b: WorkRow): number {
  if (a.priority !== b.priority) return a.priority - b.priority
  return a.id.localeCompare(b.id)
}

/**
 * Everything hanging under an epic, however deep. A sub-epic's children are
 * part of the same process, so the flow follows the parent chain all the way
 * down rather than stopping at the first generation.
 */
export function flowMembers(rows: readonly WorkRow[], epic: WorkRow): WorkRow[] {
  const inStore = rows.filter(row => row.projectPath === epic.projectPath)
  const childrenOf = new Map<string, WorkRow[]>()
  inStore.forEach(row => {
    if (!row.parent) return
    const siblings = childrenOf.get(row.parent) ?? []
    siblings.push(row)
    childrenOf.set(row.parent, siblings)
  })
  const found: WorkRow[] = []
  const seen = new Set<string>([epic.id])
  const walk = (parentId: string) => {
    ;(childrenOf.get(parentId) ?? []).slice().sort(byPriorityThenId).forEach(child => {
      if (seen.has(child.id)) return
      seen.add(child.id)
      found.push(child)
      walk(child.id)
    })
  }
  walk(epic.id)
  return found
}

/** The epics a store offers the flow, in the order the picker lists them. */
export function flowEpics(rows: readonly WorkRow[]): WorkRow[] {
  return rows.filter(row => row.type === 'epic').slice().sort(byPriorityThenId)
}

/**
 * Every Bead joined to a target by a parent or blocking relationship. This is
 * the graph an explicit Open in Flow request uses when the target does not sit
 * neatly inside the epic currently selected in the picker.
 */
export function flowComponent(rows: readonly WorkRow[], target: WorkRow): WorkRow[] {
  const inStore = rows.filter(row => row.projectPath === target.projectPath)
  const byID = new Map(inStore.map(row => [row.id, row]))
  if (!byID.has(target.id)) return []

  const neighbours = new Map(inStore.map(row => [row.id, new Set<string>()]))
  const join = (left: string, right: string) => {
    if (!byID.has(left) || !byID.has(right)) return
    neighbours.get(left)?.add(right)
    neighbours.get(right)?.add(left)
  }
  inStore.forEach(row => {
    if (row.parent) join(row.id, row.parent)
    row.blockedBy?.forEach(blocker => join(row.id, blocker))
  })

  const found: WorkRow[] = []
  const queued = [target.id]
  const seen = new Set(queued)
  while (queued.length > 0) {
    const id = queued.shift() as string
    const row = byID.get(id)
    if (row) found.push(row)
    ;[...(neighbours.get(id) ?? [])].sort().forEach(next => {
      if (seen.has(next)) return
      seen.add(next)
      queued.push(next)
    })
  }
  return found.sort(byPriorityThenId)
}

export function hasFlowLinks(rows: readonly WorkRow[], target: WorkRow): boolean {
  return flowComponent(rows, target).length > 1
}

/** Stable identity for an explicitly constructed connected component. */
export function flowComponentKey(rows: readonly WorkRow[]): string {
  return rows.map(row => row.id).sort().join('\u0000')
}

function edgeName(from: string, to: string): string {
  return `${from}\u0000${to}`
}

/**
 * Lay an epic's work out in waves.
 *
 * A Bead's wave is one past the deepest Bead blocking it, counting only
 * blockers that are part of this epic: a Bead waiting on work elsewhere is
 * still ready as far as this drawing is concerned, and the row itself says
 * what it waits for. Within a column, Beads of the same parent stay together
 * so the band behind them is one rectangle, and then priority and id order
 * them, which is the order every other Beads view uses.
 */
function layoutMembers(members: readonly WorkRow[], bandRootId?: string): FlowGraph {
  if (members.length === 0) return EMPTY_FLOW

  const order = members.slice().sort(byPriorityThenId)
  const byKey = new Map(order.map(row => [beadRowKey(row), row]))
  const blockers = new Map<string, string[]>()
  order.forEach(row => {
    const kept = (row.blockedBy ?? [])
      .map(id => beadRowKey({ projectPath: row.projectPath, id }))
      .filter(key => byKey.has(key))
    blockers.set(beadRowKey(row), kept)
  })

  const wave = new Map<string, number>()
  const visiting = new Set<string>()
  const broken = new Set<string>()
  const waveOf = (key: string): number => {
    const known = wave.get(key)
    if (known !== undefined) return known
    visiting.add(key)
    let value = 0
    ;(blockers.get(key) ?? []).forEach(blocker => {
      // A blocker still on the stack is a loop this store should not hold.
      // The edge is broken here and remembered, so the drawing can say so.
      if (visiting.has(blocker)) {
        broken.add(edgeName(blocker, key))
        return
      }
      value = Math.max(value, waveOf(blocker) + 1)
    })
    visiting.delete(key)
    wave.set(key, value)
    return value
  }
  order.forEach(row => waveOf(beadRowKey(row)))

  const columns = new Map<number, WorkRow[]>()
  order.forEach(row => {
    const column = wave.get(beadRowKey(row)) ?? 0
    const stack = columns.get(column) ?? []
    stack.push(row)
    columns.set(column, stack)
  })

  const nodes: FlowNode[] = []
  ;[...columns.keys()].sort((a, b) => a - b).forEach(column => {
    const stack = (columns.get(column) as WorkRow[]).slice().sort((a, b) => {
      const group = (a.parent ?? '').localeCompare(b.parent ?? '')
      return group !== 0 ? group : byPriorityThenId(a, b)
    })
    stack.forEach((row, lane) => {
      nodes.push({
        key: beadRowKey(row),
        row,
        wave: column,
        lane,
        x: PADDING + column * COLUMN_STRIDE,
        y: PADDING + lane * LANE_STRIDE,
      })
    })
  })

  const placed = new Map(nodes.map(node => [node.key, node]))
  const edges: FlowEdge[] = []
  nodes.forEach(node => {
    ;(blockers.get(node.key) ?? []).forEach(blockerKey => {
      const from = placed.get(blockerKey)
      if (!from) return
      edges.push({
        key: edgeName(blockerKey, node.key),
        from: blockerKey,
        to: node.key,
        x1: from.x + NODE_WIDTH,
        y1: from.y + NODE_HEIGHT / 2,
        x2: node.x,
        y2: node.y + NODE_HEIGHT / 2,
        back: broken.has(edgeName(blockerKey, node.key)),
      })
    })
  })

  // A band marks a parent inside the epic. The epic itself is the roof of
  // everything drawn, so it earns no band; a parent with one child would be a
  // box around a box, so it earns none either.
  const bands: FlowBand[] = []
  const held = new Map<string, FlowNode[]>()
  nodes.forEach(node => {
    const parent = node.row.parent
    if (!parent || parent === bandRootId) return
    const group = held.get(parent) ?? []
    group.push(node)
    held.set(parent, group)
  })
  ;[...held.keys()].sort().forEach(parent => {
    const group = held.get(parent) as FlowNode[]
    if (group.length < 2) return
    const left = Math.min(...group.map(node => node.x))
    const top = Math.min(...group.map(node => node.y))
    const right = Math.max(...group.map(node => node.x + NODE_WIDTH))
    const bottom = Math.max(...group.map(node => node.y + NODE_HEIGHT))
    bands.push({
      key: parent,
      x: left - BAND_INSET,
      y: top - BAND_INSET,
      width: right - left + BAND_INSET * 2,
      height: bottom - top + BAND_INSET * 2,
    })
  })

  const waves = Math.max(...nodes.map(node => node.wave)) + 1
  const lanes = Math.max(...nodes.map(node => node.lane)) + 1
  return {
    nodes,
    edges,
    bands,
    waves,
    width: PADDING * 2 + waves * NODE_WIDTH + (waves - 1) * COLUMN_GAP,
    height: PADDING * 2 + lanes * NODE_HEIGHT + (lanes - 1) * LANE_GAP,
  }
}

export function layoutFlow(rows: readonly WorkRow[], epic: WorkRow): FlowGraph {
  return layoutMembers(flowMembers(rows, epic), epic.id)
}

/** A one-off flow built around an explicitly requested linked Bead. */
export function layoutFlowComponent(rows: readonly WorkRow[], target: WorkRow): FlowGraph {
  return layoutMembers(flowComponent(rows, target))
}

/** The middle of a node, which is what a click brings to the middle of the box. */
export function flowCentre(node: FlowNode): { x: number; y: number } {
  return { x: node.x + NODE_WIDTH / 2, y: node.y + NODE_HEIGHT / 2 }
}

export type FlowStep = 'up' | 'down' | 'left' | 'right'

/**
 * Travelling the flow from the keyboard: up and down the column the Bead is
 * in, left and right to the neighbouring wave, keeping the lane where the
 * neighbouring wave has one and taking its last Bead where it does not.
 */
export function flowNeighbour(graph: FlowGraph, key: string, step: FlowStep): FlowNode | null {
  const here = graph.nodes.find(node => node.key === key)
  if (!here) return null
  if (step === 'up' || step === 'down') {
    const lane = here.lane + (step === 'down' ? 1 : -1)
    return graph.nodes.find(node => node.wave === here.wave && node.lane === lane) ?? null
  }
  const wave = here.wave + (step === 'right' ? 1 : -1)
  const column = graph.nodes.filter(node => node.wave === wave)
  if (column.length === 0) return null
  return column.find(node => node.lane === here.lane) ?? column[column.length - 1]
}
