/**
 * Where everything on the process map sits.
 *
 * The host is the hub at the origin. Each group (a tmux session, a service)
 * is a circle around it, sized by what it holds, pushed apart until none
 * overlap and pulled in so the map stays compact. Inside a group its
 * processes form a radial tree: the heaviest root in the middle, children on
 * rings around it. Everything is in one world space that the view fits to
 * its box. Given the previous layout, groups start where they were, so a
 * poll moves things a little instead of reshuffling the map.
 */

import type { ProcessGroup, ProcessNode } from '../../services/systemClient'

export const MAX_GROUPS = 12
const HUB_RADIUS = 64
const GROUP_GAP = 30
const RELAX_STEPS = 240

export const GROUP_HUES = ['#34d399', '#e879f9', '#22d3ee', '#fbbf24', '#fb7185', '#a78bfa', '#60a5fa', '#2dd4bf', '#fb923c', '#a3e635', '#f472b6', '#38bdf8']

export interface PlacedNode {
  pid: number
  parent: number | null
  x: number
  y: number
  r: number
  node: ProcessNode
  centre: boolean
  labelled: boolean
}

export interface PlacedGroup {
  key: string
  group: ProcessGroup
  hue: string
  x: number
  y: number
  r: number
  nodes: PlacedNode[]
}

export interface ProcessLayout {
  groups: PlacedGroup[]
  hidden: ProcessGroup[]
  bounds: { minX: number; minY: number; maxX: number; maxY: number }
}

const MiB = 1024 * 1024

export function groupWeight(group: ProcessGroup) {
  return group.cpuPercent + group.rssBytes / (64 * MiB)
}

function nodeWeight(node: ProcessNode) {
  return node.cpuPercent + node.rssBytes / (64 * MiB)
}

export function hueFor(key: string) {
  let hash = 0
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0
  return GROUP_HUES[hash % GROUP_HUES.length]
}

export function nodeRadius(node: ProcessNode) {
  const mb = node.rssBytes / MiB
  return Math.min(15, 3 + 1.6 * Math.sqrt(mb / 16))
}

function groupRadius(group: ProcessGroup) {
  const gb = group.rssBytes / (1024 * MiB)
  return Math.max(70, Math.min(190, 58 + 11 * Math.sqrt(group.processes.length) + 34 * Math.sqrt(gb)))
}

/** The processes of one group, laid out around (0, 0) inside radius r. */
export function layoutGroup(group: ProcessGroup, r: number): PlacedNode[] {
  const nodes = group.processes
  if (nodes.length === 0) return []
  const shown = new Set(nodes.map(node => node.pid))
  const children = new Map<number, ProcessNode[]>()
  const roots: ProcessNode[] = []
  for (const node of nodes) {
    if (shown.has(node.ppid) && node.ppid !== node.pid) {
      const list = children.get(node.ppid) || []
      list.push(node)
      children.set(node.ppid, list)
    } else {
      roots.push(node)
    }
  }
  roots.sort((a, b) => nodeWeight(b) - nodeWeight(a) || a.pid - b.pid)
  const centre = roots[0]
  // The other roots hang off the centre, so one group reads as one tree.
  children.set(centre.pid, [...(children.get(centre.pid) || []), ...roots.slice(1)])
  for (const list of children.values()) list.sort((a, b) => a.pid - b.pid)

  const leaves = new Map<number, number>()
  const countLeaves = (pid: number, depth: number): number => {
    const kids = depth > 12 ? [] : children.get(pid) || []
    const count = kids.length ? kids.reduce((sum, kid) => sum + countLeaves(kid.pid, depth + 1), 0) : 1
    leaves.set(pid, count)
    return count
  }
  countLeaves(centre.pid, 0)

  // Labels for the few that matter: the centre, then the heaviest.
  const ranked = [...nodes].sort((a, b) => nodeWeight(b) - nodeWeight(a))
  const labelled = new Set(ranked.slice(0, 6).filter(node => node.rssBytes > 24 * MiB || node.cpuPercent >= 1).map(node => node.pid))
  labelled.add(centre.pid)

  const ringFor = (depth: number) => Math.min(r - 16, r * (0.5 + 0.22 * (depth - 1)))
  const placed: PlacedNode[] = []
  const place = (node: ProcessNode, parent: number | null, depth: number, from: number, to: number) => {
    const angle = (from + to) / 2
    const radius = depth === 0 ? 0 : ringFor(depth)
    placed.push({
      pid: node.pid,
      parent,
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
      r: depth === 0 ? Math.max(9, nodeRadius(node)) : nodeRadius(node),
      node,
      centre: depth === 0,
      labelled: labelled.has(node.pid),
    })
    const kids = depth > 12 ? [] : children.get(node.pid) || []
    const total = kids.reduce((sum, kid) => sum + (leaves.get(kid.pid) || 1), 0)
    let at = from
    for (const kid of kids) {
      const share = (to - from) * (leaves.get(kid.pid) || 1) / total
      place(kid, node.pid, depth + 1, at, at + share)
      at += share
    }
  }
  place(centre, null, 0, -Math.PI / 2, Math.PI * 1.5)
  return placed
}

/** The whole map. `previous` keeps groups near where they last were. */
export function layoutProcessMap(groups: ProcessGroup[], previous?: ProcessLayout | null): ProcessLayout {
  const ranked = [...groups].sort((a, b) => groupWeight(b) - groupWeight(a) || a.key.localeCompare(b.key))
  const visible = ranked.slice(0, MAX_GROUPS)
  const hidden = ranked.slice(MAX_GROUPS)
  // Stable order around the hub, so a group keeps its side of the map.
  visible.sort((a, b) => a.key.localeCompare(b.key))

  const before = new Map(previous?.groups.map(group => [group.key, group]) || [])
  const bodies = visible.map((group, index) => {
    const r = groupRadius(group)
    const last = before.get(group.key)
    const angle = (index / Math.max(1, visible.length)) * Math.PI * 2 - Math.PI / 2
    const distance = HUB_RADIUS + GROUP_GAP + r + 60
    return {
      group,
      r,
      x: last ? last.x : Math.cos(angle) * distance,
      y: last ? last.y : Math.sin(angle) * distance,
    }
  })

  for (let step = 0; step < RELAX_STEPS; step++) {
    for (const body of bodies) {
      // A gentle pull towards the hub keeps the map compact.
      body.x *= 0.985
      body.y *= 0.985
      const distance = Math.hypot(body.x, body.y) || 1
      const clear = HUB_RADIUS + GROUP_GAP + body.r
      if (distance < clear) {
        body.x = body.x / distance * clear
        body.y = body.y / distance * clear
      }
    }
    for (let i = 0; i < bodies.length; i++) {
      for (let j = i + 1; j < bodies.length; j++) {
        const a = bodies[i]
        const b = bodies[j]
        let dx = b.x - a.x
        let dy = b.y - a.y
        let distance = Math.hypot(dx, dy)
        if (distance === 0) {
          dx = 1
          dy = 0
          distance = 1
        }
        const overlap = a.r + b.r + GROUP_GAP - distance
        if (overlap > 0) {
          const push = overlap / 2 / distance
          a.x -= dx * push
          a.y -= dy * push
          b.x += dx * push
          b.y += dy * push
        }
      }
    }
  }

  const placed = bodies.map(body => ({
    key: body.group.key,
    group: body.group,
    hue: hueFor(body.group.key),
    x: body.x,
    y: body.y,
    r: body.r,
    nodes: layoutGroup(body.group, body.r),
  }))

  const bounds = { minX: -HUB_RADIUS, minY: -HUB_RADIUS, maxX: HUB_RADIUS, maxY: HUB_RADIUS }
  for (const group of placed) {
    bounds.minX = Math.min(bounds.minX, group.x - group.r)
    bounds.minY = Math.min(bounds.minY, group.y - group.r - 36)
    bounds.maxX = Math.max(bounds.maxX, group.x + group.r)
    bounds.maxY = Math.max(bounds.maxY, group.y + group.r)
  }
  return { groups: placed, hidden, bounds }
}

/** A layout part way from one to the next, for gliding between polls. */
export function blendLayouts(from: ProcessLayout, to: ProcessLayout, t: number): ProcessLayout {
  const ease = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2
  const mix = (a: number, b: number) => a + (b - a) * ease
  const oldGroups = new Map(from.groups.map(group => [group.key, group]))
  return {
    ...to,
    bounds: {
      minX: mix(from.bounds.minX, to.bounds.minX),
      minY: mix(from.bounds.minY, to.bounds.minY),
      maxX: mix(from.bounds.maxX, to.bounds.maxX),
      maxY: mix(from.bounds.maxY, to.bounds.maxY),
    },
    groups: to.groups.map(group => {
      const old = oldGroups.get(group.key)
      if (!old) return group
      const oldNodes = new Map(old.nodes.map(node => [node.pid, node]))
      return {
        ...group,
        x: mix(old.x, group.x),
        y: mix(old.y, group.y),
        r: mix(old.r, group.r),
        nodes: group.nodes.map(node => {
          const was = oldNodes.get(node.pid)
          return was ? { ...node, x: mix(was.x, node.x), y: mix(was.y, node.y), r: mix(was.r, node.r) } : node
        }),
      }
    }),
  }
}
