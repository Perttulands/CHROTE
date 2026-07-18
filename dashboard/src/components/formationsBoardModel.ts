import type {
  BoardConnection,
  BoardDocument,
  BoardUndoAction,
  FormationBrief,
  FormationPort,
  LayoutNode,
} from './formationsTypes'

export function upsertNode(nodes: LayoutNode[], next: LayoutNode): LayoutNode[] {
  const index = nodes.findIndex(node => node.id === next.id)
  if (index < 0) return [...nodes, next]
  return nodes.map(node => node.id === next.id ? next : node)
}

export function undoBoardPatch(action: BoardUndoAction): Record<string, unknown> {
  switch (action.kind) {
    case 'deleteFormation':
      return { deleteFormation: { id: action.formationId } }
    case 'deleteGate':
      return { deleteGate: { id: action.gateId } }
    case 'deleteMission':
      return { deleteMission: { id: action.missionId } }
    case 'assignSlot':
      return {
        assignSlot: {
          formationId: action.formationId,
          slotId: action.slotId,
          agentId: action.agentId,
          harness: action.harness,
        },
      }
    case 'makeController':
      return {
        makeController: {
          formationId: action.formationId,
          slotId: action.slotId,
        },
      }
    case 'setBrief':
      if (!action.brief) {
        return { clearBrief: { formationId: action.formationId } }
      }
      return {
        setBrief: {
          formationId: action.formationId,
          goal: action.brief.goal || '',
          beadId: action.brief.beadId || '',
          files: action.brief.files || [],
          links: action.brief.links || [],
        },
      }
    case 'removePort':
      return {
        removePort: {
          formationId: action.formationId,
          portId: action.portId,
        },
      }
    case 'unwireConnection':
      return {
        unwireConnection: {
          from: action.from,
          to: action.to,
        },
      }
  }
}

export function findAddedByID<T extends { id: string }>(before: T[], after: T[]): T | undefined {
  const known = new Set(before.map(item => item.id))
  return after.find(item => !known.has(item.id))
}

export function findAddedPort(before: BoardDocument | null, after: BoardDocument | null, formationId: string, direction: 'input' | 'output'): FormationPort | undefined {
  const beforeFormation = before?.formations.find(formation => formation.id === formationId)
  const afterFormation = after?.formations.find(formation => formation.id === formationId)
  if (!afterFormation) return undefined
  const key = direction === 'input' ? 'inputs' : 'outputs'
  return findAddedByID(beforeFormation?.[key] || [], afterFormation[key] || [])
}

export function cloneBrief(brief: FormationBrief): FormationBrief {
  return {
    goal: brief.goal || '',
    beadId: brief.beadId || '',
    files: [...(brief.files || [])],
    links: [...(brief.links || [])],
  }
}

export function judgeChainWithReturn(board: BoardDocument, gateId: string, returnEndpoint: string): string[] {
  const returnNodeId = endpointNodeId(returnEndpoint)
  if (!board.formations.some(formation => formation.id === returnNodeId)) return []
  const entry = board.connections.find(connection => connection.from === `${gateId}:judge`)
  if (!entry) return [returnNodeId]

  const chain = walkJudgeChain(board.connections, endpointNodeId(entry.to), gateId)
  const existingIndex = chain.indexOf(returnNodeId)
  if (existingIndex >= 0) return chain.slice(0, existingIndex + 1)
  return [...chain, returnNodeId]
}

function walkJudgeChain(connections: BoardConnection[], startNodeId: string, gateId: string): string[] {
  const chain: string[] = []
  const seen = new Set<string>()
  let current = startNodeId
  while (current && !seen.has(current)) {
    seen.add(current)
    chain.push(current)
    const next = connections.find(connection => {
      const fromNodeId = endpointNodeId(connection.from)
      const toNodeId = endpointNodeId(connection.to)
      return fromNodeId === current && connection.to !== `${gateId}:judge` && toNodeId !== gateId
    })
    if (!next) break
    current = endpointNodeId(next.to)
  }
  return chain
}

function endpointNodeId(endpoint: string): string {
  return endpoint.split(':', 1)[0]
}
