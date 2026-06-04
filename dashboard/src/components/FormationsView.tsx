import { MouseEvent, PointerEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { LocateFixed, Minus, Plus } from 'lucide-react'
import { routeFormationWire, type ObstacleRect } from './formationsRouting'

interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: { code: string; message: string }
}

interface BoardSummary {
  id: string
  slug: string
  title: string
  rev: number
  etag: string
}

interface FormationPort {
  id: string
  label: string
}

interface FormationSlot {
  id: string
  label: string
  controller: boolean
  agentId?: string
  harness?: string
}

interface FormationBrief {
  goal?: string
  beadId?: string
  files?: string[]
  links?: string[]
}

interface FormationVerification {
  id: string
  kinds: string[]
  criterion: string
  onFail: 'block' | 'pushback'
}

interface FormationNode {
  id: string
  type: 'solo' | 'peer' | 'flow' | 'orchestrated'
  title: string
  brief?: FormationBrief
  inputs: FormationPort[]
  outputs: FormationPort[]
  slots: FormationSlot[]
  verification?: FormationVerification
}

interface BoardConnection {
  id: string
  from: string
  to: string
}

interface BoardDocument {
  id: string
  slug: string
  title: string
  rev: number
  etag: string
  missions?: MissionNode[]
  formations: FormationNode[]
  gates?: GateNode[]
  connections: BoardConnection[]
}

interface MissionNode {
  id: string
  title: string
  goal: string
  beadId: string
}

interface GateNode {
  id: string
  title: string
  kinds: string[]
  criterion: string
}

interface RunStatusProjection {
  runId: string
  status: string
  final: boolean
  boardSlug: string
  missionId: string
  eventCount: number
  resumeAllowed?: boolean
}

interface RunEvent {
  seq: number
  type: string
  runId: string
  nodeId?: string
  gateId?: string
  data?: Record<string, unknown>
}

interface RunStartResult {
  runId: string
  status: RunStatusProjection
}

interface RunStatusResult {
  status: RunStatusProjection
}

interface LayoutNode {
  id: string
  x: number
  y: number
}

interface LayoutEdge {
  id: string
  lane: string
}

interface LayoutDocument {
  boardId: string
  boardRev: number
  etag: string
  nodes: LayoutNode[]
  edges?: LayoutEdge[]
}

interface ViewTransform {
  x: number
  y: number
  scale: number
}

interface ContextMenuItem {
  label: string
  action?: () => void
  destructive?: boolean
  disabled?: boolean
}

interface ContextMenuState {
  label: string
  x: number
  y: number
  items: ContextMenuItem[]
}

interface AgentProjection {
  id: string
  displayName?: string
  harnessDefault?: string
  assignable: boolean
  unbound?: boolean
  liveness?: string
}

interface BriefDraft {
  goal: string
  beadId: string
  files: string
  links: string
}

interface VerificationDraft {
  criterion: string
  onFail: 'block' | 'pushback'
}

interface TerminalPopup {
  agentId: string
  title: string
  liveness: string
  x: number
  y: number
  width: number
  height: number
  focusedAt: number
  dragged?: boolean
  resized?: boolean
}

type WireDragState =
  | { kind: 'new'; from: string }
  | { kind: 'reconnect-target'; connection: BoardConnection }
  | { kind: 'judge'; gateId: string }

type UndoAction =
  | { kind: 'deleteFormation'; formationId: string }
  | { kind: 'deleteGate'; gateId: string }
  | { kind: 'deleteMission'; missionId: string }
  | { kind: 'assignSlot'; formationId: string; slotId: string; agentId: string; harness: string }
  | { kind: 'makeController'; formationId: string; slotId: string }
  | { kind: 'setBrief'; formationId: string; brief?: FormationBrief }
  | { kind: 'setVerification'; formationId: string; verification?: FormationVerification }
  | { kind: 'removePort'; formationId: string; portId: string }
  | { kind: 'unwireConnection'; from: string; to: string }
  | { kind: 'moveNode'; node: LayoutNode }

type BoardUndoAction = Exclude<UndoAction, { kind: 'moveNode'; node: LayoutNode }>

class ApiRequestError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function fetchApi<T>(endpoint: string, init?: RequestInit): Promise<{ data: T; etag: string }> {
  const response = await fetch(endpoint, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    signal: AbortSignal.timeout(10000),
  })
  const result = await response.json() as ApiResponse<T>
  if (!response.ok || !result.success || !result.data) {
    throw new ApiRequestError(result.error?.message || `Request failed: ${response.status}`, response.status, result.error?.code || '')
  }
  return { data: result.data, etag: response.headers.get('ETag') || '' }
}

const cardWidth = 240
const cardHeight = 150
const fitPadding = 64

const typeLabels: Record<FormationNode['type'], string> = {
  solo: 'Solo',
  peer: 'Peer',
  flow: 'Flow',
  orchestrated: 'Orchestrated',
}

const runEventTypes = [
  'run_started',
  'run_resumed',
  'node_waiting',
  'node_started',
  'slot_dispatch',
  'slot_result',
  'node_output',
  'gate_evaluating',
  'gate_verdict',
  'human_input_requested',
  'human_verdict_recorded',
  'verification_verdict',
  'escalation_raised',
  'error',
  'run_blocked',
  'run_canceled',
  'run_failed',
  'run_succeeded',
]

const starterBoardID = 'brd_starter_session_search'
const starterBoardSlug = 'starter-session-search'
const starterBoardETag = 'starter-board-etag-1'
const starterLayoutETag = 'starter-layout-etag-1'
let starterLocalSequence = 0

function createStarterBoard(): BoardDocument {
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

function createStarterLayout(): LayoutDocument {
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

function starterBoardSummary(board: BoardDocument): BoardSummary {
  return {
    id: board.id,
    slug: board.slug,
    title: board.title,
    rev: board.rev,
    etag: board.etag,
  }
}

function isStarterBoard(board: BoardDocument | null | undefined): boolean {
  return board?.id === starterBoardID && board.slug === starterBoardSlug
}

function starterID(prefix: string): string {
  starterLocalSequence += 1
  return `${prefix}_starter_local_${starterLocalSequence}`
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

function withStarterLayoutRev(layout: LayoutDocument, boardRev: number): LayoutDocument {
  return {
    ...layout,
    boardRev,
    etag: `starter-layout-etag-${boardRev}-${starterLocalSequence}`,
  }
}

function applyStarterBoardPatch(
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

  const createGate = patch.createGate as Partial<{ title: string; kinds: string[]; criterion: string }> | undefined
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
        nodes: upsertNode(layout.nodes || [], { id: gate.id, x: 420, y: 220 }),
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

function defaultPosition(index: number): LayoutNode {
  return { id: '', x: 120 + index * 280, y: 120 + (index % 2) * 180 }
}

function round(value: number): number {
  return Math.round(value)
}

export default function FormationsView() {
  const [agents, setAgents] = useState<AgentProjection[]>([])
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [selectedSlug, setSelectedSlug] = useState('')
  const [board, setBoard] = useState<BoardDocument | null>(null)
  const [layout, setLayout] = useState<LayoutDocument | null>(null)
  const [title, setTitle] = useState('')
  const [missionTitle, setMissionTitle] = useState('')
  const [missionGoal, setMissionGoal] = useState('')
  const [missionBead, setMissionBead] = useState('')
  const [briefDrafts, setBriefDrafts] = useState<Record<string, BriefDraft>>({})
  const [verificationDrafts, setVerificationDrafts] = useState<Record<string, VerificationDraft>>({})
  const [activeRun, setActiveRun] = useState<RunStatusProjection | null>(null)
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
  const [error, setError] = useState('')
  const [view, setView] = useState<ViewTransform>({ x: 0, y: 0, scale: 1 })
  const [draggingNodeId, setDraggingNodeId] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [terminals, setTerminals] = useState<TerminalPopup[]>([])
  const boardETag = board?.etag || ''
  const layoutETag = layout?.etag || ''
  const canvasRef = useRef<HTMLDivElement | null>(null)
  const panning = useRef<{ startX: number; startY: number; view: ViewTransform } | null>(null)
  const boardRef = useRef<BoardDocument | null>(null)
  const layoutRef = useRef<LayoutDocument | null>(null)
  const wireDrag = useRef<WireDragState | null>(null)
  const undoStack = useRef<UndoAction[]>([])

  useEffect(() => {
    boardRef.current = board
  }, [board])

  useEffect(() => {
    layoutRef.current = layout
  }, [layout])

  const layoutByNode = useMemo(() => {
    const byID = new Map<string, LayoutNode>()
    layout?.nodes?.forEach(node => byID.set(node.id, node))
    return byID
  }, [layout])

  const layoutEdgeByID = useMemo(() => {
    const byID = new Map<string, LayoutEdge>()
    layout?.edges?.forEach(edge => byID.set(edge.id, edge))
    return byID
  }, [layout])

  const positionedFormations = useMemo(() => (board?.formations || []).map((formation, index) => {
    const fallback = defaultPosition(index)
    return {
      formation,
      position: layoutByNode.get(formation.id) || { ...fallback, id: formation.id },
    }
  }), [board, layoutByNode])

  const positionedGates = useMemo(() => (board?.gates || []).map((gate, index) => ({
    gate,
    position: layoutByNode.get(gate.id) || { id: gate.id, x: 160 + index * 280, y: 420 },
  })), [board, layoutByNode])

  const positionedMissions = useMemo(() => (board?.missions || []).map((mission, index) => ({
    mission,
    position: layoutByNode.get(mission.id) || { id: mission.id, x: 80 + index * 280, y: -120 },
  })), [board, layoutByNode])

  const escalatingNodeIds = useMemo(() => new Set(runEvents
    .filter(event => event.type === 'escalation_raised' && event.nodeId)
    .map(event => event.nodeId as string)), [runEvents])

  const positionedCanvasNodes = useMemo(() => [
    ...positionedFormations.map(item => item.position),
    ...positionedGates.map(item => item.position),
    ...positionedMissions.map(item => item.position),
  ], [positionedFormations, positionedGates, positionedMissions])

  const connectionRoutes = useMemo(() => {
    const positions = new Map([
      ...positionedFormations.map(item => [item.formation.id, item.position] as const),
      ...positionedGates.map(item => [item.gate.id, item.position] as const),
      ...positionedMissions.map(item => [item.mission.id, item.position] as const),
    ])
    const obstacles: ObstacleRect[] = [
      ...positionedFormations.map(item => ({
      id: item.formation.id,
      x: item.position.x,
      y: item.position.y,
      width: cardWidth,
      height: cardHeight,
      })),
      ...positionedGates.map(item => ({
        id: item.gate.id,
        x: item.position.x,
        y: item.position.y,
        width: cardWidth,
        height: cardHeight,
      })),
      ...positionedMissions.map(item => ({
        id: item.mission.id,
        x: item.position.x,
        y: item.position.y,
        width: cardWidth,
        height: cardHeight,
      })),
    ]
    return (board?.connections || []).flatMap(connection => {
      const fromId = endpointNodeId(connection.from)
      const toId = endpointNodeId(connection.to)
      const from = positions.get(fromId)
      const to = positions.get(toId)
      if (!from || !to) return []
      const route = routeFormationWire(
        { x: from.x + cardWidth, y: from.y + cardHeight / 2 },
        { x: to.x, y: to.y + cardHeight / 2 },
        {
          fromId,
          toId,
          draggingNodeId,
          obstacles: obstacles.filter(obstacle => obstacle.id !== fromId && obstacle.id !== toId),
        }
      )
      return [{ connection, fromId, toId, route }]
    })
  }, [board, draggingNodeId, positionedFormations, positionedGates, positionedMissions])

  const inputPortOptions = useMemo(() => (board?.formations || []).flatMap(formation => (
    (formation.inputs || []).map(input => ({
      endpoint: `${formation.id}:${input.id}`,
      label: `${formation.title} / ${input.label}`,
    }))
  )), [board])

  const loadBoard = useCallback(async (slug: string) => {
    const boardResult = await fetchApi<{ board: BoardDocument }>(`/api/formations/boards/${encodeURIComponent(slug)}`)
    const nextBoard = {
      ...boardResult.data.board,
      etag: boardResult.etag || boardResult.data.board.etag,
      missions: boardResult.data.board.missions || [],
      formations: boardResult.data.board.formations || [],
      gates: boardResult.data.board.gates || [],
      connections: boardResult.data.board.connections || [],
    }
    let nextLayout: LayoutDocument
    try {
      const layoutResult = await fetchApi<{ layout: LayoutDocument }>(`/api/formations/boards/${encodeURIComponent(slug)}/layout`)
      nextLayout = {
        ...layoutResult.data.layout,
        etag: layoutResult.etag || layoutResult.data.layout.etag,
        nodes: layoutResult.data.layout.nodes || [],
        edges: layoutResult.data.layout.edges || [],
      }
    } catch (err) {
      if (!(err instanceof ApiRequestError) || (err.status !== 404 && err.code !== 'NOT_FOUND')) throw err
      nextLayout = {
        boardId: nextBoard.id,
        boardRev: nextBoard.rev,
        etag: '*',
        nodes: [],
        edges: [],
      }
    }
    boardRef.current = nextBoard
    layoutRef.current = nextLayout
    setBoard(nextBoard)
    setLayout(nextLayout)
    setSelectedSlug(slug)
  }, [])

  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const result = await fetchApi<{ boards: BoardSummary[] }>('/api/formations/boards')
        if (cancelled) return
        const summaries = result.data.boards || []
        setBoards(summaries)
        if (summaries[0]) {
          await loadBoard(summaries[0].slug)
        } else {
          const starterBoard = createStarterBoard()
          const starterLayout = createStarterLayout()
          boardRef.current = starterBoard
          layoutRef.current = starterLayout
          setBoards([starterBoardSummary(starterBoard)])
          setBoard(starterBoard)
          setLayout(starterLayout)
          setSelectedSlug(starterBoard.slug)
          setView({ x: 24, y: 24, scale: 0.88 })
        }
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load formations')
      }
    }
    load()
    return () => {
      cancelled = true
    }
  }, [loadBoard])

  useEffect(() => {
    let cancelled = false
    async function loadAgents() {
      try {
        const result = await fetchApi<{ agents: AgentProjection[] }>('/api/agents')
        if (!cancelled) {
          setAgents((result.data.agents || []).filter(agent => agent.assignable && !agent.unbound))
        }
      } catch {
        if (!cancelled) setAgents([])
      }
    }
    loadAgents()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!activeRun?.runId || activeRun.final || typeof EventSource === 'undefined') return
    const source = new EventSource(`/api/formations/runs/${encodeURIComponent(activeRun.runId)}/stream?since=0`)
    const handleEvent = (message: MessageEvent) => {
      try {
        const event = JSON.parse(message.data) as RunEvent
        setRunEvents(previous => upsertRunEvent(previous, event))
        const status = statusFromRunEvent(event)
        if (status) {
          setActiveRun(previous => previous && previous.runId === event.runId
            ? {
              ...previous,
              status,
              final: status !== 'blocked' && status !== 'running',
              eventCount: Math.max(previous.eventCount || 0, event.seq || 0),
              resumeAllowed: runEventResumeAllowed(event, previous.resumeAllowed || false),
            }
            : previous)
          if (status !== 'blocked' && status !== 'running') source.close()
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to read run event')
      }
    }
    runEventTypes.forEach(type => source.addEventListener(type, handleEvent))
    return () => {
      runEventTypes.forEach(type => source.removeEventListener(type, handleEvent))
      source.close()
    }
  }, [activeRun?.final, activeRun?.runId])

  useEffect(() => {
    if (!selectedSlug || !board || isStarterBoard(board)) return
    let cancelled = false
    const checkChanges = async () => {
      try {
        const result = await fetchApi<{ signal: { changed?: boolean } }>(
          `/api/formations/boards/${encodeURIComponent(selectedSlug)}/changes?rev=${encodeURIComponent(String(board.rev))}`
        )
        if (!cancelled && result.data.signal?.changed) {
          await loadBoard(selectedSlug)
        }
      } catch {
        // Board-change signals are best-effort; direct edits still use ETag failures.
      }
    }
    const timer = window.setInterval(() => {
      void checkChanges()
    }, 600)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [board, loadBoard, selectedSlug])

  useEffect(() => {
    if (!selectedSlug || isStarterBoard(board)) return
    const runID = window.localStorage.getItem(activeRunStorageKey(selectedSlug))
    if (!runID || activeRun?.runId === runID) return
    const restoredRunID = runID
    let cancelled = false
    async function restoreRun() {
      try {
        const statusResult = await fetchApi<RunStatusProjection | RunStatusResult>(`/api/formations/runs/${encodeURIComponent(restoredRunID)}`)
        if (cancelled) return
        const status = runStatusFromResponse(statusResult.data)
        setActiveRun(status)
        try {
          const eventsResult = await fetchApi<{ events: RunEvent[] }>(`/api/formations/runs/${encodeURIComponent(restoredRunID)}/events`)
          if (!cancelled) setRunEvents((eventsResult.data.events || []).sort((a, b) => a.seq - b.seq))
        } catch {
          if (!cancelled) setRunEvents([])
        }
        if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      } catch {
        if (!cancelled) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      }
    }
    void restoreRun()
    return () => {
      cancelled = true
    }
  }, [activeRun?.runId, selectedSlug])

  const createFormation = useCallback(async (type: FormationNode['type']) => {
    if (!board) return
    const requestedTitle = title.trim() || typeLabels[type]
    try {
      if (isStarterBoard(board)) {
        const before = boardRef.current
        const result = applyStarterBoardPatch(board, layoutRef.current || createStarterLayout(), {
          createFormation: {
            type,
            title: requestedTitle,
            x: 120,
            y: 120,
          },
        })
        boardRef.current = result.board
        setBoard(result.board)
        setBoards([starterBoardSummary(result.board)])
        if (result.layout) {
          layoutRef.current = result.layout
          setLayout(result.layout)
        }
        const formation = findAddedByID(before?.formations || [], result.board.formations || [])
        if (formation) undoStack.current.push({ kind: 'deleteFormation', formationId: formation.id })
        setTitle('')
        setError('')
        return
      }
      const result = await fetchApi<{ board: BoardDocument; layout: LayoutDocument; formation: FormationNode }>(
        `/api/formations/boards/${encodeURIComponent(board.slug)}`,
        {
          method: 'PATCH',
          headers: { 'If-Match': boardETag },
          body: JSON.stringify({
            expectedRev: board.rev,
            updatedBy: 'agent:ui',
            createFormation: {
              type,
              title: requestedTitle,
              x: 120,
              y: 120,
            },
          }),
        }
      )
      const nextBoard = {
        ...result.data.board,
        etag: result.etag || result.data.board.etag,
        missions: result.data.board.missions || [],
        formations: result.data.board.formations || [],
        gates: result.data.board.gates || [],
        connections: result.data.board.connections || [],
      }
      const nextLayout = {
        ...result.data.layout,
        etag: result.data.layout.etag,
        nodes: result.data.layout.nodes || [],
        edges: result.data.layout.edges || [],
      }
      boardRef.current = nextBoard
      layoutRef.current = nextLayout
      setBoard(nextBoard)
      setLayout(nextLayout)
      undoStack.current.push({ kind: 'deleteFormation', formationId: result.data.formation.id })
      setTitle('')
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create formation')
    }
  }, [board, boardETag, title])

  const patchBoard = useCallback(async (patch: Record<string, unknown>) => {
    const currentBoard = boardRef.current
    if (!currentBoard) return null
    if (isStarterBoard(currentBoard)) {
      const result = applyStarterBoardPatch(currentBoard, layoutRef.current || createStarterLayout(), patch)
      boardRef.current = result.board
      setBoard(result.board)
      setBoards([starterBoardSummary(result.board)])
      if (result.layout) {
        layoutRef.current = result.layout
        setLayout(result.layout)
      }
      setError('')
      return result.board
    }
    const result = await fetchApi<{ board: BoardDocument; layout?: LayoutDocument }>(
      `/api/formations/boards/${encodeURIComponent(currentBoard.slug)}`,
      {
        method: 'PATCH',
        headers: { 'If-Match': currentBoard.etag },
        body: JSON.stringify({
          expectedRev: currentBoard.rev,
          updatedBy: 'agent:ui',
          ...patch,
        }),
      }
    )
    const nextBoard = {
      ...result.data.board,
      etag: result.etag || result.data.board.etag,
      missions: result.data.board.missions || [],
      formations: result.data.board.formations || [],
      gates: result.data.board.gates || [],
      connections: result.data.board.connections || [],
    }
    boardRef.current = nextBoard
    setBoard(nextBoard)
    if (result.data.layout) {
      const nextLayout = {
        ...result.data.layout,
        etag: result.data.layout.etag,
        nodes: result.data.layout.nodes || [],
        edges: result.data.layout.edges || [],
      }
      layoutRef.current = nextLayout
      setLayout(nextLayout)
    }
    setError('')
    return nextBoard
  }, [])

  const createGate = useCallback(async () => {
    try {
      const before = boardRef.current
      const nextBoard = await patchBoard({
        createGate: {
          title: 'Review gate',
          kinds: ['code'],
          criterion: '',
        },
      })
      const gate = findAddedByID(before?.gates || [], nextBoard?.gates || [])
      if (gate) undoStack.current.push({ kind: 'deleteGate', gateId: gate.id })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create gate')
    }
  }, [patchBoard])

  const setGateJudgeChain = useCallback(async (gate: GateNode, chain: string[]) => {
    if (chain.length === 0) return
    try {
      await patchBoard({
        setGateJudge: {
          gateId: gate.id,
          chain,
        },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to attach judge')
    }
  }, [patchBoard])

  const setGateJudge = useCallback(async (gate: GateNode, value: string) => {
    if (!value) return
    await setGateJudgeChain(gate, [value])
  }, [setGateJudgeChain])

  const detachGateJudge = useCallback(async (gate: GateNode) => {
    try {
      await patchBoard({
        detachGateJudge: {
          gateId: gate.id,
        },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to detach judge')
    }
  }, [patchBoard])

  const createJudgeFormation = useCallback(async (gate: GateNode) => {
    try {
      const before = boardRef.current
      const nextBoard = await patchBoard({
        createFormation: {
          type: 'solo',
          title: 'Judge formation',
          x: (layoutByNode.get(gate.id)?.x || 360) + 120,
          y: (layoutByNode.get(gate.id)?.y || 220) - 160,
        },
      })
      const created = findAddedByID(before?.formations || [], nextBoard?.formations || [])
      if (created) await setGateJudgeChain(gate, [created.id])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create judge')
    }
  }, [layoutByNode, patchBoard, setGateJudgeChain])

  const createMission = useCallback(async () => {
    try {
      const before = boardRef.current
      const nextBoard = await patchBoard({
        createMission: {
          title: missionTitle.trim() || 'Mission',
          goal: missionGoal.trim(),
          beadId: missionBead.trim(),
        },
      })
      const mission = findAddedByID(before?.missions || [], nextBoard?.missions || [])
      if (mission) undoStack.current.push({ kind: 'deleteMission', missionId: mission.id })
      setMissionTitle('')
      setMissionGoal('')
      setMissionBead('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create mission')
    }
  }, [missionBead, missionGoal, missionTitle, patchBoard])

  const startMission = useCallback(async (mission: MissionNode) => {
    const currentBoard = boardRef.current
    if (!currentBoard) return
    if (isStarterBoard(currentBoard)) {
      const runId = 'run-starter-preview'
      const events: RunEvent[] = [
        { seq: 1, type: 'run_started', runId, data: { actor: 'agent:ui' } },
        { seq: 2, type: 'run_succeeded', runId, nodeId: mission.id, data: { text: 'starter board preview run' } },
      ]
      setActiveRun({
        runId,
        status: 'succeeded',
        final: true,
        boardSlug: currentBoard.slug,
        missionId: mission.id,
        eventCount: events.length,
        resumeAllowed: false,
      })
      setRunEvents(events)
      setError('')
      return
    }
    try {
      const result = await fetchApi<RunStartResult>('/api/formations/runs', {
        method: 'POST',
        headers: { 'If-Match': currentBoard.etag },
        body: JSON.stringify({
          board: currentBoard.slug,
          missionId: mission.id,
          actor: 'agent:ui',
        }),
      })
      setActiveRun({
        ...result.data.status,
        runId: result.data.status.runId || result.data.runId,
      })
      window.localStorage.setItem(activeRunStorageKey(currentBoard.slug), result.data.status.runId || result.data.runId)
      setRunEvents([])
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start run')
    }
  }, [])

  const abortRun = useCallback(async () => {
    if (!activeRun?.runId || activeRun.final) return
    try {
      const result = await fetchApi<RunStatusProjection | RunStatusResult>(
        `/api/formations/runs/${encodeURIComponent(activeRun.runId)}/abort`,
        {
          method: 'POST',
          body: JSON.stringify({
            reason: 'operator stop',
            requestedBy: 'agent:ui',
          }),
        }
      )
      setActiveRun(runStatusFromResponse(result.data))
      if (runStatusFromResponse(result.data).final && selectedSlug) {
        window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to abort run')
    }
  }, [activeRun, selectedSlug])

  const resumeRun = useCallback(async () => {
    if (!activeRun?.runId || activeRun.final || !activeRun.resumeAllowed) return
    try {
      const result = await fetchApi<RunStatusProjection | RunStatusResult>(
        `/api/formations/runs/${encodeURIComponent(activeRun.runId)}/resume`,
        {
          method: 'POST',
          body: JSON.stringify({
            actor: 'agent:ui',
            mode: 'reattach',
            reason: 'operator resume',
          }),
        }
      )
      setActiveRun(runStatusFromResponse(result.data))
      if (selectedSlug) {
        window.localStorage.setItem(activeRunStorageKey(selectedSlug), runStatusFromResponse(result.data).runId)
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to resume run')
    }
  }, [activeRun, selectedSlug])

  const recordHumanGateVerdict = useCallback(async (gateId: string, verdict: 'pass' | 'fail') => {
    if (!activeRun?.runId || activeRun.final) return
    try {
      const result = await fetchApi<RunStatusProjection | RunStatusResult>(
        `/api/formations/runs/${encodeURIComponent(activeRun.runId)}/gates/${encodeURIComponent(gateId)}/verdict`,
        {
          method: 'POST',
          body: JSON.stringify({
            actor: 'agent:ui',
            verdict,
            reason: verdict === 'pass' ? 'operator approved' : 'operator rejected',
          }),
        }
      )
      setActiveRun(runStatusFromResponse(result.data))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to record human verdict')
    }
  }, [activeRun])

  const closeContextMenu = useCallback(() => {
    setContextMenu(null)
  }, [])

  const openContextMenu = useCallback((event: MouseEvent<Element>, label: string, items: ContextMenuItem[]) => {
    event.preventDefault()
    event.stopPropagation()
    const width = 220
    const height = Math.max(48, items.length * 34 + 12)
    setContextMenu({
      label,
      x: Math.max(8, Math.min(event.clientX, window.innerWidth - width - 8)),
      y: Math.max(8, Math.min(event.clientY, window.innerHeight - height - 8)),
      items,
    })
  }, [])

  const assignSlot = useCallback(async (formation: FormationNode, slot: FormationSlot, value: string) => {
    const [agentId, harness] = splitAgentValue(value)
    try {
      await patchBoard({
        assignSlot: {
          formationId: formation.id,
          slotId: slot.id,
          agentId,
          harness,
        },
      })
      undoStack.current.push({
        kind: 'assignSlot',
        formationId: formation.id,
        slotId: slot.id,
        agentId: slot.agentId || '',
        harness: slot.harness || '',
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to assign slot')
    }
  }, [patchBoard])

  const makeController = useCallback(async (formation: FormationNode, slot: FormationSlot) => {
    try {
      const previousController = formation.slots.find(candidate => candidate.controller)
      await patchBoard({
        makeController: {
          formationId: formation.id,
          slotId: slot.id,
        },
      })
      if (previousController && previousController.id !== slot.id) {
        undoStack.current.push({
          kind: 'makeController',
          formationId: formation.id,
          slotId: previousController.id,
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update controller')
    }
  }, [patchBoard])

  const updateBriefDraft = useCallback((formation: FormationNode, patch: Partial<BriefDraft>) => {
    setBriefDrafts(previous => {
      const current = previous[formation.id] || {
        goal: formation.brief?.goal || '',
        beadId: formation.brief?.beadId || '',
        files: (formation.brief?.files || []).join(', '),
        links: (formation.brief?.links || []).join(', '),
      }
      return {
        ...previous,
        [formation.id]: { ...current, ...patch },
      }
    })
  }, [])

  const saveBrief = useCallback(async (formation: FormationNode) => {
    const draft = briefDrafts[formation.id] || {
      goal: formation.brief?.goal || '',
      beadId: formation.brief?.beadId || '',
      files: (formation.brief?.files || []).join(', '),
      links: (formation.brief?.links || []).join(', '),
    }
    try {
      await patchBoard({
        setBrief: {
          formationId: formation.id,
          goal: draft.goal.trim(),
          beadId: draft.beadId.trim(),
          files: splitRefs(draft.files),
          links: splitRefs(draft.links),
        },
      })
      undoStack.current.push({
        kind: 'setBrief',
        formationId: formation.id,
        brief: formation.brief ? cloneBrief(formation.brief) : undefined,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save brief')
    }
  }, [briefDrafts, patchBoard])

  const updateVerificationDraft = useCallback((formation: FormationNode, patch: Partial<VerificationDraft>) => {
    setVerificationDrafts(previous => {
      const current = previous[formation.id] || {
        criterion: formation.verification?.criterion || '',
        onFail: formation.verification?.onFail || 'block',
      }
      return {
        ...previous,
        [formation.id]: { ...current, ...patch },
      }
    })
  }, [])

  const saveVerification = useCallback(async (formation: FormationNode) => {
    const draft = verificationDrafts[formation.id] || {
      criterion: formation.verification?.criterion || '',
      onFail: formation.verification?.onFail || 'block',
    }
    try {
      await patchBoard({
        setVerification: {
          formationId: formation.id,
          kinds: ['code'],
          criterion: draft.criterion.trim(),
          onFail: draft.onFail,
        },
      })
      undoStack.current.push({
        kind: 'setVerification',
        formationId: formation.id,
        verification: formation.verification ? cloneVerification(formation.verification) : undefined,
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save verification')
    }
  }, [patchBoard, verificationDrafts])

  const addPort = useCallback(async (formation: FormationNode, direction: 'input' | 'output') => {
    try {
      const before = boardRef.current
      const nextBoard = await patchBoard({
        addPort: {
          formationId: formation.id,
          direction,
          label: direction === 'input' ? 'Input' : 'Output',
        },
      })
      const addedPort = findAddedPort(before, nextBoard, formation.id, direction)
      if (addedPort) undoStack.current.push({ kind: 'removePort', formationId: formation.id, portId: addedPort.id })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add port')
    }
  }, [patchBoard])

  const wireConnection = useCallback(async (formation: FormationNode, output: FormationPort, to: string) => {
    if (!to) return
    const from = `${formation.id}:${output.id}`
    try {
      await patchBoard({
        wireConnection: {
          from,
          to,
        },
      })
      undoStack.current.push({ kind: 'unwireConnection', from, to })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to wire connection')
    }
  }, [patchBoard])

  const wireEndpoints = useCallback(async (from: string, to: string) => {
    if (!from || !to || endpointNodeId(from) === endpointNodeId(to)) return
    try {
      await patchBoard({
        wireConnection: {
          from,
          to,
        },
      })
      undoStack.current.push({ kind: 'unwireConnection', from, to })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to wire connection')
    }
  }, [patchBoard])

  const rewireTarget = useCallback(async (connection: BoardConnection, to: string) => {
    if (!to || connection.to === to || endpointNodeId(connection.from) === endpointNodeId(to)) return
    try {
      await patchBoard({
        unwireConnection: {
          from: connection.from,
          to: connection.to,
        },
      })
      await patchBoard({
        wireConnection: {
          from: connection.from,
          to,
        },
      })
      undoStack.current.push({ kind: 'unwireConnection', from: connection.from, to })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reconnect wire')
    }
  }, [patchBoard])

  const patchLayoutEdges = useCallback(async (edges: LayoutEdge[]) => {
    const currentBoard = boardRef.current
    const currentLayout = layoutRef.current
    if (!currentBoard || !currentLayout) return
    if (isStarterBoard(currentBoard)) {
      const nextLayout = withStarterLayoutRev({
        ...currentLayout,
        edges: [
          ...(currentLayout.edges || []).filter(edge => !edges.some(patch => patch.id === edge.id)),
          ...edges,
        ],
      }, currentBoard.rev)
      layoutRef.current = nextLayout
      setLayout(nextLayout)
      setError('')
      return
    }
    try {
      const result = await fetchApi<{ layout: LayoutDocument }>(
        `/api/formations/boards/${encodeURIComponent(currentBoard.slug)}/layout`,
        {
          method: 'PATCH',
          headers: { 'If-Match': currentLayout.etag },
          body: JSON.stringify({ edges }),
        }
      )
      const nextLayout = {
        ...result.data.layout,
        etag: result.etag || result.data.layout.etag,
        nodes: result.data.layout.nodes || [],
        edges: result.data.layout.edges || [],
      }
      layoutRef.current = nextLayout
      setLayout(nextLayout)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update wire routing')
    }
  }, [])

  const resetWireRoute = useCallback(async (connection: BoardConnection) => {
    await patchLayoutEdges([{ id: connection.id, lane: 'auto' }])
  }, [patchLayoutEdges])

  const handRouteWire = useCallback(async (connection: BoardConnection) => {
    await patchLayoutEdges([{ id: connection.id, lane: 'manual' }])
  }, [patchLayoutEdges])

  const removeWire = useCallback(async (connection: BoardConnection) => {
    try {
      await patchBoard({
        unwireConnection: {
          from: connection.from,
          to: connection.to,
        },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to remove wire')
    }
  }, [patchBoard])

  const finishWireDrag = useCallback(async (drag: WireDragState, target: Element | null, point: { x: number; y: number }) => {
    const hit = formationDropTarget(target, point)
    const endpoint = hit.endpoint
    const judgeGateID = hit.judgeGateID
    const formationID = hit.formationID
    const currentBoard = boardRef.current
    if (!currentBoard) return

    if (drag.kind === 'new') {
      if (judgeGateID) {
        const gate = currentBoard.gates?.find(candidate => candidate.id === judgeGateID)
        if (gate) await setGateJudgeChain(gate, [endpointNodeId(drag.from)])
        return
      }
      if (endpoint) await wireEndpoints(drag.from, endpoint)
      return
    }

    if (drag.kind === 'reconnect-target') {
      if (endpoint) await rewireTarget(drag.connection, endpoint)
      return
    }

    const gate = currentBoard.gates?.find(candidate => candidate.id === drag.gateId)
    if (!gate) return
    if (formationID) {
      await setGateJudgeChain(gate, [formationID])
      return
    }
    await createJudgeFormation(gate)
  }, [createJudgeFormation, rewireTarget, setGateJudgeChain, wireEndpoints])

  const beginWireDrag = useCallback((event: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>, drag: WireDragState) => {
    if (event.button !== 0) return
    if (wireDrag.current) return
    event.preventDefault()
    event.stopPropagation()
    wireDrag.current = drag
    const finish = (clientX: number, clientY: number) => {
      const active = wireDrag.current
      wireDrag.current = null
      window.removeEventListener('pointerup', up)
      window.removeEventListener('mouseup', mouseUp)
      if (!active) return
      const target = document.elementFromPoint(clientX, clientY)
      void finishWireDrag(active, target, { x: clientX, y: clientY })
    }
    const up = (upEvent: globalThis.PointerEvent) => {
      finish(upEvent.clientX, upEvent.clientY)
    }
    const mouseUp = (upEvent: globalThis.MouseEvent) => {
      finish(upEvent.clientX, upEvent.clientY)
    }
    window.addEventListener('pointerup', up)
    window.addEventListener('mouseup', mouseUp)
  }, [finishWireDrag])

  const openTerminal = useCallback((formation: FormationNode, slot: FormationSlot) => {
    if (!slot.agentId) return
    const agent = agents.find(candidate => candidate.id === slot.agentId)
    const title = agent?.displayName || slot.agentId
    const liveness = agent?.liveness || 'unknown'
    const position = layoutByNode.get(formation.id) || { id: formation.id, x: 160, y: 160 }
    setTerminals(previous => {
      const now = Date.now()
      const existing = previous.find(terminal => terminal.agentId === slot.agentId)
      if (existing) {
        return previous.map(terminal => terminal.agentId === slot.agentId ? { ...terminal, focusedAt: now } : terminal)
      }
      return [
        ...previous,
        {
          agentId: slot.agentId || '',
          title,
          liveness,
          x: position.x + 36,
          y: position.y + cardHeight + 28,
          width: 430,
          height: 260,
          focusedAt: now,
        },
      ]
    })
  }, [agents, layoutByNode])

  const closeTerminal = useCallback((agentId: string) => {
    setTerminals(previous => previous.filter(terminal => terminal.agentId !== agentId))
  }, [])

  const moveTerminal = useCallback((agentId: string, event: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    const terminal = terminals.find(candidate => candidate.agentId === agentId)
    if (!terminal) return
    const start = { x: event.clientX, y: event.clientY, terminal }
    const moveTo = (clientX: number, clientY: number) => {
      setTerminals(previous => previous.map(candidate => candidate.agentId === agentId
        ? {
          ...candidate,
          x: round(start.terminal.x + (clientX - start.x) / view.scale),
          y: round(start.terminal.y + (clientY - start.y) / view.scale),
          dragged: true,
          focusedAt: Date.now(),
        }
        : candidate))
    }
    const move = (moveEvent: globalThis.PointerEvent) => moveTo(moveEvent.clientX, moveEvent.clientY)
    const mouseMove = (moveEvent: globalThis.MouseEvent) => moveTo(moveEvent.clientX, moveEvent.clientY)
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('mousemove', mouseMove)
      window.removeEventListener('mouseup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
    window.addEventListener('mousemove', mouseMove)
    window.addEventListener('mouseup', up)
  }, [terminals, view.scale])

  const resizeTerminal = useCallback((agentId: string, event: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    const terminal = terminals.find(candidate => candidate.agentId === agentId)
    if (!terminal) return
    const start = { x: event.clientX, y: event.clientY, terminal }
    const moveTo = (clientX: number, clientY: number) => {
      setTerminals(previous => previous.map(candidate => candidate.agentId === agentId
        ? {
          ...candidate,
          width: Math.max(280, round(start.terminal.width + (clientX - start.x) / view.scale)),
          height: Math.max(180, round(start.terminal.height + (clientY - start.y) / view.scale)),
          resized: true,
          focusedAt: Date.now(),
        }
        : candidate))
    }
    const move = (moveEvent: globalThis.PointerEvent) => moveTo(moveEvent.clientX, moveEvent.clientY)
    const mouseMove = (moveEvent: globalThis.MouseEvent) => moveTo(moveEvent.clientX, moveEvent.clientY)
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('mousemove', mouseMove)
      window.removeEventListener('mouseup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
    window.addEventListener('mousemove', mouseMove)
    window.addEventListener('mouseup', up)
  }, [terminals, view.scale])

  const beginJudgeSocketPointer = useCallback((event: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>, gate: GateNode) => {
    if (event.button !== 0) return
    event.stopPropagation()
    const start = { x: event.clientX, y: event.clientY }
    let dragging = false
    const noteMove = (clientX: number, clientY: number) => {
      const distance = Math.hypot(clientX - start.x, clientY - start.y)
      if (distance > 8) dragging = true
    }
    const move = (moveEvent: globalThis.PointerEvent) => noteMove(moveEvent.clientX, moveEvent.clientY)
    const mouseMove = (moveEvent: globalThis.MouseEvent) => noteMove(moveEvent.clientX, moveEvent.clientY)
    const finish = (clientX: number, clientY: number) => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('mousemove', mouseMove)
      window.removeEventListener('mouseup', mouseUp)
      if (!dragging) return
      const target = document.elementFromPoint(clientX, clientY)
      void finishWireDrag({ kind: 'judge', gateId: gate.id }, target, { x: clientX, y: clientY })
    }
    const up = (upEvent: globalThis.PointerEvent) => {
      finish(upEvent.clientX, upEvent.clientY)
    }
    const mouseUp = (upEvent: globalThis.MouseEvent) => {
      finish(upEvent.clientX, upEvent.clientY)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
    window.addEventListener('mousemove', mouseMove)
    window.addEventListener('mouseup', mouseUp)
  }, [finishWireDrag])

  const saveNodeLayout = useCallback(async (node: LayoutNode, undoNode?: LayoutNode) => {
    if (!board || !layout) return
    if (isStarterBoard(board)) {
      const nextLayout = withStarterLayoutRev({
        ...layout,
        nodes: upsertNode(layout.nodes || [], node),
      }, board.rev)
      layoutRef.current = nextLayout
      setLayout(nextLayout)
      if (undoNode && (undoNode.x !== node.x || undoNode.y !== node.y)) {
        undoStack.current.push({ kind: 'moveNode', node: undoNode })
      }
      setError('')
      return
    }
    try {
      const result = await fetchApi<{ layout: LayoutDocument }>(
        `/api/formations/boards/${encodeURIComponent(board.slug)}/layout`,
        {
          method: 'PATCH',
          headers: { 'If-Match': layoutETag },
          body: JSON.stringify({ nodes: [node] }),
        }
      )
      const nextLayout = {
        ...result.data.layout,
        etag: result.etag || result.data.layout.etag,
        nodes: result.data.layout.nodes || [],
        edges: result.data.layout.edges || [],
      }
      layoutRef.current = nextLayout
      setLayout(nextLayout)
      if (undoNode && (undoNode.x !== node.x || undoNode.y !== node.y)) {
        undoStack.current.push({ kind: 'moveNode', node: undoNode })
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save layout')
    }
  }, [board, layout, layoutETag])

  const performUndo = useCallback(async () => {
    const action = undoStack.current.pop()
    if (!action) return
    try {
      if (action.kind === 'moveNode') {
        const currentBoard = boardRef.current
        const currentLayout = layoutRef.current
        if (!currentBoard || !currentLayout) throw new Error('No formation layout loaded')
        if (isStarterBoard(currentBoard)) {
          const nextLayout = withStarterLayoutRev({
            ...currentLayout,
            nodes: upsertNode(currentLayout.nodes || [], action.node),
          }, currentBoard.rev)
          layoutRef.current = nextLayout
          setLayout(nextLayout)
          setError('')
          return
        }
        const result = await fetchApi<{ layout: LayoutDocument }>(
          `/api/formations/boards/${encodeURIComponent(currentBoard.slug)}/layout`,
          {
            method: 'PATCH',
            headers: { 'If-Match': currentLayout.etag },
            body: JSON.stringify({ nodes: [action.node] }),
          }
        )
        const nextLayout = {
          ...result.data.layout,
          etag: result.etag || result.data.layout.etag,
          nodes: result.data.layout.nodes || [],
          edges: result.data.layout.edges || [],
        }
        layoutRef.current = nextLayout
        setLayout(nextLayout)
      } else {
        const nextBoard = await patchBoard(undoBoardPatch(action))
        if (!nextBoard) throw new Error('No formation board loaded')
      }
      setError('')
    } catch (err) {
      undoStack.current.push(action)
      setError(err instanceof Error ? err.message : 'Failed to undo formation change')
    }
  }, [patchBoard])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() !== 'z' || (!event.ctrlKey && !event.metaKey) || event.shiftKey) return
      if (isTextEditingTarget(event.target) || isTextEditingTarget(document.activeElement)) return
      event.preventDefault()
      void performUndo()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [performUndo])

  const startPan = useCallback((event: PointerEvent<HTMLElement>) => {
    closeContextMenu()
    const target = event.target instanceof Element ? event.target : null
    if (event.button !== 0 || target?.closest('.formation-card')) return
    panning.current = { startX: event.clientX, startY: event.clientY, view }
    const move = (moveEvent: globalThis.PointerEvent) => {
      const active = panning.current
      if (!active) return
      setView({
        ...active.view,
        x: round(active.view.x + moveEvent.clientX - active.startX),
        y: round(active.view.y + moveEvent.clientY - active.startY),
      })
    }
    const up = () => {
      panning.current = null
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }, [closeContextMenu, view])

  const zoomBy = useCallback((factor: number, cursor?: { x: number; y: number }) => {
    setView(current => ({
      ...zoomTransform(current, factor, cursor),
    }))
  }, [])

  const fit = useCallback(() => {
    if (positionedCanvasNodes.length === 0) {
      setView({ x: 0, y: 0, scale: 1 })
      return
    }
    const rect = canvasRef.current?.getBoundingClientRect()
    const viewportWidth = rect?.width || 800
    const viewportHeight = rect?.height || 500
    const bounds = positionedCanvasNodes.reduce((acc, position) => ({
      minX: Math.min(acc.minX, position.x),
      minY: Math.min(acc.minY, position.y),
      maxX: Math.max(acc.maxX, position.x + cardWidth),
      maxY: Math.max(acc.maxY, position.y + cardHeight),
    }), {
      minX: Number.POSITIVE_INFINITY,
      minY: Number.POSITIVE_INFINITY,
      maxX: Number.NEGATIVE_INFINITY,
      maxY: Number.NEGATIVE_INFINITY,
    })
    const contentWidth = Math.max(1, bounds.maxX - bounds.minX)
    const contentHeight = Math.max(1, bounds.maxY - bounds.minY)
    const scale = clampScale(Math.min(
      (viewportWidth - fitPadding * 2) / contentWidth,
      (viewportHeight - fitPadding * 2) / contentHeight
    ), 1.2)
    setView({
      x: round((viewportWidth - contentWidth * scale) / 2 - bounds.minX * scale),
      y: round((viewportHeight - contentHeight * scale) / 2 - bounds.minY * scale),
      scale,
    })
  }, [positionedCanvasNodes])

  const moveNode = useCallback((formation: FormationNode, event: PointerEvent<HTMLElement>) => {
    if (event.button !== 0) return
    event.stopPropagation()
    setDraggingNodeId(formation.id)
    const fallback = defaultPosition(board?.formations.findIndex(item => item.id === formation.id) ?? 0)
    const current = layoutByNode.get(formation.id) || { ...fallback, id: formation.id }
    const start = { x: event.clientX, y: event.clientY, node: current }
    let latest = current
    const move = (moveEvent: globalThis.PointerEvent) => {
      latest = {
        id: formation.id,
        x: round(start.node.x + (moveEvent.clientX - start.x) / view.scale),
        y: round(start.node.y + (moveEvent.clientY - start.y) / view.scale),
      }
      setLayout(previous => previous ? {
        ...previous,
        nodes: upsertNode(previous.nodes || [], latest),
      } : previous)
    }
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      setDraggingNodeId(null)
      saveNodeLayout(latest, current)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }, [board, layoutByNode, saveNodeLayout, view.scale])

  const zoomLevel = `${Math.round(view.scale * 100)}%`
  const boardMenuItems: ContextMenuItem[] = [
    { label: 'Mission', action: () => void createMission() },
    { label: 'Solo formation', action: () => void createFormation('solo') },
    { label: 'Peer formation', action: () => void createFormation('peer') },
    { label: 'Flow formation', action: () => void createFormation('flow') },
    { label: 'Orchestrated formation', action: () => void createFormation('orchestrated') },
    { label: 'Gate', action: () => void createGate() },
    { label: 'Plan template', disabled: true },
  ]

  const formationMenuItems = (formation: FormationNode): ContextMenuItem[] => [
    { label: 'Run formation', disabled: true },
    { label: 'Rename', disabled: true },
    ...(formation.type !== 'solo' ? [{ label: formation.type === 'flow' ? 'Add step' : 'Add slot', disabled: true }] : []),
    { label: 'Add input', action: () => void addPort(formation, 'input') },
    { label: 'Add output', action: () => void addPort(formation, 'output') },
    { label: formation.verification ? 'Configure verification' : 'Add verification', disabled: true },
    { label: 'Clear output', disabled: true },
    { label: 'Set input', disabled: true },
    { label: 'Duplicate', disabled: true },
    { label: 'Delete formation', destructive: true, disabled: true },
  ]

  const slotMenuItems = (formation: FormationNode, slot: FormationSlot): ContextMenuItem[] => [
    ...(agents.length > 0
      ? agents.map(agent => ({
        label: `Assign ${agent.displayName || agent.id}`,
        action: () => void assignSlot(formation, slot, `${agent.id}|${agent.harnessDefault || ''}`),
      }))
      : [{ label: 'Assign agent', disabled: true }]),
    ...(slot.agentId ? [
      { label: 'Open terminal', action: () => openTerminal(formation, slot) },
      { label: 'Unassign', action: () => void assignSlot(formation, slot, '') },
    ] : []),
    ...(formation.type === 'orchestrated' && !slot.controller ? [{ label: 'Make controller', action: () => void makeController(formation, slot) }] : []),
    ...(formation.type !== 'solo' ? [{ label: 'Add slot/step after', disabled: true }, { label: 'Remove slot', destructive: true, disabled: true }] : []),
  ]

  const gateMenuItems: ContextMenuItem[] = [
    { label: 'Configure gate' },
    { label: 'Duplicate gate', disabled: true },
    { label: 'Delete gate', destructive: true, disabled: true },
  ]

  const wireMenuItems = (connection: BoardConnection): ContextMenuItem[] => [
    { label: 'Remove connection', destructive: true, action: () => void removeWire(connection) },
    { label: 'Reset routing', action: () => void resetWireRoute(connection) },
  ]

  const judgeSocketMenuItems = (gate: GateNode): ContextMenuItem[] => {
    const chainCandidates = (board?.formations || []).slice(0, 3)
    return [
      { label: 'New judge formation', action: () => void createJudgeFormation(gate) },
      ...((board?.formations || []).map(formation => ({
        label: `Use ${formation.title}`,
        action: () => void setGateJudgeChain(gate, [formation.id]),
      }))),
      ...(chainCandidates.length >= 3 ? [{
        label: `Use chain: ${chainCandidates.map(formation => formation.title).join(', ')}`,
        action: () => void setGateJudgeChain(gate, chainCandidates.map(formation => formation.id)),
      }] : []),
      { label: 'Detach judge', action: () => void detachGateJudge(gate) },
    ]
  }

  const missionMenuItems: ContextMenuItem[] = [
    { label: 'Start mission', disabled: true },
    { label: 'Open panel' },
    { label: 'Rename mission', disabled: true },
    { label: 'Delete mission', destructive: true, disabled: true },
  ]

  return (
    <section aria-label="Formations" className="formations-view" data-testid="formations-view">
      <div className="formations-toolbar">
        <div className="formations-board-picker">
          <label>
            Board
            <select value={selectedSlug} onChange={event => loadBoard(event.target.value)}>
              {boards.map(summary => (
                <option key={summary.id} value={summary.slug}>{summary.title}</option>
              ))}
            </select>
          </label>
          {board && <span className="formations-rev">rev {board.rev}</span>}
        </div>
        <form className="formations-create" onSubmit={event => event.preventDefault()}>
          <input
            aria-label="Formation title"
            value={title}
            onChange={event => setTitle(event.target.value)}
            placeholder="New formation"
          />
          {(['solo', 'peer', 'flow', 'orchestrated'] as const).map(type => (
            <button key={type} type="button" onClick={() => createFormation(type)}>
              {typeLabels[type]}
            </button>
          ))}
          <button type="button" onClick={createGate}>Gate</button>
          <input
            aria-label="Mission title"
            value={missionTitle}
            onChange={event => setMissionTitle(event.target.value)}
            placeholder="Mission"
          />
          <input
            aria-label="Mission goal"
            value={missionGoal}
            onChange={event => setMissionGoal(event.target.value)}
            placeholder="Goal"
          />
          <input
            aria-label="Mission bead"
            value={missionBead}
            onChange={event => setMissionBead(event.target.value)}
            placeholder="home-..."
          />
          <button type="button" onClick={createMission}>Mission</button>
        </form>
        <div className="formations-zoom-controls">
          <button type="button" aria-label="Zoom out" onClick={() => zoomBy(1 / 1.2)}><Minus size={15} /></button>
          <span data-testid="formation-zoom-level">{zoomLevel}</span>
          <button type="button" aria-label="Zoom in" onClick={() => zoomBy(1.2)}><Plus size={15} /></button>
          <button type="button" onClick={fit}><LocateFixed size={15} /> FIT</button>
        </div>
      </div>

      {error && <div className="formations-error" role="alert">{error}</div>}

      {activeRun && (
        <div className="formations-run-panel" aria-label="Run status">
          <div className="formations-run-summary" data-testid="formation-run-status">
            <strong>{activeRun.runId}</strong>
            <span>{activeRun.status}</span>
            <span>{activeRun.eventCount} events</span>
          </div>
          {!activeRun.final && (
            <div className="formations-run-actions">
              {activeRun.resumeAllowed && <button type="button" onClick={resumeRun}>Resume run</button>}
              <button type="button" onClick={abortRun}>Abort run</button>
            </div>
          )}
          <ol className="formations-run-events" aria-label="Run timeline">
            {runEvents.map(event => (
              <li key={`${event.runId}-${event.seq}`}>
                <span>{event.seq}</span>
                <strong>{event.type}</strong>
                <span>{event.nodeId || event.gateId || '-'}</span>
                {runEventText(event) && <span>{runEventText(event)}</span>}
                {runEventReportRef(event) && <span>{runEventReportRef(event)}</span>}
                {event.type === 'human_input_requested' && event.gateId && !activeRun.final && (
                  <span className="formations-gate-actions">
                    <button
                      type="button"
                      aria-label={`Approve gate ${event.gateId}`}
                      onClick={() => recordHumanGateVerdict(event.gateId || '', 'pass')}
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      aria-label={`Reject gate ${event.gateId}`}
                      onClick={() => recordHumanGateVerdict(event.gateId || '', 'fail')}
                    >
                      Reject
                    </button>
                  </span>
                )}
              </li>
            ))}
          </ol>
        </div>
      )}

      <div
        ref={canvasRef}
        className="formations-canvas"
        data-testid="formations-canvas"
        data-pan-x={String(round(view.x))}
        data-pan-y={String(round(view.y))}
        onPointerDown={startPan}
        onContextMenu={event => openContextMenu(event, 'Board actions', boardMenuItems)}
        onWheel={event => {
          event.preventDefault()
          const rect = event.currentTarget.getBoundingClientRect()
          zoomBy(event.deltaY < 0 ? 1.12 : 1 / 1.12, {
            x: event.clientX - rect.left,
            y: event.clientY - rect.top,
          })
        }}
      >
        <div
          className="formations-world"
          data-testid="formations-world"
          style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})` }}
        >
          <svg className="formations-wires" aria-hidden="true">
            {connectionRoutes.map(item => (
              <path
                key={item.connection.id}
                className={`formation-wire ${layoutEdgeByID.get(item.connection.id)?.lane === 'manual' ? 'manual' : ''}`}
                data-testid={`formation-wire-${item.connection.id}`}
                data-from={item.fromId}
                data-to={item.toId}
                d={visibleWirePath(item.route.path)}
                onPointerDown={event => {
                  if (event.button === 0) {
                    event.preventDefault()
                    event.stopPropagation()
                    const up = () => {
                      window.removeEventListener('pointerup', up)
                      void handRouteWire(item.connection)
                    }
                    window.addEventListener('pointerup', up)
                  }
                }}
                onContextMenu={event => openContextMenu(event, 'Connection actions', wireMenuItems(item.connection))}
              />
            ))}
          </svg>
          {positionedFormations.map(({ formation, position }) => {
            const briefDraft = briefDrafts[formation.id] || {
              goal: formation.brief?.goal || '',
              beadId: formation.brief?.beadId || '',
              files: (formation.brief?.files || []).join(', '),
              links: (formation.brief?.links || []).join(', '),
            }
            const verificationDraft = verificationDrafts[formation.id] || {
              criterion: formation.verification?.criterion || '',
              onFail: formation.verification?.onFail || 'block',
            }
            return (
              <article
                key={formation.id}
                className={`formation-card formation-card-${formation.type}${escalatingNodeIds.has(formation.id) ? ' formation-card-escalating' : ''}`}
                data-testid={`formation-node-${formation.id}`}
                data-formation-id={formation.id}
                data-formation-type={formation.type}
                style={{ left: position.x, top: position.y }}
                onPointerDown={event => moveNode(formation, event)}
                onContextMenu={event => openContextMenu(event, 'Formation actions', formationMenuItems(formation))}
              >
                <header>
                  <strong>{formation.title}</strong>
                  <span>{typeLabels[formation.type]}</span>
                </header>
                <div className="formation-card-body">
                  {formation.type === 'peer' && <span className="formation-type-note">peers, no hierarchy</span>}
                  {formation.type === 'flow' && <span className="formation-type-note">ordered steps</span>}
                  {formation.type === 'orchestrated' && <span className="formation-type-note">one controller</span>}
                  <div className="formation-slots">
                    {formation.slots.map(slot => (
                      <div
                        key={slot.id}
                        className={slot.controller ? 'formation-slot controller' : 'formation-slot'}
                        data-testid={`formation-slot-${slot.id}`}
                        onPointerDown={stopControlPointer}
                        onContextMenu={event => openContextMenu(event, 'Slot actions', slotMenuItems(formation, slot))}
                      >
                        <span>{slot.label}</span>
                        <select
                          aria-label={`Agent for ${slot.label}`}
                          value={slotAgentValue(slot, agents)}
                          onChange={event => assignSlot(formation, slot, event.target.value)}
                        >
                          <option value="">Unassigned</option>
                          {slot.agentId && !agents.some(agent => agent.id === slot.agentId) && (
                            <option value={`${slot.agentId}|${slot.harness || ''}`}>
                              {slot.agentId}
                            </option>
                          )}
                          {agents.map(agent => (
                            <option key={agent.id} value={`${agent.id}|${agent.harnessDefault || ''}`}>
                              {agent.displayName || agent.id}
                            </option>
                          ))}
                        </select>
                        {formation.type === 'orchestrated' && !slot.controller && (
                          <button type="button" aria-label={`Make ${slot.label} controller`} onClick={() => makeController(formation, slot)}>
                            Controller
                          </button>
                        )}
                      </div>
                    ))}
                  </div>
                  <div className="formation-port-editor" onPointerDown={stopControlPointer}>
                    <div className="formation-port-list">
                      {(formation.inputs || []).map(input => {
                        const endpoint = `${formation.id}:${input.id}`
                        const incoming = (board?.connections || []).find(connection => connection.to === endpoint)
                        return (
                          <button
                            key={input.id}
                            type="button"
                            className={`formation-port formation-port-input${incoming ? ' connected' : ''}`}
                            data-testid={`formation-input-${formation.id}-${input.id}`}
                            data-formation-endpoint={endpoint}
                            aria-label={`${formation.title} input ${input.label}`}
                            onPointerDown={event => {
                              event.stopPropagation()
                              if (incoming) beginWireDrag(event, { kind: 'reconnect-target', connection: incoming })
                            }}
                            onMouseDown={event => {
                              event.stopPropagation()
                              if (incoming) beginWireDrag(event, { kind: 'reconnect-target', connection: incoming })
                            }}
                          >
                            {input.label}
                          </button>
                        )
                      })}
                      <button type="button" aria-label={`Add input to ${formation.title}`} onClick={() => addPort(formation, 'input')}>
                        Add input
                      </button>
                    </div>
                    <div className="formation-port-list">
                      {(formation.outputs || []).map(output => (
                        <label key={output.id}>
                          <button
                            type="button"
                            className="formation-port formation-port-output"
                            data-testid={`formation-output-${formation.id}-${output.id}`}
                            aria-label={`${formation.title} output ${output.label}`}
                            onPointerDown={event => beginWireDrag(event, { kind: 'new', from: `${formation.id}:${output.id}` })}
                            onMouseDown={event => beginWireDrag(event, { kind: 'new', from: `${formation.id}:${output.id}` })}
                          >
                            {output.label}
                          </button>
                          <select
                            aria-label={`Wire ${output.label} from ${formation.title}`}
                            value=""
                            onChange={event => wireConnection(formation, output, event.target.value)}
                          >
                            <option value="">Wire to...</option>
                            {inputPortOptions
                              .filter(option => endpointNodeId(option.endpoint) !== formation.id)
                              .map(option => (
                                <option key={option.endpoint} value={option.endpoint}>{option.label}</option>
                              ))}
                          </select>
                        </label>
                      ))}
                      <button type="button" aria-label={`Add output to ${formation.title}`} onClick={() => addPort(formation, 'output')}>
                        Add output
                      </button>
                    </div>
                  </div>
                  <div className="formation-brief-editor" onPointerDown={stopControlPointer}>
                    <input
                      aria-label={`Goal for ${formation.title}`}
                      value={briefDraft.goal}
                      onChange={event => updateBriefDraft(formation, { goal: event.target.value })}
                      placeholder="Goal"
                    />
                    <input
                      aria-label={`Bead for ${formation.title}`}
                      value={briefDraft.beadId}
                      onChange={event => updateBriefDraft(formation, { beadId: event.target.value })}
                      placeholder="home-..."
                    />
                    <input
                      aria-label={`Files for ${formation.title}`}
                      value={briefDraft.files}
                      onChange={event => updateBriefDraft(formation, { files: event.target.value })}
                      placeholder="files"
                    />
                    <input
                      aria-label={`Links for ${formation.title}`}
                      value={briefDraft.links}
                      onChange={event => updateBriefDraft(formation, { links: event.target.value })}
                      placeholder="links"
                    />
                    <button type="button" aria-label={`Save brief for ${formation.title}`} onClick={() => saveBrief(formation)}>
                      Save brief
                    </button>
                  </div>
                  <div className="formation-verification-editor" onPointerDown={stopControlPointer}>
                    <input
                      aria-label={`Verification criterion for ${formation.title}`}
                      value={verificationDraft.criterion}
                      onChange={event => updateVerificationDraft(formation, { criterion: event.target.value })}
                      placeholder="Verification"
                    />
                    <select
                      aria-label={`Verification failure for ${formation.title}`}
                      value={verificationDraft.onFail}
                      onChange={event => updateVerificationDraft(formation, { onFail: event.target.value as VerificationDraft['onFail'] })}
                    >
                      <option value="block">Block</option>
                      <option value="pushback">Pushback</option>
                    </select>
                    <button type="button" aria-label={`Save verification for ${formation.title}`} onClick={() => saveVerification(formation)}>
                      Save verification
                    </button>
                  </div>
                </div>
              </article>
            )
          })}
          {positionedGates.map(({ gate, position }) => (
            <article
              key={gate.id}
              className="formation-card gate-card"
              data-testid={`gate-node-${gate.id}`}
              style={{ left: position.x, top: position.y }}
              onContextMenu={event => openContextMenu(event, 'Gate actions', gateMenuItems)}
            >
              <header>
                <strong>{gate.title}</strong>
                <span>{gate.kinds.join(' · ')}</span>
              </header>
              <div className="formation-card-body">
                <span className="formation-type-note">{gate.criterion || 'Gate criterion unset'}</span>
                <div className="formation-port-editor" onPointerDown={stopControlPointer}>
                  <div className="gate-port-row">
                    <button
                      type="button"
                      className="formation-port formation-port-input"
                      data-testid={`formation-input-${gate.id}-in`}
                      data-formation-endpoint={`${gate.id}:in`}
                      aria-label={`${gate.title} input`}
                    >
                      in
                    </button>
                    <button
                      type="button"
                      className="formation-port gate-port-pass"
                      data-testid={`gate-output-${gate.id}-pass`}
                      aria-label={`${gate.title} pass output`}
                      onPointerDown={event => beginWireDrag(event, { kind: 'new', from: `${gate.id}:pass` })}
                      onMouseDown={event => beginWireDrag(event, { kind: 'new', from: `${gate.id}:pass` })}
                    >
                      pass
                    </button>
                    <button
                      type="button"
                      className="formation-port gate-port-fail"
                      data-testid={`gate-output-${gate.id}-fail`}
                      aria-label={`${gate.title} fail output`}
                      onPointerDown={event => beginWireDrag(event, { kind: 'new', from: `${gate.id}:fail` })}
                      onMouseDown={event => beginWireDrag(event, { kind: 'new', from: `${gate.id}:fail` })}
                    >
                      fail
                    </button>
                    <button
                      type="button"
                      className="formation-port gate-port-judge"
                      data-testid={`gate-judge-socket-${gate.id}`}
                      data-gate-judge-socket={gate.id}
                      aria-label={`${gate.title} judge socket`}
                      onPointerDown={event => beginJudgeSocketPointer(event, gate)}
                      onMouseDown={event => beginJudgeSocketPointer(event, gate)}
                      onClick={event => openContextMenu(event, 'Judge socket actions', judgeSocketMenuItems(gate))}
                    >
                      judge
                    </button>
                  </div>
                  <label>
                    <span>Judge</span>
                    <select
                      aria-label={`Judge chain for ${gate.title}`}
                      value=""
                      onChange={event => setGateJudge(gate, event.target.value)}
                    >
                      <option value="">Attach judge...</option>
                      {(board?.formations || []).map(formation => (
                        <option key={formation.id} value={formation.id}>{formation.title}</option>
                      ))}
                    </select>
                  </label>
                </div>
              </div>
            </article>
          ))}
          {positionedMissions.map(({ mission, position }) => (
            <article
              key={mission.id}
              className="formation-card mission-card"
              data-testid={`mission-node-${mission.id}`}
              style={{ left: position.x, top: position.y }}
              onContextMenu={event => openContextMenu(event, 'Mission actions', missionMenuItems)}
            >
              <header>
                <strong>{mission.title}</strong>
                <span>Mission</span>
              </header>
              <div className="formation-card-body">
                <span className="formation-type-note">{mission.goal}</span>
                <span className="formation-type-note">{mission.beadId}</span>
                <span className="formation-type-note">out</span>
                <button
                  type="button"
                  aria-label={`Start ${mission.title}`}
                  onPointerDown={stopControlPointer}
                  onClick={() => startMission(mission)}
                >
                  Start
                </button>
              </div>
            </article>
          ))}
          {[...terminals].sort((a, b) => a.focusedAt - b.focusedAt).map((terminal, index) => {
            const live = terminal.liveness === 'live'
            return (
              <section
                key={terminal.agentId}
                className="formation-terminal-popup"
                data-testid={`formation-terminal-${terminal.agentId}`}
                data-world-scale={String(view.scale)}
                data-dragged={terminal.dragged ? 'true' : undefined}
                data-resized={terminal.resized ? 'true' : undefined}
                style={{
                  left: terminal.x,
                  top: terminal.y,
                  width: terminal.width,
                  height: terminal.height,
                  zIndex: 80 + index,
                }}
                onPointerDown={event => event.stopPropagation()}
              >
                <header
                  data-testid={`formation-terminal-${terminal.agentId}-header`}
                  onPointerDown={event => moveTerminal(terminal.agentId, event)}
                  onMouseDown={event => moveTerminal(terminal.agentId, event)}
                >
                  <strong>{terminal.title}</strong>
                  <span>{terminal.liveness}</span>
                  <button
                    type="button"
                    aria-label={`Close terminal ${terminal.title}`}
                    onPointerDown={stopControlPointer}
                    onClick={() => closeTerminal(terminal.agentId)}
                  >
                    Close
                  </button>
                </header>
                {live ? (
                  <iframe
                    title={`${terminal.title} terminal`}
                    src={`/terminal/?agent=${encodeURIComponent(terminal.agentId)}&watch=1`}
                  />
                ) : (
                  <div className="formation-terminal-dead">
                    session is not live
                  </div>
                )}
                <button
                  type="button"
                  aria-label={`Resize terminal ${terminal.title}`}
                  className="formation-terminal-resize"
                  data-testid={`formation-terminal-${terminal.agentId}-resize`}
                  onPointerDown={event => resizeTerminal(terminal.agentId, event)}
                  onMouseDown={event => resizeTerminal(terminal.agentId, event)}
                />
              </section>
            )
          })}
        </div>
      </div>
      {contextMenu && (
        <div
          className="formations-context-menu"
          role="menu"
          aria-label={contextMenu.label}
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onPointerDown={event => event.stopPropagation()}
        >
          {contextMenu.items.map(item => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              aria-disabled={item.disabled ? 'true' : undefined}
              className={item.destructive ? 'destructive' : undefined}
              onClick={() => {
                if (item.disabled) return
                closeContextMenu()
                item.action?.()
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </section>
  )
}

function upsertNode(nodes: LayoutNode[], next: LayoutNode): LayoutNode[] {
  const index = nodes.findIndex(node => node.id === next.id)
  if (index < 0) return [...nodes, next]
  return nodes.map(node => node.id === next.id ? next : node)
}

function undoBoardPatch(action: BoardUndoAction): Record<string, unknown> {
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
    case 'setVerification':
      if (!action.verification) {
        return { removeVerification: { formationId: action.formationId } }
      }
      return {
        setVerification: {
          formationId: action.formationId,
          kinds: action.verification.kinds || ['code'],
          criterion: action.verification.criterion || '',
          onFail: action.verification.onFail,
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

function findAddedByID<T extends { id: string }>(before: T[], after: T[]): T | undefined {
  const known = new Set(before.map(item => item.id))
  return after.find(item => !known.has(item.id))
}

function findAddedPort(before: BoardDocument | null, after: BoardDocument | null, formationId: string, direction: 'input' | 'output'): FormationPort | undefined {
  const beforeFormation = before?.formations.find(formation => formation.id === formationId)
  const afterFormation = after?.formations.find(formation => formation.id === formationId)
  if (!afterFormation) return undefined
  const key = direction === 'input' ? 'inputs' : 'outputs'
  return findAddedByID(beforeFormation?.[key] || [], afterFormation[key] || [])
}

function upsertRunEvent(events: RunEvent[], next: RunEvent): RunEvent[] {
  if (!next.runId || !next.seq) return events
  const existing = events.findIndex(event => event.seq === next.seq && event.runId === next.runId)
  const merged = existing >= 0
    ? events.map((event, index) => index === existing ? next : event)
    : [...events, next]
  return merged.sort((a, b) => a.seq - b.seq)
}

function statusFromRunEvent(event: RunEvent): string {
  switch (event.type) {
    case 'run_started':
    case 'run_resumed':
      return 'running'
    case 'run_blocked':
      return 'blocked'
    case 'run_canceled':
      return 'canceled'
    case 'run_failed':
      return 'failed'
    case 'run_succeeded':
      return 'succeeded'
    default:
      return ''
  }
}

function runEventResumeAllowed(event: RunEvent, fallback: boolean): boolean {
  if (event.type === 'run_blocked') return event.data?.resumeAllowed === true
  if (event.type === 'run_started' || event.type === 'run_resumed') return false
  if (event.type === 'run_canceled' || event.type === 'run_failed' || event.type === 'run_succeeded') return false
  return fallback
}

function runEventText(event: RunEvent): string {
  const data = event.data || {}
  if (typeof data.text === 'string') return data.text
  if (typeof data.reason === 'string') return data.reason
  if (typeof data.prompt === 'string') return data.prompt
  if (typeof data.error === 'string') return data.error
  return ''
}

function runStatusFromResponse(data: RunStatusProjection | RunStatusResult): RunStatusProjection {
  const nested = (data as RunStatusResult).status
  return typeof nested === 'object' && nested !== null ? nested : data as RunStatusProjection
}

function runEventReportRef(event: RunEvent): string {
  const reportRef = event.data?.reportRef
  return typeof reportRef === 'string' ? reportRef : ''
}

function cloneBrief(brief: FormationBrief): FormationBrief {
  return {
    goal: brief.goal || '',
    beadId: brief.beadId || '',
    files: [...(brief.files || [])],
    links: [...(brief.links || [])],
  }
}

function cloneVerification(verification: FormationVerification): FormationVerification {
  return {
    id: verification.id,
    kinds: [...(verification.kinds || [])],
    criterion: verification.criterion,
    onFail: verification.onFail,
  }
}

function slotAgentValue(slot: FormationSlot, agents: AgentProjection[]): string {
  if (!slot.agentId) return ''
  const harness = slot.harness || agents.find(agent => agent.id === slot.agentId)?.harnessDefault || ''
  return `${slot.agentId}|${harness}`
}

function splitAgentValue(value: string): [string, string] {
  if (!value) return ['', '']
  const [agentId, harness = ''] = value.split('|')
  return [agentId, harness]
}

function splitRefs(value: string): string[] {
  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean)
}

function stopControlPointer(event: PointerEvent<HTMLElement>) {
  event.stopPropagation()
}

function isTextEditingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false
  if (target.closest('input, textarea, select')) return true
  return Boolean(target.closest('[contenteditable="true"], [contenteditable=""]'))
}

function endpointNodeId(endpoint: string): string {
  return endpoint.split(':', 1)[0]
}

function formationDropTarget(target: Element | null, point: { x: number; y: number }): {
  endpoint: string
  judgeGateID: string
  formationID: string
} {
  const closestEndpoint = target?.closest<HTMLElement>('[data-formation-endpoint]')?.dataset.formationEndpoint || ''
  const closestJudge = target?.closest<HTMLElement>('[data-gate-judge-socket]')?.dataset.gateJudgeSocket || ''
  const closestFormation = target?.closest<HTMLElement>('[data-formation-id]')?.dataset.formationId || ''
  if (closestEndpoint || closestJudge || closestFormation) {
    return { endpoint: closestEndpoint, judgeGateID: closestJudge, formationID: closestFormation }
  }

  const hit = <T extends HTMLElement>(selector: string, data: (element: T) => string) => {
    const elements = Array.from(document.querySelectorAll<T>(selector))
    const direct = elements.find(element => {
      const rect = element.getBoundingClientRect()
      return point.x >= rect.left && point.x <= rect.right && point.y >= rect.top && point.y <= rect.bottom
    })
    return direct ? data(direct) : ''
  }

  const endpoint = hit<HTMLElement>('[data-formation-endpoint]', element => element.dataset.formationEndpoint || '')
  if (endpoint) return { endpoint, judgeGateID: '', formationID: '' }
  const judgeGateID = hit<HTMLElement>('[data-gate-judge-socket]', element => element.dataset.gateJudgeSocket || '')
  if (judgeGateID) return { endpoint: '', judgeGateID, formationID: '' }
  const formationID = hit<HTMLElement>('[data-formation-id]', element => element.dataset.formationId || '')
  return { endpoint: '', judgeGateID: '', formationID }
}

function visibleWirePath(path: string): string {
  const match = path.match(/^M(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?) L(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$/)
  if (!match || match[2] !== match[4]) return path
  const y = Number(match[4])
  if (!Number.isFinite(y)) return path
  return `${path} L${match[3]},${y + 1}`
}

function activeRunStorageKey(slug: string): string {
  return `chrote-formations-active-run-${slug}`
}

function clampScale(scale: number, max = 2.2): number {
  return Math.max(0.4, Math.min(max, Number(scale.toFixed(2))))
}

function zoomTransform(current: ViewTransform, factor: number, cursor?: { x: number; y: number }): ViewTransform {
  const scale = clampScale(current.scale * factor)
  if (!cursor) return { ...current, scale }
  const worldX = (cursor.x - current.x) / current.scale
  const worldY = (cursor.y - current.y) / current.scale
  return {
    x: round(cursor.x - worldX * scale),
    y: round(cursor.y - worldY * scale),
    scale,
  }
}
