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
  openHumanGateId,
  projectNodeStates,
  runStatusFromResponse,
  upsertRunEvent,
} from './formationsRunState'
import { clampScale, displayLayoutFor, fallbackNodePosition, zoomTransform } from './formationsCanvas'
import { GATE_SVG, PLAY_SVG, TYPE_TAG, agentColor, agentRole, agentState, initials } from './formationsCockpitVisuals'
import { connectionKind, findInputPortAt, findOutputPortAt, isTextEditingTarget, laneYFrom, splitList } from './formationsCockpitDom'
import { routeJudgeWire, routeOrthoWire } from './formationsRouting'
import type { ObstacleRect } from './formationsRouting'
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



type DragStaff = { agentId: string; harness: string; fromSlot?: { formationId: string; slotId: string }; startX: number; startY: number; moved: boolean }
type DragNode = { id: string; pointerId: number; startX: number; startY: number; originX: number; originY: number; moved: boolean }
type WirePath = { id: string; d: string; kind: 'wire' | 'pass' | 'fail' | 'judge'; flowing: boolean }
type WireDrag =
  | { kind: 'new'; from: string; wireKind: WirePath['kind'] }
  | { kind: 'reconnect-target'; connection: BoardConnection }
  | { kind: 'reconnect-source'; connection: BoardConnection }
  | { kind: 'judge'; gate: GateNode; moved: boolean; startX: number; startY: number }
type LaneDrag = { connectionId: string; previousLane: string; moved: boolean }
type MenuItem = { label: string; action?: () => void; destructive?: boolean; disabled?: boolean; head?: boolean }
type MenuState = { label: string; x: number; y: number; items: MenuItem[] }
type VerificationEditorState = { formationId: string; title: string; kinds: string[]; criterion: string; onFail: 'block' | 'pushback' }
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
  | { kind: 'rewireSource'; previousFrom: string; from: string; to: string }
  | { kind: 'deleteFormation'; id: string }
  | { kind: 'deleteGate'; id: string }
  | { kind: 'deleteMission'; id: string }
  | { kind: 'assignSlot'; formationId: string; slotId: string; agentId: string; harness: string }
  | { kind: 'moveNode'; id: string; x: number; y: number }
  | { kind: 'setLane'; edgeId: string; lane: string }
  | { kind: 'setGateJudge'; gateId: string; chain: string[] }
  | { kind: 'detachGateJudge'; gateId: string }
  | { kind: 'setVerification'; formationId: string; kinds: string[]; criterion: string; onFail: string }
  | { kind: 'removeVerification'; formationId: string }
  | { kind: 'removePort'; formationId: string; portId: string }
  | { kind: 'makeController'; formationId: string; slotId: string }

export default function FormationsCockpit({ active = true }: { active?: boolean } = {}) {
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
  const [tempWire, setTempWire] = useState<{ ax: number; ay: number; bx: number; by: number; kind: WirePath['kind']; moving: 'a' | 'b' } | null>(null)
  const [hoverPort, setHoverPort] = useState<string | null>(null)
  const [gateGhost, setGateGhost] = useState<{ x: number; y: number } | null>(null)
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [briefEditor, setBriefEditor] = useState<BriefEditorState | null>(null)
  const [verificationEditor, setVerificationEditor] = useState<VerificationEditorState | null>(null)
  const [hiddenWireId, setHiddenWireId] = useState<string | null>(null)
  const [judgeHover, setJudgeHover] = useState<string | null>(null)
  const [laneDraft, setLaneDraft] = useState<{ connectionId: string; y: number } | null>(null)

  const boardRef = useRef<BoardDocument | null>(null)
  const layoutRef = useRef<LayoutDocument | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const worldRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<ViewTransform>(view)
  const staffRef = useRef<DragStaff | null>(null)
  const dragNodeRef = useRef<DragNode | null>(null)
  const panRef = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null)
  const wireDragRef = useRef<WireDrag | null>(null)
  const laneDragRef = useRef<LaneDrag | null>(null)
  const gateDragRef = useRef<boolean>(false)
  const undoStack = useRef<CockpitUndo[]>([])
  const fittedBoardRef = useRef<string | null>(null)
  const judgeHoverRef = useRef<string | null>(null)
  const openJudgePickerRef = useRef<((gate: GateNode, x: number, y: number) => void) | null>(null)

  viewRef.current = view
  useEffect(() => { boardRef.current = board }, [board])
  useEffect(() => { layoutRef.current = layout }, [layout])
  useEffect(() => { judgeHoverRef.current = judgeHover }, [judgeHover])

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
    // Paused while the tab is hidden (keep-alive); reactivation re-runs this
    // effect and refreshes immediately via the leading checkChanges() call.
    if (!active || !selectedSlug || !board?.rev || isStarterBoard(board)) return
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
  }, [active, board?.rev, selectedSlug])

  useEffect(() => {
    if (!active) return
    let cancelled = false
    const load = () => fetchAgents().then(list => !cancelled && setAgents(list)).catch(() => undefined)
    load()
    const timer = window.setInterval(load, 8000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [active])

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
    const cssEscape = (value: string) => value.replace(/["\\]/g, '\\$&')
    const centerOfEl = (el: HTMLElement | null): { x: number; y: number } | null => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: (r.left + r.width / 2 - worldRect.left) / scale, y: (r.top + r.height / 2 - worldRect.top) / scale }
    }
    // Judge endpoints (`<gateId>:judge`) anchor on the gate's top socket, which is
    // not a data-port element; everything else resolves by exact port endpoint.
    const endpointEl = (endpoint: string, direction: 'out' | 'in'): HTMLElement | null => {
      const [nodeId, portId] = endpoint.split(':')
      if (portId === 'judge') return world.querySelector<HTMLElement>(`[data-gate-judge-socket="${cssEscape(nodeId)}"]`)
      return world.querySelector<HTMLElement>(`[data-port-${direction}="${cssEscape(endpoint)}"]`)
    }
    // Card boxes in world coordinates double as routing obstacles and judge-bracket anchors.
    const obstacles: ObstacleRect[] = Array.from(world.querySelectorAll<HTMLElement>('[data-node]')).map(el => {
      const r = el.getBoundingClientRect()
      return {
        id: el.dataset.node || '',
        x: (r.left - worldRect.left) / scale,
        y: (r.top - worldRect.top) / scale,
        width: r.width / scale,
        height: r.height / scale,
      }
    })
    const cardRect = (id: string): ObstacleRect | null => obstacles.find(rect => rect.id === id) || null
    const layoutEdges = layout?.edges || []
    const laneFor = (connectionId: string): number | null => {
      if (laneDraft && laneDraft.connectionId === connectionId) return laneDraft.y
      return laneYFrom(layoutEdges.find(edge => edge.id === connectionId)?.lane)
    }
    const paths: WirePath[] = []
    for (const conn of board.connections || []) {
      if (conn.id === hiddenWireId) continue // being reconnected — drawn as the temp wire instead
      const [fromNode, fromPort] = conn.from.split(':')
      const [toNode] = conn.to.split(':')
      const a = centerOfEl(endpointEl(conn.from, 'out'))
      const b = centerOfEl(endpointEl(conn.to, 'in'))
      if (!a || !b) continue
      const kind = connectionKind(conn)
      let d: string
      let flowing: boolean
      if (kind === 'judge') {
        const direction = fromPort === 'judge' ? 'send' : 'return'
        const gateId = fromPort === 'judge' ? fromNode : toNode
        const judgeId = fromPort === 'judge' ? toNode : fromNode
        d = routeJudgeWire(a, b, { direction, nodeRect: cardRect(judgeId) })
        // Judge wires pulse only while their gate evaluates (reference drawWires).
        flowing = nodeStates.get(gateId) === 'running'
      } else {
        d = routeOrthoWire(a, b, {
          fromId: fromNode,
          toId: toNode,
          obstacles,
          frozen: !!dragPos,
          laneY: laneFor(conn.id),
        })
        flowing = nodeStates.get(fromNode) === 'running' || nodeStates.get(toNode) === 'running'
      }
      paths.push({ id: conn.id, d, kind, flowing })
    }
    setWires(paths)
  }, [board, layout, view, dragPos, agents, nodeStates, hiddenWireId, laneDraft])

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
    const retry = () => undoStack.current.push(action)
    if (action.kind === 'rewireSource') {
      // Undo of a source-end reconnect is two sequential ops: drop the new wire,
      // restore the previous one.
      const removed = await patchBoard({ unwireConnection: { from: action.from, to: action.to } })
      if (!removed) { retry(); return }
      await patchBoard({ wireConnection: { from: action.previousFrom, to: action.to } })
      return
    }
    if (action.kind === 'moveNode') {
      await persistPosition(action.id, action.x, action.y)
      return
    }
    if (action.kind === 'setLane') {
      await patchLayoutEdge(action.edgeId, action.lane)
      return
    }
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
      case 'deleteFormation':
        patch = { deleteFormation: { id: action.id } }
        break
      case 'deleteGate':
        patch = { deleteGate: { id: action.id } }
        break
      case 'deleteMission':
        patch = { deleteMission: { id: action.id } }
        break
      case 'assignSlot':
        patch = { assignSlot: { formationId: action.formationId, slotId: action.slotId, agentId: action.agentId, harness: action.harness } }
        break
      case 'setGateJudge':
        patch = { setGateJudge: { gateId: action.gateId, chain: action.chain } }
        break
      case 'detachGateJudge':
        patch = { detachGateJudge: { gateId: action.gateId } }
        break
      case 'setVerification':
        patch = { setVerification: { formationId: action.formationId, kinds: action.kinds, criterion: action.criterion, onFail: action.onFail } }
        break
      case 'removeVerification':
        patch = { removeVerification: { formationId: action.formationId } }
        break
      case 'removePort':
        patch = { removePort: { formationId: action.formationId, portId: action.portId } }
        break
      case 'makeController':
        patch = { makeController: { formationId: action.formationId, slotId: action.slotId } }
        break
    }
    const result = await patchBoard(patch)
    if (!result) retry()
  }, [patchBoard, patchLayoutEdge, persistPosition])

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

  // Context menus dismiss on outside click, Escape, or scroll (reference behavior).
  useEffect(() => {
    if (!menu) return
    const onPointerDown = (event: Event) => {
      const target = event.target as HTMLElement | null
      if (!target?.closest?.('.ctxmenu')) setMenu(null)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenu(null)
    }
    window.addEventListener('pointerdown', onPointerDown, true)
    window.addEventListener('wheel', onPointerDown, { capture: true, passive: true })
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown, true)
      window.removeEventListener('wheel', onPointerDown, true)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [menu])

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

  const createGateAt = useCallback(async (worldX: number, worldY: number) => {
    const before = boardRef.current
    const result = await patchBoard({ createGate: { title: 'Review gate', kinds: ['code'], criterion: '', x: Math.round(worldX), y: Math.round(worldY) } })
    if (!before || !result) return
    if (isStarterBoard(result.board)) return
    const gate = findAddedByID(before.gates || [], result.board.gates || [])
    if (gate) undoStack.current.push({ kind: 'deleteGate', id: gate.id })
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
    undoStack.current.push({ kind: 'assignSlot', formationId: formation.id, slotId: slot.id, agentId: slot.agentId || '', harness: slot.harness || '' })
    void patchBoard({ assignSlot: { formationId: formation.id, slotId: slot.id, agentId, harness } })
  }, [patchBoard])

  const unassignSlot = useCallback((formation: FormationNode, slot: FormationSlot) => {
    if (!slot.agentId) return
    assignSlot(formation, slot, '', '')
  }, [assignSlot])

  const makeControllerOp = useCallback((formation: FormationNode, slot: FormationSlot) => {
    const previous = formation.slots.find(item => item.controller)
    if (previous && previous.id !== slot.id) {
      undoStack.current.push({ kind: 'makeController', formationId: formation.id, slotId: previous.id })
    }
    void patchBoard({ makeController: { formationId: formation.id, slotId: slot.id } })
  }, [patchBoard])

  const createFormationAt = useCallback(async (type: FormationNode['type'], title: string, x: number, y: number): Promise<FormationNode | null> => {
    const before = boardRef.current
    const result = await patchBoard({ createFormation: { type, title, x: Math.round(x), y: Math.round(y) } })
    if (!before || !result) return null
    const created = findAddedByID(before.formations || [], result.board.formations || [])
    if (created) undoStack.current.push({ kind: 'deleteFormation', id: created.id })
    return created ?? null
  }, [patchBoard])

  const createMissionAt = useCallback(async (x: number, y: number) => {
    const before = boardRef.current
    const result = await patchBoard({ createMission: { title: 'New mission', goal: '', x: Math.round(x), y: Math.round(y) } })
    if (!before || !result) return
    const created = findAddedByID(before.missions || [], result.board.missions || [])
    if (created) undoStack.current.push({ kind: 'deleteMission', id: created.id })
  }, [patchBoard])

  const deleteFormationOp = useCallback((formation: FormationNode) => {
    void patchBoard({ deleteFormation: { id: formation.id } })
  }, [patchBoard])

  const deleteGateOp = useCallback((gate: GateNode) => {
    void patchBoard({ deleteGate: { id: gate.id } })
  }, [patchBoard])

  const deleteMissionOp = useCallback((mission: MissionNode) => {
    void patchBoard({ deleteMission: { id: mission.id } })
  }, [patchBoard])

  const addPortOp = useCallback(async (formation: FormationNode, direction: 'in' | 'out') => {
    const before = boardRef.current?.formations.find(item => item.id === formation.id)
    const result = await patchBoard({ addPort: { formationId: formation.id, direction, label: direction === 'in' ? 'Input' : 'Output' } })
    if (!before || !result) return
    const after = result.board.formations.find(item => item.id === formation.id)
    if (!after) return
    const created = direction === 'in'
      ? findAddedByID(before.inputs || [], after.inputs || [])
      : findAddedByID(before.outputs || [], after.outputs || [])
    if (created) undoStack.current.push({ kind: 'removePort', formationId: formation.id, portId: created.id })
  }, [patchBoard])

  const removePortOp = useCallback((formation: FormationNode, portId: string) => {
    void patchBoard({ removePort: { formationId: formation.id, portId } })
  }, [patchBoard])

  const removeVerificationOp = useCallback((formation: FormationNode) => {
    const previous = formation.verification
    if (previous) {
      undoStack.current.push({
        kind: 'setVerification',
        formationId: formation.id,
        kinds: previous.kinds || [],
        criterion: previous.criterion || '',
        onFail: previous.onFail || 'block',
      })
    }
    void patchBoard({ removeVerification: { formationId: formation.id } })
  }, [patchBoard])

  const openVerificationEditor = useCallback((formation: FormationNode) => {
    setMenu(null)
    setVerificationEditor({
      formationId: formation.id,
      title: formation.title,
      kinds: formation.verification?.kinds?.length ? [...formation.verification.kinds] : ['code'],
      criterion: formation.verification?.criterion || '',
      onFail: formation.verification?.onFail === 'pushback' ? 'pushback' : 'block',
    })
  }, [])

  const saveVerificationEditor = useCallback(async () => {
    if (!verificationEditor) return
    const formation = boardRef.current?.formations.find(item => item.id === verificationEditor.formationId)
    if (formation?.verification) {
      undoStack.current.push({
        kind: 'setVerification',
        formationId: formation.id,
        kinds: formation.verification.kinds || [],
        criterion: formation.verification.criterion || '',
        onFail: formation.verification.onFail || 'block',
      })
    } else if (formation) {
      undoStack.current.push({ kind: 'removeVerification', formationId: formation.id })
    }
    await patchBoard({
      setVerification: {
        formationId: verificationEditor.formationId,
        kinds: verificationEditor.kinds.length ? verificationEditor.kinds : ['code'],
        criterion: verificationEditor.criterion.trim(),
        onFail: verificationEditor.onFail,
      },
    })
    setVerificationEditor(null)
  }, [patchBoard, verificationEditor])

  /** Reconstruct the gate's judge chain from persisted `<gateId>:judge` edges. */
  const judgeChainOf = useCallback((gateId: string): string[] => {
    const connections = boardRef.current?.connections || []
    const socket = `${gateId}:judge`
    const send = connections.find(connection => connection.from === socket)
    if (!send) return []
    const start = send.to.split(':')[0]
    const visit = (node: string, acc: string[]): string[] | null => {
      const nextAcc = [...acc, node]
      for (const connection of connections) {
        if (connection.from.split(':')[0] !== node) continue
        if (connection.to === socket) return nextAcc
        const next = connection.to.split(':')[0]
        if (nextAcc.includes(next)) continue
        const found = visit(next, nextAcc)
        if (found) return found
      }
      return null
    }
    return visit(start, []) || [start]
  }, [])

  const attachJudge = useCallback((gate: GateNode, chain: string[]) => {
    const previous = judgeChainOf(gate.id)
    undoStack.current.push(previous.length
      ? { kind: 'setGateJudge', gateId: gate.id, chain: previous }
      : { kind: 'detachGateJudge', gateId: gate.id })
    void patchBoard({ setGateJudge: { gateId: gate.id, chain } })
  }, [judgeChainOf, patchBoard])

  const detachJudge = useCallback((gate: GateNode) => {
    const previous = judgeChainOf(gate.id)
    if (previous.length) undoStack.current.push({ kind: 'setGateJudge', gateId: gate.id, chain: previous })
    void patchBoard({ detachGateJudge: { gateId: gate.id } })
  }, [judgeChainOf, patchBoard])

  /** Drop on empty canvas / picker "new judge": create the formation, then wire it as judge. */
  const createJudgeFor = useCallback(async (gate: GateNode, type: FormationNode['type'], title: string, x: number, y: number) => {
    const created = await createFormationAt(type, title, x, y)
    if (created) attachJudge(gate, [created.id])
  }, [attachJudge, createFormationAt])

  const rewireSource = useCallback(async (connection: BoardConnection, newFrom: string) => {
    if (!newFrom || newFrom === connection.from || newFrom.split(':')[0] === connection.to.split(':')[0]) return
    const removed = await patchBoard({ unwireConnection: { from: connection.from, to: connection.to } })
    if (!removed) return
    const added = await patchBoard({ wireConnection: { from: newFrom, to: connection.to } })
    if (added) undoStack.current.push({ kind: 'rewireSource', previousFrom: connection.from, from: newFrom, to: connection.to })
  }, [patchBoard])

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

  // Native non-passive listener: React's onWheel is passive in the embedded
  // dashboard, so preventDefault there cannot stop the page from scrolling.
  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      const rect = viewport.getBoundingClientRect()
      const cursor = { x: event.clientX - rect.left, y: event.clientY - rect.top }
      setView(current => zoomTransform(current, event.deltaY < 0 ? 1.12 : 0.892, cursor))
    }
    viewport.addEventListener('wheel', onWheel, { passive: false })
    return () => viewport.removeEventListener('wheel', onWheel)
  }, [])

  const zoomBy = useCallback((factor: number) => {
    const rect = viewportRef.current?.getBoundingClientRect()
    const cursor = rect ? { x: rect.width / 2, y: rect.height / 2 } : undefined
    setView(current => zoomTransform(current, factor, cursor))
  }, [])

  const fitView = useCallback((options?: { smooth?: boolean }) => {
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
    const next = clampScale(Math.min(rect.width / (maxX - minX + pad * 2), rect.height / (maxY - minY + pad * 2)), 1)
    // The FIT button glides via the .world.smooth transition (reference feel);
    // the initial auto-fit on board load snaps so geometry is stable immediately.
    if (options?.smooth) {
      world.classList.add('smooth')
      window.setTimeout(() => world.classList.remove('smooth'), 420)
    }
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
        // 3px movement threshold so plain clicks don't jiggle cards (reference feel).
        if (!d.moved && Math.abs(event.clientX - d.startX) + Math.abs(event.clientY - d.startY) < 3) return
        d.moved = true
        const scale = viewRef.current.scale || 1
        const nx = d.originX + (event.clientX - d.startX) / scale
        const ny = d.originY + (event.clientY - d.startY) / scale
        setDragPos({ id: d.id, x: Math.round(nx), y: Math.round(ny) })
        return
      }
      if (laneDragRef.current) {
        const lane = laneDragRef.current
        lane.moved = true
        const w = screenToWorld(event.clientX, event.clientY)
        setLaneDraft({ connectionId: lane.connectionId, y: Math.round(w.y) })
        return
      }
      if (wireDragRef.current) {
        const active = wireDragRef.current
        const w = screenToWorld(event.clientX, event.clientY)
        if (active.kind === 'judge') {
          if (!active.moved && Math.abs(event.clientX - active.startX) + Math.abs(event.clientY - active.startY) < 4) return
          active.moved = true
          setTempWire(prev => (prev ? { ...prev, ax: w.x, ay: w.y } : prev))
          const card = (document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null)?.closest<HTMLElement>('.formation')
          setJudgeHover(card?.dataset.node && card.dataset.node !== active.gate.id ? card.dataset.node : null)
          return
        }
        setTempWire(prev => (prev ? (prev.moving === 'a' ? { ...prev, ax: w.x, ay: w.y } : { ...prev, bx: w.x, by: w.y }) : prev))
        if (active.kind === 'reconnect-source') {
          const outPort = findOutputPortAt(event.clientX, event.clientY)
          setHoverPort(outPort?.dataset.portOut ?? null)
        } else {
          const inPort = findInputPortAt(event.clientX, event.clientY)
          setHoverPort(inPort?.dataset.portIn ?? (inPort?.dataset.gateJudgeSocket ? `${inPort.dataset.gateJudgeSocket}:judge` : null))
        }
        return
      }
      if (gateDragRef.current) {
        setGateGhost({ x: event.clientX, y: event.clientY })
        return
      }
      if (staffRef.current) {
        const staff = staffRef.current
        if (!staff.moved && Math.abs(event.clientX - staff.startX) + Math.abs(event.clientY - staff.startY) >= 6) staff.moved = true
        setGhost({ x: event.clientX, y: event.clientY, agentId: staff.agentId })
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
          undoStack.current.push({ kind: 'moveNode', id: d.id, x: d.originX, y: d.originY })
          void persistPosition(d.id, Math.round(d.originX + (event.clientX - d.startX) / scale), Math.round(d.originY + (event.clientY - d.startY) / scale))
        }
        setDragPos(null)
      }
      if (laneDragRef.current) {
        const lane = laneDragRef.current
        laneDragRef.current = null
        if (lane.moved) {
          const w = screenToWorld(event.clientX, event.clientY)
          undoStack.current.push({ kind: 'setLane', edgeId: lane.connectionId, lane: lane.previousLane })
          void patchLayoutEdge(lane.connectionId, `y:${Math.round(w.y)}`).then(() => setLaneDraft(null))
        } else {
          setLaneDraft(null)
        }
      }
      if (wireDragRef.current) {
        const active = wireDragRef.current
        wireDragRef.current = null
        setTempWire(null)
        setHoverPort(null)
        setHiddenWireId(null)
        const hoveredJudge = judgeHoverRef.current
        setJudgeHover(null)
        if (active.kind === 'judge') {
          const gate = active.gate
          if (!active.moved) {
            openJudgePickerRef.current?.(gate, event.clientX, event.clientY)
          } else if (hoveredJudge) {
            attachJudge(gate, [hoveredJudge])
          } else {
            // Dropped on empty canvas inside the viewport → spawn a judge there (reference just-works).
            const rect = viewportRef.current?.getBoundingClientRect()
            const overCard = (event.target as HTMLElement | null)?.closest?.('.formation,.gatecard,.missioncard,.ctxmenu,.pop')
            const inView = rect && event.clientX > rect.left && event.clientX < rect.right && event.clientY > rect.top && event.clientY < rect.bottom
            if (!overCard && inView) {
              const w = screenToWorld(event.clientX, event.clientY)
              void createJudgeFor(gate, 'solo', 'Judge', w.x - 100, w.y - 58)
            }
          }
        } else if (active.kind === 'reconnect-source') {
          const outPort = findOutputPortAt(event.clientX, event.clientY)
          if (outPort?.dataset.portOut) void rewireSource(active.connection, outPort.dataset.portOut)
        } else {
          const target = findInputPortAt(event.clientX, event.clientY)
          const judgeGateId = target?.dataset.gateJudgeSocket
          if (judgeGateId) {
            // Dropping an output onto a gate's judge socket makes it the judge (reference setJudgeReturn).
            const gate = (boardRef.current?.gates || []).find(item => item.id === judgeGateId)
            const fromEndpoint = active.kind === 'new' ? active.from : active.connection.from
            const fromNodeId = fromEndpoint.split(':')[0]
            const isFormation = boardRef.current?.formations.some(item => item.id === fromNodeId)
            if (gate && isFormation) {
              if (active.kind === 'reconnect-target') removeWire(active.connection)
              attachJudge(gate, [fromNodeId])
            }
          } else if (target?.dataset.portIn) {
            if (active.kind === 'new') wire(active.from, target.dataset.portIn)
            else rewireTarget(active.connection, target.dataset.portIn)
          }
        }
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
        } else if (staff.fromSlot && staff.moved) {
          // Dragging an agent out of its slot onto anything that isn't a slot unassigns it (reference).
          const f = boardRef.current?.formations.find(item => item.id === staff.fromSlot?.formationId)
          const s = f?.slots.find(item => item.id === staff.fromSlot?.slotId)
          if (f && s) unassignSlot(f, s)
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
  }, [assignSlot, attachJudge, createGateAt, createJudgeFor, patchLayoutEdge, persistPosition, removeWire, rewireSource, rewireTarget, screenToWorld, unassignSlot, wire])

  const beginStaff = useCallback((event: ReactPointerEvent, agentId: string, harness: string, fromSlot?: DragStaff['fromSlot']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    staffRef.current = { agentId, harness, fromSlot, startX: event.clientX, startY: event.clientY, moved: false }
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

  const endpointWorldCenter = useCallback((endpoint: string, direction: 'out' | 'in') => {
    const world = worldRef.current
    if (!world) return null
    const escaped = endpoint.replace(/["\\]/g, '\\$&')
    const [nodeId, portId] = endpoint.split(':')
    const el = portId === 'judge'
      ? world.querySelector<HTMLElement>(`[data-gate-judge-socket="${nodeId.replace(/["\\]/g, '\\$&')}"]`)
      : world.querySelector<HTMLElement>(`[data-port-${direction}="${escaped}"]`)
    if (!el) return null
    const wr = world.getBoundingClientRect()
    const r = el.getBoundingClientRect()
    const s = viewRef.current.scale || 1
    return { x: (r.left + r.width / 2 - wr.left) / s, y: (r.top + r.height / 2 - wr.top) / s }
  }, [])

  const portWorldCenter = useCallback((endpoint: string) => endpointWorldCenter(endpoint, 'out'), [endpointWorldCenter])

  const beginWire = useCallback((event: ReactPointerEvent, endpoint: string, kind: WirePath['kind']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    wireDragRef.current = { kind: 'new', from: endpoint, wireKind: kind }
    const start = portWorldCenter(endpoint)
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind, moving: 'b' } : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'b' })
  }, [portWorldCenter, screenToWorld])

  const beginReconnect = useCallback((event: ReactPointerEvent<HTMLElement> | ReactMouseEvent<HTMLElement>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (wireDragRef.current) return
    event.stopPropagation()
    wireDragRef.current = { kind: 'reconnect-target', connection }
    setHiddenWireId(connection.id)
    const kind = connectionKind(connection)
    const start = endpointWorldCenter(connection.from, 'out')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start
      ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind, moving: 'b' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'b' })
  }, [endpointWorldCenter, screenToWorld])

  const beginReconnectSource = useCallback((event: ReactPointerEvent<Element> | ReactMouseEvent<Element>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (wireDragRef.current) return
    event.stopPropagation()
    wireDragRef.current = { kind: 'reconnect-source', connection }
    setHiddenWireId(connection.id)
    const kind = connectionKind(connection)
    const fixed = endpointWorldCenter(connection.to, 'in')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(fixed
      ? { ax: w.x, ay: w.y, bx: fixed.x, by: fixed.y, kind, moving: 'a' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'a' })
  }, [endpointWorldCenter, screenToWorld])

  const beginJudgeDrag = useCallback((event: ReactPointerEvent<HTMLElement>, gate: GateNode) => {
    if (event.button !== 0) return
    if (wireDragRef.current) return
    event.stopPropagation()
    event.preventDefault()
    wireDragRef.current = { kind: 'judge', gate, moved: false, startX: event.clientX, startY: event.clientY }
    const start = endpointWorldCenter(`${gate.id}:judge`, 'out')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start
      ? { ax: w.x, ay: w.y, bx: start.x, by: start.y, kind: 'judge', moving: 'a' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind: 'judge', moving: 'a' })
  }, [endpointWorldCenter, screenToWorld])

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

  /** Grab a committed wire: near an ENDPOINT reconnects that end; the MIDDLE hand-routes its lane (reference startWireDrag). */
  const beginWireDrag = useCallback((event: ReactPointerEvent<SVGPathElement>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (wireDragRef.current || laneDragRef.current) return
    event.stopPropagation()
    const a = endpointWorldCenter(connection.from, 'out')
    const b = endpointWorldCenter(connection.to, 'in')
    const w = screenToWorld(event.clientX, event.clientY)
    if (a && b) {
      const near = 70 // world px
      const dFrom = Math.hypot(w.x - a.x, w.y - a.y)
      const dTo = Math.hypot(w.x - b.x, w.y - b.y)
      if (dTo <= dFrom && dTo < near) return beginReconnect(event as unknown as ReactPointerEvent<HTMLElement>, connection)
      if (dFrom < dTo && dFrom < near) return beginReconnectSource(event, connection)
    }
    const previousLane = layoutRef.current?.edges?.find(edge => edge.id === connection.id)?.lane || 'auto'
    laneDragRef.current = { connectionId: connection.id, previousLane, moved: false }
  }, [beginReconnect, beginReconnectSource, endpointWorldCenter, screenToWorld])

  const beginGateToken = useCallback((event: ReactPointerEvent) => {
    if (event.button !== 0) return
    event.preventDefault()
    gateDragRef.current = true
    setGateGhost({ x: event.clientX, y: event.clientY })
  }, [])

  const gateHasJudge = useCallback((gateId: string): boolean => {
    return (boardRef.current?.connections || []).some(connection =>
      connection.from === `${gateId}:judge` || connection.to === `${gateId}:judge`)
  }, [])

  const formationMenu = useCallback((event: ReactMouseEvent<HTMLElement>, formation: FormationNode) => {
    openMenu(event, 'Formation actions', [
      { label: 'Run formation', action: () => void runFormation(formation) },
      { label: 'Set input', action: () => openBriefEditor(formation) },
      { label: 'Add input port', action: () => void addPortOp(formation, 'in') },
      { label: 'Add output port', action: () => void addPortOp(formation, 'out') },
      { label: formation.verification ? 'Configure verification' : 'Add verification', action: () => openVerificationEditor(formation) },
      ...(formation.verification ? [{ label: 'Remove verification', destructive: true, action: () => removeVerificationOp(formation) }] : []),
      { label: 'Delete formation', destructive: true, action: () => deleteFormationOp(formation) },
    ])
  }, [addPortOp, deleteFormationOp, openBriefEditor, openMenu, openVerificationEditor, removeVerificationOp, runFormation])

  const slotMenu = useCallback((event: ReactMouseEvent<HTMLElement>, formation: FormationNode, slot: FormationSlot) => {
    const items: MenuItem[] = []
    if (slot.agentId) {
      items.push({ label: `Unassign ${slot.agentId}`, action: () => unassignSlot(formation, slot) })
    }
    if (formation.type === 'orchestrated' && !slot.controller) {
      items.push({ label: 'Make controller', action: () => makeControllerOp(formation, slot) })
    }
    const assignable = agents.filter(agent => agent.id !== slot.agentId)
    if (assignable.length) {
      items.push({ label: 'Assign agent', head: true })
      for (const agent of assignable) {
        items.push({ label: agent.displayName || agent.id, action: () => assignSlot(formation, slot, agent.id, agent.harnessDefault || '') })
      }
    } else if (!items.length) {
      items.push({ label: 'No agents on the socket', disabled: true })
    }
    openMenu(event, `Slot · ${slot.label}`, items)
  }, [agents, assignSlot, makeControllerOp, openMenu, unassignSlot])

  const wireMenu = useCallback((event: ReactMouseEvent<SVGPathElement>, connection: BoardConnection) => {
    openMenu(event, 'Connection actions', [
      { label: 'Reset routing', action: () => void patchLayoutEdge(connection.id, 'auto') },
      { label: 'Remove connection', destructive: true, action: () => removeWire(connection) },
    ])
  }, [openMenu, patchLayoutEdge, removeWire])

  const openJudgePicker = useCallback((gate: GateNode, x: number, y: number) => {
    const pos = displayLayoutByNode.get(gate.id)
    const gateX = pos?.x ?? 200
    const gateY = pos?.y ?? 200
    const items: MenuItem[] = [
      { label: 'Judge with a NEW formation', head: true },
      { label: 'Solo · 1 agent', action: () => void createJudgeFor(gate, 'solo', 'Judge', gateX, gateY - 200) },
      { label: 'Peer · 2 equals', action: () => void createJudgeFor(gate, 'peer', 'Judge panel', gateX, gateY - 200) },
      { label: 'Orchestrated · controller', action: () => void createJudgeFor(gate, 'orchestrated', 'Judge desk', gateX, gateY - 200) },
    ]
    const formations = boardRef.current?.formations || []
    if (formations.length) {
      items.push({ label: '…or an existing formation', head: true })
      for (const formation of formations) {
        items.push({ label: formation.title, action: () => attachJudge(gate, [formation.id]) })
      }
    }
    if (gateHasJudge(gate.id)) {
      items.push({ label: 'Detach judge', destructive: true, action: () => detachJudge(gate) })
    }
    setMenu({ label: 'Judge', x, y, items })
  }, [attachJudge, createJudgeFor, detachJudge, displayLayoutByNode, gateHasJudge])
  openJudgePickerRef.current = openJudgePicker

  const gateMenu = useCallback((event: ReactMouseEvent<HTMLElement>, gate: GateNode) => {
    openMenu(event, 'Gate actions', [
      { label: gateHasJudge(gate.id) ? 'Change judge…' : 'Attach judge…', action: () => openJudgePicker(gate, event.clientX, event.clientY) },
      ...(gateHasJudge(gate.id) ? [{ label: 'Detach judge', action: () => detachJudge(gate) }] : []),
      { label: 'Delete gate', destructive: true, action: () => deleteGateOp(gate) },
    ])
  }, [deleteGateOp, detachJudge, gateHasJudge, openJudgePicker, openMenu])

  const missionMenu = useCallback((event: ReactMouseEvent<HTMLElement>, mission: MissionNode) => {
    openMenu(event, 'Mission actions', [
      { label: 'Start mission', action: () => void runMission(mission) },
      { label: 'Delete mission', destructive: true, action: () => deleteMissionOp(mission) },
    ])
  }, [deleteMissionOp, openMenu, runMission])

  const inputRowMenu = useCallback((event: ReactMouseEvent<HTMLElement>, formation: FormationNode, portId: string, incoming?: BoardConnection) => {
    openMenu(event, 'Input port', [
      ...(incoming ? [{ label: 'Disconnect input', action: () => removeWire(incoming) }] : []),
      { label: 'Add input port', action: () => void addPortOp(formation, 'in') },
      { label: 'Remove this input', destructive: true, action: () => removePortOp(formation, portId) },
    ])
  }, [addPortOp, openMenu, removePortOp, removeWire])

  const outputRowMenu = useCallback((event: ReactMouseEvent<HTMLElement>, formation: FormationNode, portId: string) => {
    openMenu(event, 'Output port', [
      { label: 'Add output port', action: () => void addPortOp(formation, 'out') },
      { label: 'Remove this output', destructive: true, action: () => removePortOp(formation, portId) },
    ])
  }, [addPortOp, openMenu, removePortOp])

  const canvasMenu = useCallback((event: ReactMouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement
    if (target.closest('.formation,.gatecard,.missioncard,.ctxmenu,.pop,.run-banner,.zoomctl')) return
    if ((target as Element).closest?.('path')) return
    event.preventDefault()
    event.stopPropagation()
    const w = screenToWorld(event.clientX, event.clientY)
    setMenu({
      label: 'New',
      x: event.clientX,
      y: event.clientY,
      items: [
        { label: 'Mission', action: () => void createMissionAt(w.x, w.y) },
        { label: 'Solo formation', action: () => void createFormationAt('solo', 'New formation', w.x, w.y) },
        { label: 'Peer formation', action: () => void createFormationAt('peer', 'New peers', w.x, w.y) },
        { label: 'Flow formation', action: () => void createFormationAt('flow', 'New flow', w.x, w.y) },
        { label: 'Orchestrated formation', action: () => void createFormationAt('orchestrated', 'New desk', w.x, w.y) },
        { label: 'Gate', action: () => void createGateAt(w.x, w.y) },
      ],
    })
  }, [createFormationAt, createGateAt, createMissionAt, screenToWorld])

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
        onContextMenu={event => slotMenu(event, formation, slot)}
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

        <div className="viewport" data-testid="formations-canvas" ref={viewportRef} onPointerDownCapture={captureConnectedInputDrag} onPointerDown={onViewportPointerDown} onContextMenu={canvasMenu}>
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
                      onPointerDown={event => beginWireDrag(event, connection)}
                      onContextMenu={event => wireMenu(event, connection)}
                    />
                    <path
                      className={`wire ${path.kind}${path.flowing ? ' flowing' : ''}`}
                      data-testid={`formation-wire-${path.id}`}
                      d={path.d}
                      onPointerDown={event => beginWireDrag(event, connection)}
                      onContextMenu={event => wireMenu(event, connection)}
                    />
                  </g>
                )
              })}
              {tempWire ? (
                <path
                  className={`wire temp${tempWire.kind !== 'wire' ? ` ${tempWire.kind}` : ''}`}
                  d={`M ${tempWire.ax} ${tempWire.ay} C ${tempWire.ax + 50} ${tempWire.ay}, ${tempWire.bx - 50} ${tempWire.by}, ${tempWire.bx} ${tempWire.by}`}
                />
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
                  onContextMenu={event => missionMenu(event, mission)}
                >
                  <div className="mhd">
                    <span className="meyebrow">◆ Mission</span>
                    <button className="mrun" title="Start mission" onClick={() => void runMission(mission)} data-testid={`run-mission-${mission.id}`}>{PLAY_SVG}</button>
                  </div>
                  <div className="mtitle">{mission.title}</div>
                  <div className={`mgoal${mission.goal ? '' : ' placeholder'}`}>{mission.goal || 'set the mission objective…'}</div>
                  <div className="mstatus">{state ? state : ''}</div>
                  <span className={`port pout ready${hoverPort === `${mission.id}:out` ? ' snaptarget' : ''}`} data-port-out={`${mission.id}:out`} title="Starts the chain — drag to a step" onPointerDown={event => beginWire(event, `${mission.id}:out`, 'wire')} />
                </div>
              )
            })}

            {(board?.formations || []).map((formation, index) => {
              const pos = positionOf(formation.id, index + (board?.missions?.length || 0))
              const state = nodeStates.get(formation.id)
              return (
                <div
                  key={formation.id}
                  className={`formation type-${formation.type}${state === 'running' ? ' running' : ''}${judgeHover === formation.id ? ' judgehover' : ''}`}
                  data-node={formation.id}
                  data-testid={`formation-node-${formation.id}`}
                  style={{ left: pos.x, top: pos.y }}
                  onContextMenu={event => formationMenu(event, formation)}
                >
                  {formation.inputs.map((port, portIndex) => {
                    const endpoint = `${formation.id}:${port.id}`
                    const incoming = (board?.connections || []).find(connection => connection.to === endpoint)
                    return (
                      <div
                        className={`fio in${portIndex === 0 ? ' brief' : ''}`}
                        key={port.id}
                        onPointerDown={event => beginNodeDrag(event, formation.id, index)}
                        onContextMenu={event => inputRowMenu(event, formation, port.id, incoming)}
                      >
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
                  <div
                    className={`verify-band${formation.verification ? '' : ' empty'}`}
                    data-gate={formation.verification?.id}
                    data-testid={`verify-band-${formation.id}`}
                    onClick={() => openVerificationEditor(formation)}
                    onContextMenu={event => openMenu(event, 'Verification', [
                      { label: formation.verification ? 'Configure verification' : 'Add verification', action: () => openVerificationEditor(formation) },
                      ...(formation.verification ? [{ label: 'Remove verification', destructive: true, action: () => removeVerificationOp(formation) }] : []),
                    ])}
                  >
                    <span className="vico"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9"><path d="M12 3l7 3v5c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6z" /><path d="M9 12l2 2 4-4" /></svg></span>
                    <span className="vlabel">{formation.verification ? 'verify' : '+ verify'}</span>
                    {formation.verification ? <span className="vkinds">{formation.verification.kinds.join(' · ')} · {formation.verification.criterion}</span> : null}
                  </div>
                  {formation.outputs.map((port, portIndex) => {
                    const endpoint = `${formation.id}:${port.id}`
                    return (
                      <div className="fio out" key={port.id} onContextMenu={event => outputRowMenu(event, formation, port.id)}>
                        <span className="glyph">out</span>
                        <span className="io-status idle">{portIndex === 0 ? 'no output yet' : port.label.toLowerCase()}</span>
                        <span className={`port pout ready${hoverPort === endpoint ? ' snaptarget' : ''}`} data-port-out={endpoint} title="Drag to a downstream input" onPointerDown={event => beginWire(event, endpoint, 'wire')} />
                      </div>
                    )
                  })}
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
                  className={`gatecard${state ? ` ${state}` : ''}${gateHasJudge(gate.id) ? ' hasjudge' : ''}`}
                  data-node={gate.id}
                  data-gate={gate.id}
                  data-testid={`gate-node-${gate.id}`}
                  style={{ left: pos.x, top: pos.y }}
                  onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}
                  onContextMenu={event => gateMenu(event, gate)}
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
                    className={`pjudge${hoverPort === `${gate.id}:judge` ? ' snaptarget' : ''}`}
                    data-testid={`gate-judge-socket-${gate.id}`}
                    data-gate-judge-socket={gate.id}
                    title="Judge with a formation — drag to attach, click to pick"
                    onPointerDown={event => beginJudgeDrag(event, gate)}
                  />
                  <span className="gico" onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}>{GATE_SVG}</span>
                  <span className="gmeta" onPointerDown={event => beginNodeDrag(event, gate.id, nodeIndex)}><span className="gt">{gate.title || gate.kinds.join(' · ') || 'Gate'}</span><span className="gs">{[gate.kinds.join(' · '), gate.criterion || 'work is accepted before it proceeds'].filter(Boolean).join(' · ')}</span></span>
                  <span className="glabel pass"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4"><path d="M4 12l5 5L20 6" /></svg>pass</span>
                  <span className="glabel fail"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4"><path d="M6 6l12 12M18 6L6 18" /></svg>fail</span>
                  <span className={`port pass${hoverPort === `${gate.id}:pass` ? ' snaptarget' : ''}`} data-port-out={`${gate.id}:pass`} title="On PASS → drag to the next step" onPointerDown={event => beginWire(event, `${gate.id}:pass`, 'pass')} />
                  <span className={`port fail${hoverPort === `${gate.id}:fail` ? ' snaptarget' : ''}`} data-port-out={`${gate.id}:fail`} title="On FAIL → drag to a fallback" onPointerDown={event => beginWire(event, `${gate.id}:fail`, 'fail')} />
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
            <button onClick={() => fitView({ smooth: true })} title="Fit">FIT</button>
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

      {verificationEditor ? (
        <div
          className="pop"
          role="dialog"
          aria-label={`Verification · ${verificationEditor.title}`}
          onPointerDown={event => event.stopPropagation()}
        >
          <div className="pop-head">
            <span className="pt">Verification · {verificationEditor.title}</span>
            <button className="x" type="button" aria-label="Close verification editor" onClick={() => setVerificationEditor(null)}>x</button>
          </div>
          <div className="pop-body">
            <label>Checks</label>
            <div className="chiprow">
              {(['code', 'human', 'formation'] as const).map(kind => (
                <label key={kind} className="kindchip">
                  <input
                    type="checkbox"
                    aria-label={`Verification kind ${kind}`}
                    checked={verificationEditor.kinds.includes(kind)}
                    onChange={event => setVerificationEditor(current => {
                      if (!current) return current
                      const kinds = event.target.checked
                        ? [...current.kinds, kind]
                        : current.kinds.filter(item => item !== kind)
                      return { ...current, kinds }
                    })}
                  />
                  {kind}
                </label>
              ))}
            </div>
            <label htmlFor="cockpit-verification-criterion">Criterion</label>
            <textarea
              id="cockpit-verification-criterion"
              aria-label={`Criterion for ${verificationEditor.title}`}
              value={verificationEditor.criterion}
              onChange={event => setVerificationEditor(current => current ? { ...current, criterion: event.target.value } : current)}
            />
            <label htmlFor="cockpit-verification-onfail">On fail</label>
            <select
              id="cockpit-verification-onfail"
              aria-label={`On fail for ${verificationEditor.title}`}
              value={verificationEditor.onFail}
              onChange={event => setVerificationEditor(current => current ? { ...current, onFail: event.target.value === 'pushback' ? 'pushback' : 'block' } : current)}
            >
              <option value="block">block — stop the run</option>
              <option value="pushback">pushback — return to agents with feedback</option>
            </select>
            <button className="save" type="button" onClick={() => void saveVerificationEditor()}>Save verification</button>
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
          {menu.items.map((item, itemIndex) => item.head ? (
            <div key={`${item.label}-${itemIndex}`} className="msection">{item.label}</div>
          ) : (
            <button
              key={`${item.label}-${itemIndex}`}
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
