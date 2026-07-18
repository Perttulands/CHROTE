import { describe, expect, it } from 'vitest'
import {
  cloneBrief,
  findAddedByID,
  findAddedPort,
  judgeChainWithReturn,
  undoBoardPatch,
  upsertNode,
} from './formationsBoardModel'
import type { BoardDocument, FormationBrief, LayoutNode } from './formationsTypes'

describe('formations board model helpers', () => {
  it('upserts layout nodes without reordering unrelated nodes', () => {
    const nodes: LayoutNode[] = [
      { id: 'formation-frame', x: 120, y: 120 },
      { id: 'formation-research', x: 420, y: 120 },
    ]

    expect(upsertNode(nodes, { id: 'formation-ship', x: 720, y: 120 })).toEqual([
      ...nodes,
      { id: 'formation-ship', x: 720, y: 120 },
    ])
    expect(upsertNode(nodes, { id: 'formation-frame', x: 160, y: 180 })).toEqual([
      { id: 'formation-frame', x: 160, y: 180 },
      nodes[1],
    ])
  })

  it('turns undo actions into single-writer board patches', () => {
    expect(undoBoardPatch({ kind: 'assignSlot', formationId: 'formation-frame', slotId: 'slot-lead', agentId: 'codex', harness: 'openai-codex' })).toEqual({
      assignSlot: {
        formationId: 'formation-frame',
        slotId: 'slot-lead',
        agentId: 'codex',
        harness: 'openai-codex',
      },
    })
    expect(undoBoardPatch({ kind: 'setBrief', formationId: 'formation-frame' })).toEqual({
      clearBrief: { formationId: 'formation-frame' },
    })
    expect(undoBoardPatch({ kind: 'unwireConnection', from: 'formation-frame:out', to: 'formation-ship:in' })).toEqual({
      unwireConnection: { from: 'formation-frame:out', to: 'formation-ship:in' },
    })
  })

  it('finds new ids and newly added ports across board revisions', () => {
    const before: BoardDocument = {
      id: 'board',
      slug: 'board',
      title: 'Board',
      rev: 1,
      etag: 'before',
      formations: [{
        id: 'formation-frame',
        type: 'solo',
        title: 'Frame',
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [],
        slots: [],
      }],
      connections: [],
    }
    const after: BoardDocument = {
      ...before,
      rev: 2,
      etag: 'after',
      formations: [{
        ...before.formations[0],
        inputs: [...before.formations[0].inputs, { id: 'extra', label: 'extra' }],
      }],
    }

    expect(findAddedByID([{ id: 'a' }], [{ id: 'a' }, { id: 'b' }])).toEqual({ id: 'b' })
    expect(findAddedPort(before, after, 'formation-frame', 'input')).toEqual({ id: 'extra', label: 'extra' })
  })

  it('clones mutable brief arrays for undo snapshots', () => {
    const brief: FormationBrief = { goal: 'Ship', beadId: 'home-vdki.27', files: ['a.md'], links: ['https://example.test'] }
    const briefClone = cloneBrief(brief)

    brief.files?.push('later.md')

    expect(briefClone.files).toEqual(['a.md'])
  })

  it('preserves the existing judge entry chain when moving the return edge', () => {
    const board: BoardDocument = {
      id: 'board',
      slug: 'board',
      title: 'Board',
      rev: 1,
      etag: 'etag',
      formations: ['frame', 'research', 'ship', 'critique'].map(id => ({
        id,
        type: 'solo',
        title: id,
        inputs: [{ id: 'in', label: 'input' }],
        outputs: [{ id: 'out', label: 'output' }],
        slots: [],
      })),
      gates: [{ id: 'gate-review', title: 'Review', kinds: ['formation'], criterion: '' }],
      connections: [
        { id: 'edge-send', from: 'gate-review:judge', to: 'frame:in' },
        { id: 'edge-frame-research', from: 'frame:out', to: 'research:in' },
        { id: 'edge-research-ship', from: 'research:out', to: 'ship:in' },
        { id: 'edge-return', from: 'ship:out', to: 'gate-review:judge' },
      ],
    }

    expect(judgeChainWithReturn(board, 'gate-review', 'critique:out')).toEqual(['frame', 'research', 'ship', 'critique'])
    expect(judgeChainWithReturn(board, 'gate-review', 'research:out')).toEqual(['frame', 'research'])
  })
})
