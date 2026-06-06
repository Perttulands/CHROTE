/* FormationsCockpit — reference-faithful spatial cockpit for CHROTE Formations.
 *
 * Ported from the D7 prototype (Perttus_vision_for_agent_orchestration/03-formations.{html,js}):
 * left Agent Roster (drag an agent into a slot to staff it), an infinite pan/zoom
 * world canvas, compact typed formation cards with circular slot-spheres, mission and
 * gate cards, and SVG wires. Reuses the sound file/model/API plumbing
 * (formationsApi/Types/Canvas/RunState) — only the broken form-style view layer is replaced.
 *
 * This pass implements: roster + drag-to-staff, typed cards with slot spheres,
 * mission/gate cards, wires, port-drag wiring, pan/zoom/fit, card drag-to-reposition,
 * and run buttons with honest per-node run-state projection. Wire reconnect/delete,
 * context menus, on-canvas editors/terminals, and undo are tracked for follow passes
 * (bead home-f7as).
 */
import { MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  abortRunRequest,
  fetchAgents,
  fetchBoardChanged,
  fetchBoardDocument,
  fetchBoardLayout,
  fetchBoardSummaries,
  fetchRunEvents,
  fetchRunStatus,
  patchBoardDocument,
  patchBoardLayout,
  recordGateVerdict,
  resumeRunRequest,
  startRun,
} from './formationsApi'
import {
  activeRunStorageKey,
  runStatusFromResponse,
  upsertRunEvent,
} from './formationsRunState'
import { clampScale, zoomTransform } from './formationsCanvas'
import { findAddedByID, upsertNode } from './formationsBoardModel'
import {
  applyStarterBoardPatch,
  createStarterBoard,
  createStarterLayout,
  isStarterBoard,
  starterBoardSummary,
  withStarterLayoutRev,
} from './formationsStarterBoard'
import type {
  AgentProjection,
  BoardConnection,
  BoardDocument,
  BoardSummary,
  FormationBrief,
  FormationNode,
  FormationSlot,
  GateNode,
  LayoutDocument,
  LayoutNode,
  MissionNode,
  RunEvent,
  RunStatusProjection,
  ViewTransform,
} from './formationsTypes'

const TYPE_TAG: Record<FormationNode['type'], string> = {
  solo: 'Do the thing.',
  peer: 'Work together · challenge · synthesize.',
  flow: 'A, then B, then C.',
  orchestrated: 'One controller decides what happens next.',
}

const GATE_SVG = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7">
    <path d="M4 21V10a8 8 0 0116 0v11" />
    <path d="M3 21h18M8 21V9M12 21V8M16 21V9" />
  </svg>
)
const PLAY_SVG = (
  <svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 4l14 8-14 8z" /></svg>
)

function hashHue(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i += 1) h = (h * 31 + id.charCodeAt(i)) % 360
  return h
}
function agentColor(id: string): string {
  return `radial-gradient(hsl(${hashHue(id)} 60% 62%), hsl(${hashHue(id)} 55% 34%))`
}
function initials(id: string): string {
  const cleaned = id.replace(/[^a-zA-Z0-9]/g, '')
  return (cleaned.slice(0, 2) || '?').toUpperCase()
}
function agentRole(agent: AgentProjection): string {
  return agent.harnessDefault || (agent.unbound ? 'unbound' : 'agent')
}
function agentState(agent: AgentProjection): 'on' | 'idle' {
  return agent.liveness === 'live' || agent.assignable ? 'on' : 'idle'
}

type NodeRunState = '' | 'running' | 'done' | 'blocked' | 'waiting' | 'failed'

/** Project honest per-node run state from the ledger events (mirrors the engine vocabulary). */
function projectNodeStates(events: RunEvent[], activeRun: RunStatusProjection | null): Map<string, NodeRunState> {
  const map = new Map<string, NodeRunState>()
  for (const event of events) {
    const nodeId = event.nodeId || event.gateId
    if (!nodeId) continue
    switch (event.type) {
      case 'node_started':
      case 'slot_dispatch':
      case 'gate_evaluating':
        map.set(nodeId, 'running')
        break
      case 'node_waiting':
        map.set(nodeId, 'waiting')
        break
      case 'node_output': {
        const status = typeof event.data?.status === 'string' ? event.data.status : 'done'
        map.set(nodeId, status === 'blocked' ? 'blocked' : 'done')
        break
      }
      case 'gate_verdict': {
        const verdict = typeof event.data?.verdict === 'string' ? event.data.verdict : ''
        map.set(nodeId, verdict === 'fail' ? 'failed' : 'done')
        break
      }
      case 'run_blocked':
        map.set(nodeId, 'blocked')
        break
      case 'run_failed':
        map.set(nodeId, 'failed')
        break
      default:
        break
    }
  }
  if (activeRun && !activeRun.final && activeRun.status === 'running') {
    // leave node states as projected
  }
  return map
}

function openHumanGateId(events: RunEvent[]): string {
  let openGateId = ''
  for (const event of [...events].sort((a, b) => a.seq - b.seq)) {
    if (event.type === 'human_input_requested' && event.gateId) {
      openGateId = event.gateId
      continue
    }
    if ((event.type === 'human_verdict_recorded' || event.type === 'gate_verdict') && event.gateId === openGateId) {
      openGateId = ''
    }
    if (event.type === 'run_succeeded' || event.type === 'run_failed' || event.type === 'run_canceled') {
      openGateId = ''
    }
  }
  return openGateId
}

type DragStaff = { agentId: string; harness: string; fromSlot?: { formationId: string; slotId: string } }
type DragNode = { id: string; pointerId: number; startX: number; startY: number; originX: number; originY: number; moved: boolean }
type WirePath = { id: string; d: string; kind: 'wire' | 'pass' | 'fail' | 'judge'; flowing: boolean }
type WireDrag = { kind: 'new'; from: string; wireKind: WirePath['kind'] } | { kind: 'reconnect-target'; connection: BoardConnection }
type LayoutItem = { id: string; index: number; kind: 'mission' | 'gate' | FormationNode['type']; slots?: number }
type LayoutBox = { id: string; x: number; y: number; w: number; h: number }
type MenuItem = { label: string; action?: () => void; destructive?: boolean; disabled?: boolean }
type MenuState = { label: string; x: number; y: number; items: MenuItem[] }
type BriefEditorState = {
  formationId: string
  title: string
  goal: string
  beadId: string
  files: string
  links: string
}
type CockpitUndo =
  | { kind: 'clearBrief'; formationId: string }
  | { kind: 'setBrief'; formationId: string; brief: FormationBrief }
  | { kind: 'wireConnection'; from: string; to: string }
  | { kind: 'unwireConnection'; from: string; to: string }
  | { kind: 'rewireConnection'; from: string; previousTo: string; to: string }

function fallbackNodePosition(index: number): { x: number; y: number } {
  return { x: 140 + index * 300, y: 160 + (index % 2) * 200 }
}

function estimatedNodeBox(item: LayoutItem): { w: number; h: number } {
  if (item.kind === 'mission') return { w: 236, h: 136 }
  if (item.kind === 'gate') return { w: 300, h: 124 }
  if (item.kind === 'flow') return { w: Math.min(560, Math.max(300, 172 + (item.slots || 1) * 132)), h: 270 }
  if (item.kind === 'peer') return { w: 330, h: 286 }
  if (item.kind === 'orchestrated') return { w: 320, h: 372 }
  return { w: 300, h: 270 }
}

function overlaps(a: LayoutBox, b: LayoutBox, gap = 36): boolean {
  return a.x < b.x + b.w + gap &&
    a.x + a.w + gap > b.x &&
    a.y < b.y + b.h + gap &&
    a.y + a.h + gap > b.y
}

function splitList(value: string): string[] {
  return value.split(',').map(item => item.trim()).filter(Boolean)
}

function isTextEditingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable
}

function findInputPortAt(clientX: number, clientY: number): HTMLElement | null {
  const direct = (document.elementFromPoint(clientX, clientY) as HTMLElement | null)?.closest<HTMLElement>('[data-port-in]')
  if (direct) return direct
  const ports = Array.from(document.querySelectorAll<HTMLElement>('.fmx [data-port-in]'))
  let best: { element: HTMLElement; distance: number } | null = null
  for (const port of ports) {
    const rect = port.getBoundingClientRect()
    const margin = 18
    if (clientX < rect.left - margin || clientX > rect.right + margin || clientY < rect.top - margin || clientY > rect.bottom + margin) continue
    const cx = rect.left + rect.width / 2
    const cy = rect.top + rect.height / 2
    const distance = Math.hypot(clientX - cx, clientY - cy)
    if (!best || distance < best.distance) best = { element: port, distance }
  }
  return best?.element || null
}

function displayLayoutFor(board: BoardDocument, layoutByNode: Map<string, LayoutNode>): Map<string, LayoutNode> {
  const missions = board.missions || []
  const formations = board.formations || []
  const gates = board.gates || []
  const items: LayoutItem[] = [
    ...missions.map((node, index) => ({ id: node.id, index, kind: 'mission' as const })),
    ...formations.map((node, index) => ({ id: node.id, index: missions.length + index, kind: node.type, slots: node.slots.length })),
    ...gates.map((node, index) => ({ id: node.id, index: missions.length + formations.length + index, kind: 'gate' as const })),
  ]
  const placed: LayoutBox[] = []
  const out = new Map<string, LayoutNode>()
  for (const item of items) {
    const base = layoutByNode.get(item.id) || { id: item.id, ...fallbackNodePosition(item.index) }
    const size = estimatedNodeBox(item)
    let x = base.x
    let y = base.y
    for (let guard = 0; guard < 24; guard += 1) {
      const candidate = { id: item.id, x, y, ...size }
      const blocker = placed.find(prev => overlaps(candidate, prev))
      if (!blocker) break
      const right = blocker.x + blocker.w + 56
      if (right + size.w <= 1900) {
        x = right
      } else {
        x = Math.max(120, Math.min(x, blocker.x))
        y = blocker.y + blocker.h + 56
      }
    }
    out.set(item.id, { id: item.id, x, y })
    placed.push({ id: item.id, x, y, ...size })
  }
  return out
}

export default function FormationsCockpit() {
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [selectedSlug, setSelectedSlug] = useState('')
  const [board, setBoard] = useState<BoardDocument | null>(null)
  const [layout, setLayout] = useState<LayoutDocument | null>(null)
  const [agents, setAgents] = useState<AgentProjection[]>([])
  const [view, setView] = useState<ViewTransform>({ x: 40, y: 40, scale: 1 })
  const [error, setError] = useState('')
  const [activeRun, setActiveRun] = useState<RunStatusProjection | null>(null)
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
  const [ghost, setGhost] = useState<{ x: number; y: number; agentId: string } | null>(null)
  const [hoverSlot, setHoverSlot] = useState<string | null>(null)
  const [dragPos, setDragPos] = useState<{ id: string; x: number; y: number } | null>(null)
  const [wires, setWires] = useState<WirePath[]>([])
  const [tempWire, setTempWire] = useState<{ ax: number; ay: number; bx: number; by: number; kind: WirePath['kind'] } | null>(null)
  const [hoverPort, setHoverPort] = useState<string | null>(null)
  const [gateGhost, setGateGhost] = useState<{ x: number; y: number } | null>(null)
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [briefEditor, setBriefEditor] = useState<BriefEditorState | null>(null)

  const boardRef = useRef<BoardDocument | null>(null)
  const layoutRef = useRef<LayoutDocument | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const worldRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<ViewTransform>(view)
  const staffRef = useRef<DragStaff | null>(null)
  const dragNodeRef = useRef<DragNode | null>(null)
  const panRef = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null)
  const wireDragRef = useRef<WireDrag | null>(null)
  const gateDragRef = useRef<boolean>(false)
  const undoStack = useRef<CockpitUndo[]>([])
  const fittedBoardRef = useRef<string | null>(null)

  viewRef.current = view
  useEffect(() => { boardRef.current = board }, [board])
  useEffect(() => { layoutRef.current = layout }, [layout])

  // ----- data loading -----
  useEffect(() => {
    let cancelled = false
    fetchBoardSummaries()
      .then(list => {
        if (cancelled) return
        setBoards(list)
        if (list[0]) {
          setSelectedSlug(current => current || list[0].slug)
          return
        }
        const starterBoard = createStarterBoard()
        const starterLayout = createStarterLayout()
        boardRef.current = starterBoard
        layoutRef.current = starterLayout
        setBoards([starterBoardSummary(starterBoard)])
        setSelectedSlug(starterBoard.slug)
        setBoard(starterBoard)
        setLayout(starterLayout)
      })
      .catch(err => !cancelled && setError(err instanceof Error ? err.message : 'Failed to load boards'))
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!selectedSlug) return
    const currentBoard = boardRef.current
    if (currentBoard && isStarterBoard(currentBoard) && currentBoard.slug === selectedSlug) return
    let cancelled = false
    Promise.all([fetchBoardDocument(selectedSlug), fetchBoardLayout(selectedSlug)])
      .then(([nextBoard, nextLayout]) => {
        if (cancelled) return
        setBoard(nextBoard)
        setLayout(nextLayout)
        setError('')
      })
      .catch(err => !cancelled && setError(err instanceof Error ? err.message : 'Failed to load board'))
    return () => { cancelled = true }
  }, [selectedSlug])

  useEffect(() => {
    if (!selectedSlug || !board?.rev || isStarterBoard(board)) return
    let cancelled = false
    const checkChanges = async () => {
      try {
        const changed = await fetchBoardChanged(selectedSlug, board.rev)
        if (cancelled || !changed) return
        const [nextBoard, nextLayout] = await Promise.all([fetchBoardDocument(selectedSlug), fetchBoardLayout(selectedSlug)])
        if (cancelled) return
        setBoard(nextBoard)
        setLayout(nextLayout)
        setError('')
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to check board changes')
      }
    }
    void checkChanges()
    const timer = window.setInterval(() => { void checkChanges() }, 600)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [board?.rev, selectedSlug])

  useEffect(() => {
    let cancelled = false
    const load = () => fetchAgents().then(list => !cancelled && setAgents(list)).catch(() => undefined)
    load()
    const timer = window.setInterval(load, 8000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [])

  useEffect(() => {
    if (!selectedSlug || isStarterBoard(board)) return
    const runId = window.localStorage.getItem(activeRunStorageKey(selectedSlug))
    if (!runId || activeRun?.runId === runId) return
    let cancelled = false
    const restoreRun = async () => {
      try {
        const status = runStatusFromResponse(await fetchRunStatus(runId))
        if (cancelled) return
        try {
          const events = await fetchRunEvents(runId)
          if (!cancelled) {
            setRunEvents(events)
            setActiveRun(status)
          }
        } catch (err) {
          if (!cancelled) {
            setRunEvents([])
            setActiveRun(status)
            setError(err instanceof Error ? err.message : 'Failed to restore run events')
          }
        }
        if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to restore active run')
      }
    }
    void restoreRun()
    return () => { cancelled = true }
  }, [activeRun?.runId, selectedSlug])

  // ----- run polling -----
  useEffect(() => {
    if (!activeRun?.runId || activeRun.final) return
    let cancelled = false
    const tick = async () => {
      try {
        const status = runStatusFromResponse(await fetchRunStatus(activeRun.runId))
        if (cancelled) return
        setActiveRun(status)
        const events = await fetchRunEvents(activeRun.runId)
        if (cancelled) return
        setRunEvents(prev => events.reduce((acc, event) => upsertRunEvent(acc, event), prev))
        if (status.final && selectedSlug) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      } catch {
        /* transient */
      }
    }
    void tick()
    const timer = window.setInterval(tick, 1200)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [activeRun?.runId, activeRun?.final, selectedSlug])

  // ----- positioned nodes -----
  const layoutByNode = useMemo(() => {
    const map = new Map<string, LayoutNode>()
    layout?.nodes?.forEach(node => map.set(node.id, node))
    return map
  }, [layout])

  const displayLayoutByNode = useMemo(() => {
    if (!board) return layoutByNode
    return displayLayoutFor(board, layoutByNode)
  }, [board, layoutByNode])

  const positionOf = useCallback((id: string, index: number): { x: number; y: number } => {
    if (dragPos && dragPos.id === id) return { x: dragPos.x, y: dragPos.y }
    const node = displayLayoutByNode.get(id)
    if (node) return { x: node.x, y: node.y }
    return fallbackNodePosition(index)
  }, [displayLayoutByNode, dragPos])

  const nodeStates = useMemo(() => projectNodeStates(runEvents, activeRun), [runEvents, activeRun])

  // ----- wire geometry: measure rendered port centers in world coords -----
  // Reads board/layout/view STATE directly (not refs) so the measurement runs in the
  // same commit the cards are laid out in; reading refs here would lag a frame and drop wires.
  useLayoutEffect(() => {
    const world = worldRef.current
    if (!world || !board) { setWires([]); return }
    const worldRect = world.getBoundingClientRect()
    const scale = view.scale || 1
    const centerOf = (selector: string): { x: number; y: number } | null => {
      const el = world.querySelector<HTMLElement>(selector)
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: (r.left + r.width / 2 - worldRect.left) / scale, y: (r.top + r.height / 2 - worldRect.top) / scale }
    }
    const cssEscape = (value: string) => value.replace(/["\\]/g, '\\$&')
    const paths: WirePath[] = []
    for (const conn of board.connections || []) {
      const [fromNode, fromPort] = conn.from.split(':')
      const [toNode, toPort] = conn.to.split(':')
      const a = centerOf(`[data-port-out="${cssEscape(conn.from)}"]`)
      const b = centerOf(`[data-port-in="${cssEscape(conn.to)}"]`)
      if (!a || !b) continue
      const kind: WirePath['kind'] = fromPort === 'pass' ? 'pass' : fromPort === 'fail' ? 'fail' : (fromPort === 'judge' || toPort === 'judge') ? 'judge' : 'wire'
      const dx = Math.max(40, Math.abs(b.x - a.x) * 0.5)
      const d = `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`
      const flowing = nodeStates.get(fromNode) === 'running' || nodeStates.get(toNode) === 'running'
      paths.push({ id: conn.id, d, kind, flowing })
    }
    setWires(paths)
  }, [board, layout, view, dragPos, agents, nodeStates])

  // ----- mutations -----
  const patchBoard = useCallback(async (patch: Record<string, unknown>): Promise<{ board: BoardDocument; layout: LayoutDocument | null } | null> => {
    const current = boardRef.current
    if (!current) return null
    if (isStarterBoard(current)) {
      const result = applyStarterBoardPatch(current, layoutRef.current || createStarterLayout(), patch)
      boardRef.current = result.board
      setBoard(result.board)
      setBoards([starterBoardSummary(result.board)])
      if (result.layout) {
        layoutRef.current = result.layout
        setLayout(result.layout)
      }
      setError('')
      return { board: result.board, layout: result.layout ?? null }
    }
    try {
      const result = await patchBoardDocument(current.slug, current.etag, current.rev, patch)
      boardRef.current = result.board
      setBoard(result.board)
      if (result.layout) {
        layoutRef.current = result.layout
        setLayout(result.layout)
      }
      setError('')
      return { board: result.board, layout: result.layout ?? null }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
      return null
    }
  }, [])

  const patchLayoutEdge = useCallback(async (edgeId: string, lane: string) => {
    const currentBoard = boardRef.current
    const currentLayout = layoutRef.current
    if (!currentBoard || !currentLayout) return
    if (isStarterBoard(currentBoard)) {
      const next = withStarterLayoutRev({
        ...currentLayout,
        edges: [
          ...(currentLayout.edges || []).filter(edge => edge.id !== edgeId),
          { id: edgeId, lane },
        ],
      }, currentBoard.rev)
      layoutRef.current = next
      setLayout(next)
      setError('')
      return
    }
    try {
      const next = await patchBoardLayout(currentBoard.slug, currentLayout.etag, { edges: [{ id: edgeId, lane }] })
      layoutRef.current = next
      setLayout(next)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update wire routing')
    }
  }, [])

  const closeMenu = useCallback(() => setMenu(null), [])

  const openMenu = useCallback((event: ReactMouseEvent<Element> | ReactPointerEvent<Element>, label: string, items: MenuItem[]) => {
    event.preventDefault()
    event.stopPropagation()
    setMenu({ label, x: event.clientX, y: event.clientY, items })
  }, [])

  const openBriefEditor = useCallback((formation: FormationNode) => {
    setMenu(null)
    setBriefEditor({
      formationId: formation.id,
      title: formation.title,
      goal: formation.brief?.goal || '',
      beadId: formation.brief?.beadId || '',
      files: (formation.brief?.files || []).join(', '),
      links: (formation.brief?.links || []).join(', '),
    })
  }, [])

  const saveBriefEditor = useCallback(async () => {
    if (!briefEditor) return
    const currentFormation = boardRef.current?.formations.find(formation => formation.id === briefEditor.formationId)
    undoStack.current.push(currentFormation?.brief
      ? { kind: 'setBrief', formationId: briefEditor.formationId, brief: currentFormation.brief }
      : { kind: 'clearBrief', formationId: briefEditor.formationId })
    await patchBoard({
      setBrief: {
        formationId: briefEditor.formationId,
        goal: briefEditor.goal.trim(),
        beadId: briefEditor.beadId.trim(),
        files: splitList(briefEditor.files),
        links: splitList(briefEditor.links),
      },
    })
    setBriefEditor(null)
  }, [briefEditor, patchBoard])

  const performUndo = useCallback(async () => {
    const action = undoStack.current.pop()
    if (!action) return
    let patch: Record<string, unknown>
    switch (action.kind) {
      case 'clearBrief':
        patch = { clearBrief: { formationId: action.formationId } }
        break
      case 'setBrief':
        patch = {
          setBrief: {
            formationId: action.formationId,
            goal: action.brief.goal || '',
            beadId: action.brief.beadId || '',
            files: action.brief.files || [],
            links: action.brief.links || [],
          },
        }
        break
      case 'wireConnection':
        patch = { wireConnection: { from: action.from, to: action.to } }
        break
      case 'unwireConnection':
        patch = { unwireConnection: { from: action.from, to: action.to } }
        break
      case 'rewireConnection':
        patch = { rewireConnection: { from: action.from, previousTo: action.previousTo, to: action.to } }
        break
    }
    const result = await patchBoard(patch)
    if (!result) undoStack.current.push(action)
  }, [patchBoard])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.shiftKey || event.key.toLowerCase() !== 'z') return
      if (isTextEditingTarget(event.target)) return
      event.preventDefault()
      void performUndo()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [performUndo])

  const wire = useCallback((from: string, to: string) => {
    if (!from || !to || from.split(':')[0] === to.split(':')[0]) return
    undoStack.current.push({ kind: 'unwireConnection', from, to })
    void patchBoard({ wireConnection: { from, to } })
  }, [patchBoard])

  const rewireTarget = useCallback((connection: BoardConnection, to: string) => {
    if (!to || connection.to === to || connection.from.split(':')[0] === to.split(':')[0]) return
    undoStack.current.push({ kind: 'rewireConnection', from: connection.from, previousTo: to, to: connection.to })
    void patchBoard({ rewireConnection: { from: connection.from, previousTo: connection.to, to } })
  }, [patchBoard])

  const removeWire = useCallback((connection: BoardConnection) => {
    undoStack.current.push({ kind: 'wireConnection', from: connection.from, to: connection.to })
    void patchBoard({ unwireConnection: { from: connection.from, to: connection.to } })
  }, [patchBoard])

  const setGateJudge = useCallback((gate: GateNode, chain: string[]) => {
    void patchBoard({ setGateJudge: { gateId: gate.id, chain } })
  }, [patchBoard])

  const createGateAt = useCallback(async (worldX: number, worldY: number) => {
    const before = boardRef.current
    const result = await patchBoard({ createGate: { title: 'Review gate', kinds: ['code'], criterion: '', x: Math.round(worldX), y: Math.round(worldY) } })
    if (!before || !result) return
    if (isStarterBoard(result.board)) return
    const gate = findAddedByID(before.gates || [], result.board.gates || [])
    const placementEtag = result.layout?.etag || layoutRef.current?.etag
    if (gate && placementEtag) {
      try {
        const next = await patchBoardLayout(result.board.slug, placementEtag, { nodes: [{ id: gate.id, x: Math.round(worldX), y: Math.round(worldY) }] })
        setLayout(next)
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to place gate')
      }
    }
  }, [patchBoard])

  const assignSlot = useCallback((formation: FormationNode, slot: FormationSlot, agentId: string, harness: string) => {
    void patchBoard({ assignSlot: { formationId: formation.id, slotId: slot.id, agentId, harness } })
  }, [patchBoard])

  const persistPosition = useCallback(async (id: string, x: number, y: number) => {
    const currentBoard = boardRef.current
    const currentLayout = layoutRef.current
    if (!currentBoard || !currentLayout) return
    if (isStarterBoard(currentBoard)) {
      const next = withStarterLayoutRev({
        ...currentLayout,
        nodes: upsertNode(currentLayout.nodes || [], { id, x, y }),
      }, currentBoard.rev)
      layoutRef.current = next
      setLayout(next)
      setError('')
      return
    }
    try {
      const next = await patchBoardLayout(currentBoard.slug, currentLayout.etag, { nodes: [{ id, x, y }] })
      layoutRef.current = next
      setLayout(next)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save layout')
    }
  }, [])

  const runMission = useCallback(async (mission: MissionNode) => {
    const current = boardRef.current
    if (!current) return
    if (isStarterBoard(current)) {
      const runId = 'run-starter-preview'
      const events: RunEvent[] = [
        { seq: 1, type: 'run_started', runId, data: { actor: 'agent:ui' } },
        { seq: 2, type: 'run_succeeded', runId, nodeId: mission.id, data: { text: 'starter board preview run' } },
      ]
      setActiveRun({
        runId,
        status: 'succeeded',
        final: true,
        boardSlug: current.slug,
        missionId: mission.id,
        eventCount: events.length,
        resumeAllowed: false,
      })
      setRunEvents(events)
      setError('')
      return
    }
    try {
      const result = await startRun(current.etag, { board: current.slug, missionId: mission.id, actor: 'agent:ui' })
      const status = { ...result.status, runId: result.status.runId || result.runId }
      setActiveRun(status)
      setRunEvents([])
      window.localStorage.setItem(activeRunStorageKey(current.slug), status.runId)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start run')
    }
  }, [])

  const runFormation = useCallback(async (formation: FormationNode) => {
    const current = boardRef.current
    if (!current) return
    if (isStarterBoard(current)) {
      const runId = `run-starter-preview-${formation.id}`
      const events: RunEvent[] = [
        { seq: 1, type: 'run_started', runId, nodeId: formation.id, data: { actor: 'agent:ui' } },
        { seq: 2, type: 'run_succeeded', runId, nodeId: formation.id, data: { text: 'starter formation preview run' } },
      ]
      setActiveRun({
        runId,
        status: 'succeeded',
        final: true,
        boardSlug: current.slug,
        missionId: '',
        eventCount: events.length,
        resumeAllowed: false,
      })
      setRunEvents(events)
      setError('')
      return
    }
    try {
      const result = await startRun(current.etag, { board: current.slug, formationId: formation.id, actor: 'agent:ui' })
      const status = { ...result.status, runId: result.status.runId || result.runId }
      setActiveRun(status)
      setRunEvents([])
      window.localStorage.setItem(activeRunStorageKey(current.slug), status.runId)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start run')
    }
  }, [])

  const refreshRunEvents = useCallback(async (runId: string) => {
    const events = await fetchRunEvents(runId)
    setRunEvents(events)
  }, [])

  const abortActiveRun = useCallback(async () => {
    if (!activeRun?.runId || activeRun.final) return
    try {
      const status = runStatusFromResponse(await abortRunRequest(activeRun.runId, { reason: 'operator stop', requestedBy: 'agent:ui' }))
      setActiveRun(status)
      await refreshRunEvents(activeRun.runId)
      if (status.final && selectedSlug) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to abort run')
    }
  }, [activeRun, refreshRunEvents, selectedSlug])

  const resumeActiveRun = useCallback(async () => {
    if (!activeRun?.runId || activeRun.final || !activeRun.resumeAllowed) return
    try {
      const status = runStatusFromResponse(await resumeRunRequest(activeRun.runId, {
        actor: 'agent:ui',
        mode: 'reattach',
        reason: 'operator resume',
      }))
      setActiveRun(status)
      await refreshRunEvents(activeRun.runId)
      if (selectedSlug) {
        if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
        else window.localStorage.setItem(activeRunStorageKey(selectedSlug), status.runId)
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to resume run')
    }
  }, [activeRun, refreshRunEvents, selectedSlug])

  const recordHumanGateVerdict = useCallback(async (gateId: string, verdict: 'pass' | 'fail') => {
    if (!activeRun?.runId || activeRun.final) return
    try {
      const status = runStatusFromResponse(await recordGateVerdict(activeRun.runId, gateId, {
        actor: 'agent:ui',
        verdict,
        reason: verdict === 'pass' ? 'operator approved' : 'operator rejected',
      }))
      setActiveRun(status)
      await refreshRunEvents(activeRun.runId)
      if (selectedSlug) {
        if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
        else window.localStorage.setItem(activeRunStorageKey(selectedSlug), status.runId)
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to record human verdict')
    }
  }, [activeRun, refreshRunEvents, selectedSlug])

  const createSolo = useCallback(() => {
    void patchBoard({ createFormation: { type: 'solo', title: 'New formation', x: 200, y: 200 } })
  }, [patchBoard])

  // ----- pan + zoom -----
  const onViewportPointerDown = useCallback((event: ReactPointerEvent) => {
    if (event.button !== 0) return
    const target = event.target as HTMLElement
    if (target.closest('.formation,.gatecard,.missioncard,.zoomctl,.run-banner')) return
    panRef.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, originX: viewRef.current.x, originY: viewRef.current.y }
  }, [])

  const onWheel = useCallback((event: React.WheelEvent) => {
    event.preventDefault()
    const rect = viewportRef.current?.getBoundingClientRect()
    if (!rect) return
    const cursor = { x: event.clientX - rect.left, y: event.clientY - rect.top }
    setView(current => zoomTransform(current, event.deltaY < 0 ? 1.12 : 0.892, cursor))
  }, [])

  const zoomBy = useCallback((factor: number) => {
    const rect = viewportRef.current?.getBoundingClientRect()
    const cursor = rect ? { x: rect.width / 2, y: rect.height / 2 } : undefined
    setView(current => zoomTransform(current, factor, cursor))
  }, [])

  const fitView = useCallback(() => {
    const currentBoard = boardRef.current
    const rect = viewportRef.current?.getBoundingClientRect()
    const world = worldRef.current
    if (!currentBoard || !rect || !world) return
    const cards = Array.from(world.querySelectorAll<HTMLElement>('.formation,.gatecard,.missioncard'))
    if (!cards.length) { setView({ x: 40, y: 40, scale: 1 }); return }
    const worldRect = world.getBoundingClientRect()
    const scale = viewRef.current.scale || 1
    let minX = 1e9, minY = 1e9, maxX = -1e9, maxY = -1e9
    cards.forEach(card => {
      const r = card.getBoundingClientRect()
      const x = (r.left - worldRect.left) / scale
      const y = (r.top - worldRect.top) / scale
      minX = Math.min(minX, x); minY = Math.min(minY, y)
      maxX = Math.max(maxX, x + r.width / scale); maxY = Math.max(maxY, y + r.height / scale)
    })
    const pad = 64
    const next = clampScale(Math.min(rect.width / (maxX - minX + pad * 2), rect.height / (maxY - minY + pad * 2)), 1.1)
    setView({
      scale: next,
      x: (rect.width - (maxX - minX) * next) / 2 - minX * next,
      y: (rect.height - (maxY - minY) * next) / 2 - minY * next,
    })
  }, [])

  useLayoutEffect(() => {
    if (!board || !layout || fittedBoardRef.current === board.slug) return
    const frame = window.requestAnimationFrame(() => {
      fitView()
      fittedBoardRef.current = board.slug
    })
    return () => window.cancelAnimationFrame(frame)
  }, [board, layout, fitView])

  const screenToWorld = useCallback((clientX: number, clientY: number) => {
    const rect = viewportRef.current?.getBoundingClientRect()
    const v = viewRef.current
    if (!rect) return { x: 0, y: 0 }
    return { x: (clientX - rect.left - v.x) / (v.scale || 1), y: (clientY - rect.top - v.y) / (v.scale || 1) }
  }, [])

  // ----- global pointer handling for pan, card/staff/wire/gate drags -----
  useEffect(() => {
    const onMove = (event: globalThis.PointerEvent) => {
      if (panRef.current && panRef.current.pointerId === event.pointerId) {
        const p = panRef.current
        setView(current => ({ ...current, x: p.originX + (event.clientX - p.startX), y: p.originY + (event.clientY - p.startY) }))
        viewportRef.current?.classList.add('panning')
        return
      }
      if (dragNodeRef.current && dragNodeRef.current.pointerId === event.pointerId) {
        const d = dragNodeRef.current
        const scale = viewRef.current.scale || 1
        const nx = d.originX + (event.clientX - d.startX) / scale
        const ny = d.originY + (event.clientY - d.startY) / scale
        if (Math.abs(event.clientX - d.startX) + Math.abs(event.clientY - d.startY) > 2) d.moved = true
        setDragPos({ id: d.id, x: Math.round(nx), y: Math.round(ny) })
        return
      }
      if (wireDragRef.current) {
        const w = screenToWorld(event.clientX, event.clientY)
        setTempWire(prev => (prev ? { ...prev, bx: w.x, by: w.y } : prev))
        const inPort = findInputPortAt(event.clientX, event.clientY)
        setHoverPort(inPort?.dataset.portIn ?? null)
        return
      }
      if (gateDragRef.current) {
        setGateGhost({ x: event.clientX, y: event.clientY })
        return
      }
      if (staffRef.current) {
        setGhost({ x: event.clientX, y: event.clientY, agentId: staffRef.current.agentId })
        const el = document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null
        const slotEl = el?.closest<HTMLElement>('.slot')
        setHoverSlot(slotEl ? `${slotEl.dataset.fid}:${slotEl.dataset.sid}` : null)
      }
    }
    const onUp = (event: globalThis.PointerEvent) => {
      if (panRef.current && panRef.current.pointerId === event.pointerId) {
        panRef.current = null
        viewportRef.current?.classList.remove('panning')
      }
      if (dragNodeRef.current && dragNodeRef.current.pointerId === event.pointerId) {
        const d = dragNodeRef.current
        dragNodeRef.current = null
        if (d.moved) {
          const scale = viewRef.current.scale || 1
          void persistPosition(d.id, Math.round(d.originX + (event.clientX - d.startX) / scale), Math.round(d.originY + (event.clientY - d.startY) / scale))
        }
        setDragPos(null)
      }
      if (wireDragRef.current) {
        const active = wireDragRef.current
        const inPort = findInputPortAt(event.clientX, event.clientY)
        if (inPort?.dataset.portIn) {
          if (active.kind === 'new') wire(active.from, inPort.dataset.portIn)
          else rewireTarget(active.connection, inPort.dataset.portIn)
        }
        wireDragRef.current = null
        setTempWire(null)
        setHoverPort(null)
      }
      if (gateDragRef.current) {
        gateDragRef.current = false
        const rect = viewportRef.current?.getBoundingClientRect()
        if (rect && event.clientX >= rect.left && event.clientX <= rect.right && event.clientY >= rect.top && event.clientY <= rect.bottom) {
          const w = screenToWorld(event.clientX, event.clientY)
          void createGateAt(w.x, w.y)
        }
        setGateGhost(null)
      }
      if (staffRef.current) {
        const staff = staffRef.current
        const el = document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null
        const slotEl = el?.closest<HTMLElement>('.slot')
        if (slotEl && slotEl.dataset.fid && slotEl.dataset.sid) {
          const f = boardRef.current?.formations.find(item => item.id === slotEl.dataset.fid)
          const s = f?.slots.find(item => item.id === slotEl.dataset.sid)
          if (f && s) assignSlot(f, s, staff.agentId, staff.harness)
        }
        staffRef.current = null
        setGhost(null)
        setHoverSlot(null)
      }
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [assignSlot, persistPosition, rewireTarget, wire, createGateAt, screenToWorld])

  const beginStaff = useCallback((event: ReactPointerEvent, agentId: string, harness: string, fromSlot?: DragStaff['fromSlot']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    staffRef.current = { agentId, harness, fromSlot }
    setGhost({ x: event.clientX, y: event.clientY, agentId })
  }, [])

  const beginNodeDrag = useCallback((event: ReactPointerEvent, id: string, index: number) => {
    if (event.button !== 0) return
    const target = event.target as HTMLElement
    if (target.closest('.port,.frun,.mrun')) return
    event.stopPropagation()
    const pos = positionOf(id, index)
    dragNodeRef.current = { id, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, originX: pos.x, originY: pos.y, moved: false }
  }, [positionOf])

  const portWorldCenter = useCallback((endpoint: string) => {
    const world = worldRef.current
    if (!world) return null
    const el = world.querySelector<HTMLElement>(`[data-port-out="${endpoint.replace(/["\\]/g, '\\$&')}"]`)
    if (!el) return null
    const wr = world.getBoundingClientRect()
    const r = el.getBoundingClientRect()
    const s = viewRef.current.scale || 1
    return { x: (r.left + r.width / 2 - wr.left) / s, y: (r.top + r.height / 2 - wr.top) / s }
  }, [])

  const beginWire = useCallback((event: ReactPointerEvent, endpoint: string, kind: WirePath['kind']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    wireDragRef.current = { kind: 'new', from: endpoint, wireKind: kind }
    const start = portWorldCenter(endpoint)
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind } : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind })
  }, [portWorldCenter, screenToWorld])

  const beginReconnect = useCallback((event: ReactPointerEvent<HTMLElement> | ReactMouseEvent<HTMLElement>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (wireDragRef.current) return
    event.stopPropagation()
    wireDragRef.current = { kind: 'reconnect-target', connection }
    const start = portWorldCenter(connection.from)
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind: 'wire' } : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind: 'wire' })
  }, [portWorldCenter, screenToWorld])

  const captureConnectedInputDrag = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    const port = target.closest<HTMLElement>('[data-port-in]')
    const endpoint = port?.dataset.portIn
    if (!endpoint) return
    const connection = boardRef.current?.connections.find(candidate => candidate.to === endpoint) || (
      port?.dataset.reconnectFrom
        ? { id: port.dataset.reconnectId || `${port.dataset.reconnectFrom}-${endpoint}`, from: port.dataset.reconnectFrom, to: endpoint }
        : undefined
    )
    if (connection) beginReconnect(event, connection)
  }, [beginReconnect])

  const beginWireLaneDrag = useCallback((event: ReactPointerEvent<SVGPathElement>, connection: BoardConnection) => {
    if (event.button !== 0) return
    event.stopPropagation()
    const startX = event.clientX
    const startY = event.clientY
    let moved = false
    const move = (moveEvent: globalThis.PointerEvent) => {
      if (Math.abs(moveEvent.clientX - startX) + Math.abs(moveEvent.clientY - startY) > 8) moved = true
    }
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      if (moved) void patchLayoutEdge(connection.id, 'manual')
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }, [patchLayoutEdge])

  const beginGateToken = useCallback((event: ReactPointerEvent) => {
    if (event.button !== 0) return
    event.preventDefault()
    gateDragRef.current = true
    setGateGhost({ x: event.clientX, y: event.clientY })
  }, [])

  const formationMenu = useCallback((event: ReactMouseEvent<HTMLElement>, formation: FormationNode) => {
    openMenu(event, 'Formation actions', [
      { label: 'Run formation', action: () => void runFormation(formation) },
      { label: 'Set input', action: () => openBriefEditor(formation) },
    ])
  }, [openBriefEditor, openMenu, runFormation])

  const wireMenu = useCallback((event: ReactMouseEvent<SVGPathElement>, connection: BoardConnection) => {
    openMenu(event, 'Connection actions', [
      { label: 'Reset routing', action: () => void patchLayoutEdge(connection.id, 'auto') },
      { label: 'Remove connection', destructive: true, action: () => removeWire(connection) },
    ])
  }, [openMenu, patchLayoutEdge, removeWire])

  const judgeMenu = useCallback((event: ReactMouseEvent<HTMLElement>, gate: GateNode) => {
    openMenu(event, 'Judge socket actions', [
      ...((boardRef.current?.formations || []).map(formation => ({
        label: `Use ${formation.title}`,
        action: () => setGateJudge(gate, [formation.id]),
      }))),
    ])
  }, [openMenu, setGateJudge])

  // ----- render helpers -----
  const renderSlot = (formation: FormationNode, slot: FormationSlot, badge?: number) => {
    const filled = !!slot.agentId
    const key = `${formation.id}:${slot.id}`
    const runState = nodeStates.get(formation.id)
    const classes = ['slot', filled ? 'filled' : 'empty', slot.controller ? 'ctrl' : '', hoverSlot === key ? 'snaptarget' : '', runState === 'running' ? 'active' : '', runState === 'done' ? 'active done' : '']
    return (
      <div
        key={slot.id}
        className={classes.filter(Boolean).join(' ')}
        data-fid={formation.id}
        data-sid={slot.id}
        data-testid={`slot-${formation.id}-${slot.id}`}
        onPointerDown={filled ? event => beginStaff(event, slot.agentId as string, slot.harness || '', { formationId: formation.id, slotId: slot.id }) : undefined}
      >
        <div className="slot-ring">
          {badge ? <span className="badge">{badge}</span> : null}
          {filled
            ? <span className="face" style={{ background: agentColor(slot.agentId as string) }}>{initials(slot.agentId as string)}</span>
            : <span className="plus">+</span>}
        </div>
        <div className="slot-label">{slot.label}</div>
        {filled ? <div className="who">{slot.agentId}</div> : null}
      </div>
    )
  }

  const renderBody = (formation: FormationNode) => {
    const slots = formation.slots
    if (formation.type === 'peer') {
      return (
        <div className="huddle"><span className="hl">peers · no hierarchy</span>
          <div className="peers-row">{slots.map(slot => renderSlot(formation, slot))}</div>
        </div>
      )
    }
    if (formation.type === 'flow') {
      return (
        <div className="flow-row">
          {slots.map((slot, index) => (
            <div key={slot.id} style={{ display: 'flex', alignItems: 'flex-start', gap: 6 }}>
              {renderSlot(formation, slot, index + 1)}
              {index < slots.length - 1 ? (
                <div className="flow-arrow"><svg viewBox="0 0 26 12" fill="none" stroke="currentColor" strokeWidth="1.6"><path d="M0 6h22" /><path d="M19 2l4 4-4 4" /></svg></div>
              ) : null}
            </div>
          ))}
        </div>
      )
    }
    if (formation.type === 'orchestrated') {
      const ctrl = slots.find(slot => slot.controller) || slots[0]
      const workers = slots.filter(slot => slot !== ctrl)
      return (
        <div className="orch">
          <div className="ctrl-wrap">{ctrl ? renderSlot(formation, ctrl) : null}</div>
          <div className="pool"><span className="pl">open slots</span>{workers.map(slot => renderSlot(formation, slot))}</div>
        </div>
      )
    }
    return <div className="solo-body">{slots[0] ? renderSlot(formation, slots[0]) : null}</div>
  }

  const runBadgeClass = activeRun ? activeRun.status : ''
  const pendingHumanGateId = useMemo(() => openHumanGateId(runEvents), [runEvents])

  return (
    <div className="fmx" data-testid="formations-view" data-cockpit="d7">
      <div className="topbar">
        <div className="brand"><span className="nm">CHR<b>O</b>TE</span><span className="sub">Formations</span></div>
        <div className="spacer" />
        <div className="boardpick">
          board
          <select value={selectedSlug} onChange={event => setSelectedSlug(event.target.value)} data-testid="board-picker">
            {boards.map(summary => <option key={summary.slug} value={summary.slug}>{summary.title || summary.slug}</option>)}
          </select>
          {board ? <span className="rev">rev {board.rev}</span> : null}
        </div>
        <div className="gatetoken" title="Drag onto the canvas to drop a gate" data-testid="gate-token" onPointerDown={beginGateToken}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M5 11V7a7 7 0 0114 0v4" /><rect x="4" y="11" width="16" height="9" rx="2" /></svg>
          Gate
        </div>
        <button className="newbtn" onClick={createSolo} data-testid="new-formation">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 5v14M5 12h14" /></svg>
          New formation
        </button>
        <button className="helpbtn" title="Drag an agent from the roster into a slot to staff it.">?</button>
      </div>

      <div className="main">
        <aside className="roster" data-testid="agent-roster">
          <div className="roster-hd">
            <div className="t">Agent roster</div>
            <div className="s">{agents.length} agents · {new Set((board?.formations || []).flatMap(f => f.slots.map(s => s.agentId).filter(Boolean))).size} deployed</div>
          </div>
          <div className="roster-list">
            {agents.length === 0
              ? <div className="roster-empty">No agents on the socket. Spawn an agent to staff a formation.</div>
              : agents.map(agent => {
                const deployed = (board?.formations || []).some(f => f.slots.some(s => s.agentId === agent.id))
                return (
                  <div
                    key={agent.id}
                    className={`ragent${deployed ? ' deployed' : ''}${agent.unbound ? ' unbound' : ''}`}
                    data-agent={agent.id}
                    data-testid={`roster-agent-${agent.id}`}
                    onPointerDown={event => beginStaff(event, agent.id, agent.harnessDefault || '')}
                  >
                    <span className="av" style={{ background: agentColor(agent.id) }}>{initials(agent.id)}</span>
                    <div className="ri">
                      <div className="n">{agent.displayName || agent.id}</div>
                      <div className="r">{agentRole(agent)}</div>
                    </div>
                    <span className={`sd ${agentState(agent)}`} />
                  </div>
                )
              })}
          </div>
        </aside>

        <div className="viewport" data-testid="formations-canvas" ref={viewportRef} onPointerDownCapture={captureConnectedInputDrag} onPointerDown={onViewportPointerDown} onWheel={onWheel}>
          <div className="world" data-testid="formations-world" ref={worldRef} style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})` }}>
            <svg className="wires" width={3400} height={2300}>
              {wires.map(path => {
                const connection = board?.connections.find(candidate => candidate.id === path.id)
                if (!connection) return null
                return (
                  <g key={path.id}>
                    <path
                      className="wirehit"
                      d={path.d}
                      onPointerDown={event => beginWireLaneDrag(event, connection)}
                      onContextMenu={event => wireMenu(event, connection)}
                    />
                    <path
                      className={`wire ${path.kind}${path.flowing ? ' flowing' : ''}`}
                      data-testid={`formation-wire-${path.id}`}
                      d={path.d}
                      onPointerDown={event => beginWireLaneDrag(event, connection)}
                      onContextMenu={event => wireMenu(event, connection)}
                    />
                  </g>
                )
              })}
              {tempWire ? (
                <path className="temp" d={`M ${tempWire.ax} ${tempWire.ay} C ${tempWire.ax + 50} ${tempWire.ay}, ${tempWire.bx - 50} ${tempWire.by}, ${tempWire.bx} ${tempWire.by}`} />
              ) : null}
            </svg>

            {(board?.missions || []).map((mission, index) => {
              const pos = positionOf(mission.id, index)
              const state = nodeStates.get(mission.id)
              return (
                <div
                  key={mission.id}
                  className="missioncard"
                  data-node={mission.id}
                  data-testid={`mission-node-${mission.id}`}
                  style={{ left: pos.x, top: pos.y }}
                  onPointerDown={event => beginNodeDrag(event, mission.id, index)}
                >
                  <div className="mhd">
                    <span className="meyebrow">◆ Mission</span>
                    <button className="mrun" title="Start mission" onClick={() => void runMission(mission)} data-testid={`run-mission-${mission.id}`}>{PLAY_SVG}</button>
                  </div>
                  <div className="mtitle">{mission.title}</div>
                  <div className={`mgoal${mission.goal ? '' : ' placeholder'}`}>{mission.goal || 'set the mission objective…'}</div>
                  <div className="mstatus">{state ? state : ''}</div>
                  <span className="port pout ready" data-port-out={`${mission.id}:out`} title="Starts the chain — drag to a step" onPointerDown={event => beginWire(event, `${mission.id}:out`, 'wire')} />
                </div>
              )
            })}

            {(board?.formations || []).map((formation, index) => {
              const pos = positionOf(formation.id, index + (board?.missions?.length || 0))
              const state = nodeStates.get(formation.id)
              return (
                <div
                  key={formation.id}
                  className={`formation type-${formation.type}${state === 'running' ? ' running' : ''}`}
                  data-node={formation.id}
                  data-testid={`formation-node-${formation.id}`}
                  style={{ left: pos.x, top: pos.y }}
                  onContextMenu={event => formationMenu(event, formation)}
                >
                  {formation.inputs.map((port, portIndex) => {
                    const endpoint = `${formation.id}:${port.id}`
                    const incoming = (board?.connections || []).find(connection => connection.to === endpoint)
                    return (
                      <div className={`fio in${portIndex === 0 ? ' brief' : ''}`} key={port.id} onPointerDown={event => beginNodeDrag(event, formation.id, index)}>
                        <span
                          className={`port pin${hoverPort === endpoint ? ' snaptarget' : ''}${incoming ? ' has' : ''}`}
                          data-port-in={endpoint}
                          data-reconnect-id={incoming?.id}
                          data-reconnect-from={incoming?.from}
                          onPointerDown={incoming ? event => beginReconnect(event, incoming) : undefined}
                          onMouseDown={incoming ? event => beginReconnect(event, incoming) : undefined}
                        />
                        <span className="glyph">in</span>
                        <span className="io-text placeholder">{portIndex === 0 ? (formation.brief?.goal || 'set a goal or input…') : `${port.label} — wire an input…`}</span>
                      </div>
                    )
                  })}
                  <div className="fhead" onPointerDown={event => beginNodeDrag(event, formation.id, index)}>
                    <div className="ft"><div className="tt">{formation.title}</div><div className="tg">{TYPE_TAG[formation.type]}</div></div>
                    <button className="frun" title="Run formation" onClick={() => void runFormation(formation)} data-testid={`run-formation-${formation.id}`}>{PLAY_SVG}</button>
                  </div>
                  <div className="fstatus">{state && state !== 'done' ? state : ''}</div>
                  <div className="fbody" onPointerDown={event => beginNodeDrag(event, formation.id, index)}>{renderBody(formation)}</div>
                  <div className={`verify-band${formation.verification ? '' : ' empty'}`} data-gate={formation.verification?.id}>
                    <span className="vico"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9"><path d="M12 3l7 3v5c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6z" /><path d="M9 12l2 2 4-4" /></svg></span>
                    <span className="vlabel">{formation.verification ? 'verify' : '+ verify'}</span>
                    {formation.verification ? <span className="vkinds">{formation.verification.kinds.join(' · ')} · {formation.verification.criterion}</span> : null}
                  </div>
                  {formation.outputs.map((port, portIndex) => (
                    <div className="fio out" key={port.id}>
                      <span className="glyph">out</span>
                      <span className="io-status idle">{portIndex === 0 ? 'no output yet' : port.label.toLowerCase()}</span>
                      <span className="port pout ready" data-port-out={`${formation.id}:${port.id}`} title="Drag to a downstream input" onPointerDown={event => beginWire(event, `${formation.id}:${port.id}`, 'wire')} />
                    </div>
                  ))}
                </div>
              )
            })}

            {(board?.gates || []).map((gate, index) => {
              const nodeIndex = index + (board?.missions?.length || 0) + (board?.formations?.length || 0)
              const pos = positionOf(gate.id, nodeIndex)
              const state = nodeStates.get(gate.id)
              const inputEndpoint = `${gate.id}:in`
              const incoming = (board?.connections || []).find(connection => connection.to === inputEndpoint)
              return (
                <div
                  key={gate.id}
                  className={`gatecard${state ? ` ${state}` : ''}`}
                  data-node={gate.id}
                  data-gate={gate.id}
                  data-testid={`gate-node-${gate.id}`}
                  style={{ left: pos.x, top: pos.y }}
                  onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}
                >
                  <span
                    className={`port pin${hoverPort === inputEndpoint ? ' snaptarget' : ''}${incoming ? ' has' : ''}`}
                    data-port-in={inputEndpoint}
                    data-reconnect-id={incoming?.id}
                    data-reconnect-from={incoming?.from}
                    title="Work to check"
                    onPointerDown={incoming ? event => beginReconnect(event, incoming) : undefined}
                    onMouseDown={incoming ? event => beginReconnect(event, incoming) : undefined}
                  />
                  <button
                    type="button"
                    className="pjudge"
                    data-testid={`gate-judge-socket-${gate.id}`}
                    data-gate-judge-socket={gate.id}
                    title="Judge with a formation"
                    onPointerDown={event => event.stopPropagation()}
                    onClick={event => judgeMenu(event, gate)}
                  />
                  <span className="gico" onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}>{GATE_SVG}</span>
                  <span className="gmeta" onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}><span className="gt">{gate.title || gate.kinds.join(' · ') || 'Gate'}</span><span className="gs">{[gate.kinds.join(' · '), gate.criterion || 'work is accepted before it proceeds'].filter(Boolean).join(' · ')}</span></span>
                  <span className="glabel pass"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4"><path d="M4 12l5 5L20 6" /></svg>pass</span>
                  <span className="glabel fail"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4"><path d="M6 6l12 12M18 6L6 18" /></svg>fail</span>
                  <span className="port pass" data-port-out={`${gate.id}:pass`} title="On PASS → drag to the next step" onPointerDown={event => beginWire(event, `${gate.id}:pass`, 'pass')} />
                  <span className="port fail" data-port-out={`${gate.id}:fail`} title="On FAIL → drag to a fallback" onPointerDown={event => beginWire(event, `${gate.id}:fail`, 'fail')} />
                </div>
              )
            })}
          </div>

          {activeRun ? (
            <div className="run-banner" data-testid="run-banner">
              <span>run</span>
              <span className={`badge ${runBadgeClass}`}>{activeRun.status}</span>
              {!activeRun.final && pendingHumanGateId ? (
                <>
                  <button type="button" aria-label={`Approve gate ${pendingHumanGateId}`} onClick={() => void recordHumanGateVerdict(pendingHumanGateId, 'pass')}>approve</button>
                  <button type="button" aria-label={`Reject gate ${pendingHumanGateId}`} onClick={() => void recordHumanGateVerdict(pendingHumanGateId, 'fail')}>reject</button>
                </>
              ) : null}
              {!activeRun.final && activeRun.resumeAllowed ? <button type="button" onClick={() => void resumeActiveRun()}>Resume run</button> : null}
              {!activeRun.final ? <button type="button" onClick={() => void abortActiveRun()}>stop</button> : null}
            </div>
          ) : null}

          <div className="zoomlevel">{Math.round(view.scale * 100)}%</div>
          <div className="zoomctl">
            <button onClick={() => zoomBy(1.2)} title="Zoom in">+</button>
            <button onClick={() => zoomBy(1 / 1.2)} title="Zoom out">−</button>
            <button onClick={fitView} title="Fit">FIT</button>
          </div>
          {error ? <div className="errbar" data-testid="formations-error">{error}</div> : null}
        </div>
      </div>

      {briefEditor ? (
        <div
          className="pop"
          role="dialog"
          aria-label={`Input · ${briefEditor.title}`}
          onPointerDown={event => event.stopPropagation()}
        >
          <div className="pop-head">
            <span className="pt">Input · {briefEditor.title}</span>
            <button className="x" type="button" aria-label="Close input editor" onClick={() => setBriefEditor(null)}>x</button>
          </div>
          <div className="pop-body">
            <label htmlFor="cockpit-brief-goal">Goal / idea</label>
            <textarea
              id="cockpit-brief-goal"
              aria-label={`Goal for ${briefEditor.title}`}
              value={briefEditor.goal}
              onChange={event => setBriefEditor(current => current ? { ...current, goal: event.target.value } : current)}
            />
            <label htmlFor="cockpit-brief-bead">Bead</label>
            <input
              id="cockpit-brief-bead"
              className="f"
              aria-label={`Bead for ${briefEditor.title}`}
              value={briefEditor.beadId}
              onChange={event => setBriefEditor(current => current ? { ...current, beadId: event.target.value } : current)}
            />
            <label htmlFor="cockpit-brief-files">File links & context</label>
            <input
              id="cockpit-brief-files"
              className="f"
              aria-label={`Files for ${briefEditor.title}`}
              value={briefEditor.files}
              onChange={event => setBriefEditor(current => current ? { ...current, files: event.target.value } : current)}
            />
            <label htmlFor="cockpit-brief-links">Links</label>
            <input
              id="cockpit-brief-links"
              className="f"
              aria-label={`Links for ${briefEditor.title}`}
              value={briefEditor.links}
              onChange={event => setBriefEditor(current => current ? { ...current, links: event.target.value } : current)}
            />
            <button className="save" type="button" onClick={() => void saveBriefEditor()}>Save input</button>
          </div>
        </div>
      ) : null}

      {menu ? (
        <div
          className="ctxmenu"
          role="menu"
          aria-label={menu.label}
          style={{ left: Math.min(menu.x, window.innerWidth - 220), top: Math.min(menu.y, window.innerHeight - 80) }}
          onPointerDown={event => event.stopPropagation()}
        >
          <div className="mhead">{menu.label}</div>
          {menu.items.map(item => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              className={item.destructive ? 'danger' : undefined}
              onClick={() => {
                closeMenu()
                item.action?.()
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}

      {ghost ? (
        <div className="fmx-ghost" style={{ left: ghost.x, top: ghost.y, background: agentColor(ghost.agentId) }}>{initials(ghost.agentId)}</div>
      ) : null}
      {gateGhost ? (
        <div className="gateghost" style={{ left: gateGhost.x, top: gateGhost.y }}>{GATE_SVG}</div>
      ) : null}
    </div>
  )
}
