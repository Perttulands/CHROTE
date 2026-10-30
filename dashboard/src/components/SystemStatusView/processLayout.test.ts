import { describe, expect, it } from 'vitest'
import type { ProcessGroup } from '../../services/systemClient'
import { layoutGroup, layoutProcessMap, MAX_GROUPS } from './processLayout'

const MiB = 1024 * 1024

function group(key: string, processes: Array<[number, number, number?, number?]>): ProcessGroup {
  const nodes = processes.map(([pid, ppid, cpu = 0, mb = 10]) => ({
    pid, ppid, name: `p${pid}`, command: '', cpuPercent: cpu, rssBytes: mb * MiB, threads: 1,
  }))
  return {
    key,
    kind: 'service',
    name: key,
    cpuPercent: nodes.reduce((s, n) => s + n.cpuPercent, 0),
    rssBytes: nodes.reduce((s, n) => s + n.rssBytes, 0),
    count: nodes.length,
    processes: nodes,
  }
}

describe('process map layout', () => {
  it('puts the heaviest root in the middle and its children around it', () => {
    const placed = layoutGroup(group('a', [[10, 1, 0, 500], [11, 10], [12, 10], [13, 11]]), 100)
    const centre = placed.find(node => node.centre)
    expect(centre?.pid).toBe(10)
    expect(centre?.x).toBe(0)
    for (const node of placed.filter(n => !n.centre)) {
      expect(Math.hypot(node.x, node.y)).toBeLessThanOrEqual(100)
      expect(node.parent).not.toBeNull()
    }
  })

  it('keeps groups apart and clear of the hub', () => {
    const groups = Array.from({ length: 8 }, (_, i) => group(`g${i}`, [[i * 10 + 1, 1, i, 200]]))
    const layout = layoutProcessMap(groups)
    for (const a of layout.groups) {
      expect(Math.hypot(a.x, a.y)).toBeGreaterThan(a.r + 60)
      for (const b of layout.groups) {
        if (a === b) continue
        expect(Math.hypot(a.x - b.x, a.y - b.y)).toBeGreaterThan(a.r + b.r)
      }
    }
  })

  it('shows the heaviest groups and folds the rest', () => {
    const groups = Array.from({ length: MAX_GROUPS + 3 }, (_, i) => group(`g${String(i).padStart(2, '0')}`, [[i + 2, 1, i]]))
    const layout = layoutProcessMap(groups)
    expect(layout.groups).toHaveLength(MAX_GROUPS)
    expect(layout.hidden.map(g => g.key)).toEqual(['g02', 'g01', 'g00'])
  })

  it('starts from the previous layout so a poll barely moves a group', () => {
    const first = layoutProcessMap([group('a', [[2, 1, 5]]), group('b', [[3, 1, 1]])])
    const second = layoutProcessMap([group('a', [[2, 1, 6]]), group('b', [[3, 1, 1]])], first)
    const moved = Math.hypot(first.groups[0].x - second.groups[0].x, first.groups[0].y - second.groups[0].y)
    expect(moved).toBeLessThan(20)
  })
})
