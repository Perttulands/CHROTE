import { upsertNode } from './formationsBoardModel'
import { endpointNodeId } from './formationsCanvas'
import type {
  BoardConnection,
  BoardDocument,
  BoardSummary,
  FormationNode,
  FormationSlot,
  GateNode,
  LayoutDocument,
  MissionNode,
} from './formationsTypes'

const typeLabels: Record<FormationNode['type'], string> = {
  solo: 'Solo',
  peer: 'Peer',
  flow: 'Flow',
  orchestrated: 'Orchestrated',
}

const starterBoardID = 'brd_starter_session_search'
const starterBoardSlug = 'starter-session-search'
const starterBoardETag = 'starter-board-etag-1'
const starterLayoutETag = 'starter-layout-etag-1'
let starterLocalSequence = 0

export function createStarterBoard(): BoardDocument {
  return {
    id: starterBoardID,
    slug: starterBoardSlug,
    title: 'Improve session search',
    rev: 1,
    etag: starterBoardETag,
    missions: [{
      id: 'mis_starter_session_search',
      title: 'Improve session search',
      goal: 'Make session search fuzzy and keyboard-first',
      beadId: 'home-pfyv',
    }],
    formations: [
      {
        id: 'fmn_starter_frame',
        type: 'solo',
        title: 'Frame the goal',
        brief: {
          goal: 'Turn the manual report into clear acceptance criteria',
          beadId: 'home-pfyv',
        },
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [{ id: 'slot_starter_frame_agent', label: 'Agent', controller: false, agentId: 'conductor' }],
      },
      {
        id: 'fmn_starter_research',
        type: 'peer',
        title: 'Research huddle',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [
          { id: 'slot_starter_research_a', label: 'Peer', controller: false, agentId: 'codex' },
          { id: 'slot_starter_research_b', label: 'Peer', controller: false, agentId: 'claude-code' },
        ],
        verification: {
          id: 'ver_starter_research',
          kinds: ['code'],
          criterion: 'both reads converge on a safe recommendation',
          onFail: 'block',
        },
      },
      {
        id: 'fmn_starter_ship',
        type: 'flow',
        title: 'Ship a change',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [
          { id: 'slot_starter_ship_plan', label: 'Plan', controller: false, agentId: 'scout' },
          { id: 'slot_starter_ship_execute', label: 'Execute', controller: false, agentId: 'refiner' },
          { id: 'slot_starter_ship_push', label: 'Push', controller: false, agentId: 'mason' },
        ],
      },
      {
        id: 'fmn_starter_triage',
        type: 'orchestrated',
        title: 'Triage desk',
        inputs: [{ id: 'in', label: 'Input' }],
        outputs: [{ id: 'out', label: 'Output' }],
        slots: [
          { id: 'slot_starter_triage_orchestrator', label: 'Orchestrator', controller: true, agentId: 'conductor' },
          { id: 'slot_starter_triage_a', label: 'Agent', controller: false, agentId: 'witness' },
          { id: 'slot_starter_triage_b', label: 'Agent', controller: false },
          { id: 'slot_starter_triage_c', label: 'Agent', controller: false },
        ],
      },
    ],
    gates: [{
      id: 'gate_starter_review',
      title: 'Review gate',
      kinds: ['human', 'code'],
      criterion: 'research is sound and the plan is safe to build',
    }],
    connections: [
      { id: 'edge_starter_mission_frame', from: 'mis_starter_session_search:out', to: 'fmn_starter_frame:in' },
      { id: 'edge_starter_frame_research', from: 'fmn_starter_frame:out', to: 'fmn_starter_research:in' },
      { id: 'edge_starter_research_gate', from: 'fmn_starter_research:out', to: 'gate_starter_review:in' },
      { id: 'edge_starter_gate_ship', from: 'gate_starter_review:pass', to: 'fmn_starter_ship:in' },
    ],
  }
}

export function createStarterLayout(): LayoutDocument {
  return {
    boardId: starterBoardID,
    boardRev: 1,
    etag: starterLayoutETag,
    nodes: [
      { id: 'mis_starter_session_search', x: 70, y: 90 },
      { id: 'fmn_starter_frame', x: 350, y: 70 },
      { id: 'fmn_starter_research', x: 650, y: 70 },
      { id: 'gate_starter_review', x: 950, y: 95 },
      { id: 'fmn_starter_ship', x: 1250, y: 70 },
      { id: 'fmn_starter_triage', x: 350, y: 380 },
    ],
    edges: [],
  }
}

export function starterBoardSummary(board: BoardDocument): BoardSummary {
  return {
    id: board.id,
    slug: board.slug,
    title: board.title,
    rev: board.rev,
    etag: board.etag,
  }
}

export function isStarterBoard(board: BoardDocument | null | undefined): boolean {
  return board?.id === starterBoardID && board.slug === starterBoardSlug
}

export function withStarterLayoutRev(layout: LayoutDocument, boardRev: number): LayoutDocument {
  return {
    ...layout,
    boardRev,
    etag: `starter-layout-etag-${boardRev}-${starterLocalSequence}`,
  }
}

export function applyStarterBoardPatch(
  board: BoardDocument,
  layout: LayoutDocument,
  patch: Record<string, unknown>
): { board: BoardDocument; layout?: LayoutDocument } {
  const createFormation = patch.createFormation as Partial<{
    type: FormationNode['type']
    title: string
    x: number
    y: number
  }> | undefined
  if (createFormation) {
    const type = isFormationType(createFormation.type) ? createFormation.type : 'solo'
    const formation = localFormation(type, String(createFormation.title || typeLabels[type]))
    const nextBoard = withStarterBoardRev({
      ...board,
      formations: [...board.formations, formation],
    })
    return {
      board: nextBoard,
      layout: withStarterLayoutRev({
        ...layout,
        nodes: upsertNode(layout.nodes || [], {
          id: formation.id,
          x: Number(createFormation.x || 120),
          y: Number(createFormation.y || 120),
        }),
      }, nextBoard.rev),
    }
  }

  const createGate = patch.createGate as Partial<{ title: string; kinds: string[]; criterion: string; x: number; y: number }> | undefined
  if (createGate) {
    const gate: GateNode = {
      id: starterID('gate'),
      title: createGate.title || 'Review gate',
      kinds: createGate.kinds?.length ? createGate.kinds : ['code'],
      criterion: createGate.criterion || '',
    }
    const nextBoard = withStarterBoardRev({
      ...board,
      gates: [...(board.gates || []), gate],
    })
    return {
      board: nextBoard,
      layout: withStarterLayoutRev({
        ...layout,
        nodes: upsertNode(layout.nodes || [], {
          id: gate.id,
          x: Number(createGate.x || 420),
          y: Number(createGate.y || 220),
        }),
      }, nextBoard.rev),
    }
  }

  const createMission = patch.createMission as Partial<{ title: string; goal: string; beadId: string }> | undefined
  if (createMission) {
    const mission: MissionNode = {
      id: starterID('mis'),
      title: createMission.title || 'Mission',
      goal: createMission.goal || '',
      beadId: createMission.beadId || 'home-pfyv',
    }
    const nextBoard = withStarterBoardRev({
      ...board,
      missions: [...(board.missions || []), mission],
    })
    return {
      board: nextBoard,
      layout: withStarterLayoutRev({
        ...layout,
        nodes: upsertNode(layout.nodes || [], { id: mission.id, x: 80, y: 280 }),
      }, nextBoard.rev),
    }
  }

  const deleteFormation = patch.deleteFormation as Partial<{ id: string }> | undefined
  if (deleteFormation?.id) {
    return deleteStarterNodes(board, layout, new Set([deleteFormation.id]))
  }

  const deleteGate = patch.deleteGate as Partial<{ id: string }> | undefined
  if (deleteGate?.id) {
    return deleteStarterNodes(board, layout, new Set([deleteGate.id]))
  }

  const deleteMission = patch.deleteMission as Partial<{ id: string }> | undefined
  if (deleteMission?.id) {
    return deleteStarterNodes(board, layout, new Set([deleteMission.id]))
  }

  const assignSlot = patch.assignSlot as Partial<{ formationId: string; slotId: string; agentId: string; harness: string }> | undefined
  if (assignSlot?.formationId && assignSlot.slotId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === assignSlot.formationId
          ? {
            ...formation,
            slots: formation.slots.map(slot => slot.id === assignSlot.slotId
              ? { ...slot, agentId: assignSlot.agentId || undefined, harness: assignSlot.harness || undefined }
              : slot),
          }
          : formation),
      }),
    }
  }

  const makeController = patch.makeController as Partial<{ formationId: string; slotId: string }> | undefined
  if (makeController?.formationId && makeController.slotId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === makeController.formationId
          ? {
            ...formation,
            slots: formation.slots.map(slot => ({ ...slot, controller: slot.id === makeController.slotId })),
          }
          : formation),
      }),
    }
  }

  const setBrief = patch.setBrief as Partial<{ formationId: string; goal: string; beadId: string; files: string[]; links: string[] }> | undefined
  if (setBrief?.formationId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === setBrief.formationId
          ? {
            ...formation,
            brief: {
              goal: setBrief.goal || '',
              beadId: setBrief.beadId || '',
              files: setBrief.files || [],
              links: setBrief.links || [],
            },
          }
          : formation),
      }),
    }
  }

  const clearBrief = patch.clearBrief as Partial<{ formationId: string }> | undefined
  if (clearBrief?.formationId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === clearBrief.formationId
          ? { ...formation, brief: undefined }
          : formation),
      }),
    }
  }

  const setVerification = patch.setVerification as Partial<{
    formationId: string
    kinds: string[]
    criterion: string
    onFail: 'block' | 'pushback'
  }> | undefined
  if (setVerification?.formationId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === setVerification.formationId
          ? {
            ...formation,
            verification: {
              id: formation.verification?.id || starterID('ver'),
              kinds: setVerification.kinds?.length ? setVerification.kinds : ['code'],
              criterion: setVerification.criterion || '',
              onFail: setVerification.onFail || 'block',
            },
          }
          : formation),
      }),
    }
  }

  const removeVerification = patch.removeVerification as Partial<{ formationId: string }> | undefined
  if (removeVerification?.formationId) {
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === removeVerification.formationId
          ? { ...formation, verification: undefined }
          : formation),
      }),
    }
  }

  const addPort = patch.addPort as Partial<{ formationId: string; direction: 'input' | 'output'; label: string }> | undefined
  if (addPort?.formationId && (addPort.direction === 'input' || addPort.direction === 'output')) {
    const port = { id: starterID('port'), label: addPort.label || (addPort.direction === 'input' ? 'Input' : 'Output') }
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === addPort.formationId
          ? addPort.direction === 'input'
            ? { ...formation, inputs: [...formation.inputs, port] }
            : { ...formation, outputs: [...formation.outputs, port] }
          : formation),
      }),
    }
  }

  const removePort = patch.removePort as Partial<{ formationId: string; portId: string }> | undefined
  if (removePort?.formationId && removePort.portId) {
    const endpoint = `${removePort.formationId}:${removePort.portId}`
    return {
      board: withStarterBoardRev({
        ...board,
        formations: board.formations.map(formation => formation.id === removePort.formationId
          ? {
            ...formation,
            inputs: formation.inputs.filter(input => input.id !== removePort.portId),
            outputs: formation.outputs.filter(output => output.id !== removePort.portId),
          }
          : formation),
        connections: board.connections.filter(connection => connection.from !== endpoint && connection.to !== endpoint),
      }),
    }
  }

  const wireConnection = patch.wireConnection as Partial<{ from: string; to: string }> | undefined
  if (wireConnection?.from && wireConnection.to) {
    if (
      endpointNodeId(wireConnection.from) === endpointNodeId(wireConnection.to) ||
      board.connections.some(connection => connection.from === wireConnection.from && connection.to === wireConnection.to)
    ) {
      return { board }
    }
    return {
      board: withStarterBoardRev({
        ...board,
        connections: [
          ...board.connections.filter(connection => connection.to !== wireConnection.to),
          { id: starterID('edge'), from: wireConnection.from, to: wireConnection.to },
        ],
      }),
    }
  }

  const rewireConnection = patch.rewireConnection as Partial<{ from: string; previousTo: string; to: string }> | undefined
  if (rewireConnection?.from && rewireConnection.previousTo && rewireConnection.to) {
    if (
      endpointNodeId(rewireConnection.from) === endpointNodeId(rewireConnection.to) ||
      !board.connections.some(connection => connection.from === rewireConnection.from && connection.to === rewireConnection.previousTo) ||
      board.connections.some(connection => connection.to === rewireConnection.to && !(connection.from === rewireConnection.from && connection.to === rewireConnection.previousTo))
    ) {
      return { board }
    }
    return {
      board: withStarterBoardRev({
        ...board,
        connections: [
          ...board.connections.filter(connection => connection.from !== rewireConnection.from || connection.to !== rewireConnection.previousTo),
          { id: starterID('edge'), from: rewireConnection.from, to: rewireConnection.to },
        ],
      }),
    }
  }

  const unwireConnection = patch.unwireConnection as Partial<{ from: string; to: string }> | undefined
  if (unwireConnection?.from && unwireConnection.to) {
    return {
      board: withStarterBoardRev({
        ...board,
        connections: board.connections.filter(connection => connection.from !== unwireConnection.from || connection.to !== unwireConnection.to),
      }),
    }
  }

  const setGateJudge = patch.setGateJudge as Partial<{ gateId: string; chain: string[] }> | undefined
  if (setGateJudge?.gateId && setGateJudge.chain?.length) {
    const gateID = setGateJudge.gateId
    const judgeConnections = judgeConnectionsForChain(gateID, setGateJudge.chain)
    return {
      board: withStarterBoardRev({
        ...board,
        gates: (board.gates || []).map(gate => gate.id === gateID
          ? { ...gate, kinds: Array.from(new Set([...gate.kinds, 'formation'])) }
          : gate),
        connections: [
          ...board.connections.filter(connection => connection.from !== `${gateID}:judge` && connection.to !== `${gateID}:judge`),
          ...judgeConnections,
        ],
      }),
    }
  }

  const detachGateJudge = patch.detachGateJudge as Partial<{ gateId: string }> | undefined
  if (detachGateJudge?.gateId) {
    const gateID = detachGateJudge.gateId
    return {
      board: withStarterBoardRev({
        ...board,
        gates: (board.gates || []).map(gate => gate.id === gateID
          ? { ...gate, kinds: gate.kinds.filter(kind => kind !== 'formation') }
          : gate),
        connections: board.connections.filter(connection => connection.from !== `${gateID}:judge` && connection.to !== `${gateID}:judge`),
      }),
    }
  }

  const title = typeof patch.title === 'string' ? patch.title.trim() : ''
  return {
    board: title ? withStarterBoardRev({ ...board, title }) : board,
  }
}

function localFormation(type: FormationNode['type'], title: string): FormationNode {
  const id = starterID('fmn')
  return {
    id,
    type,
    title: title || typeLabels[type],
    inputs: [{ id: starterID('port'), label: 'Input' }],
    outputs: [{ id: starterID('port'), label: 'Output' }],
    slots: localFormationSlots(type),
  }
}

function localFormationSlots(type: FormationNode['type']): FormationSlot[] {
  switch (type) {
    case 'solo':
      return [{ id: starterID('slot'), label: 'Agent', controller: false }]
    case 'peer':
      return [
        { id: starterID('slot'), label: 'Peer', controller: false },
        { id: starterID('slot'), label: 'Peer', controller: false },
      ]
    case 'flow':
      return [
        { id: starterID('slot'), label: 'Plan', controller: false },
        { id: starterID('slot'), label: 'Execute', controller: false },
        { id: starterID('slot'), label: 'Push', controller: false },
      ]
    case 'orchestrated':
      return [
        { id: starterID('slot'), label: 'Orchestrator', controller: true },
        { id: starterID('slot'), label: 'Agent', controller: false },
        { id: starterID('slot'), label: 'Agent', controller: false },
      ]
  }
}

function withStarterBoardRev(board: BoardDocument): BoardDocument {
  const rev = board.rev + 1
  return {
    ...board,
    rev,
    etag: `starter-board-etag-${rev}`,
  }
}

function starterID(prefix: string): string {
  starterLocalSequence += 1
  return `${prefix}_starter_local_${starterLocalSequence}`
}

function deleteStarterNodes(
  board: BoardDocument,
  layout: LayoutDocument,
  nodeIDs: Set<string>
): { board: BoardDocument; layout: LayoutDocument } {
  const nextBoard = withStarterBoardRev({
    ...board,
    missions: (board.missions || []).filter(mission => !nodeIDs.has(mission.id)),
    formations: board.formations.filter(formation => !nodeIDs.has(formation.id)),
    gates: (board.gates || []).filter(gate => !nodeIDs.has(gate.id)),
    connections: board.connections.filter(connection => !nodeIDs.has(endpointNodeId(connection.from)) && !nodeIDs.has(endpointNodeId(connection.to))),
  })
  return {
    board: nextBoard,
    layout: withStarterLayoutRev({
      ...layout,
      nodes: (layout.nodes || []).filter(node => !nodeIDs.has(node.id)),
    }, nextBoard.rev),
  }
}

function judgeConnectionsForChain(gateID: string, chain: string[]): BoardConnection[] {
  const connections: BoardConnection[] = []
  if (chain[0]) {
    connections.push({ id: starterID('edge'), from: `${gateID}:judge`, to: `${chain[0]}:in` })
  }
  for (let i = 0; i < chain.length - 1; i += 1) {
    connections.push({ id: starterID('edge'), from: `${chain[i]}:out`, to: `${chain[i + 1]}:in` })
  }
  if (chain.length > 0) {
    connections.push({ id: starterID('edge'), from: `${chain[chain.length - 1]}:out`, to: `${gateID}:judge` })
  }
  return connections
}

function isFormationType(value: unknown): value is FormationNode['type'] {
  return value === 'solo' || value === 'peer' || value === 'flow' || value === 'orchestrated'
}
