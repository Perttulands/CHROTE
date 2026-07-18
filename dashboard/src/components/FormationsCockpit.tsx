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
import { clampScale, displayLayoutFor, fallbackNodePosition, freeGridPosition, snapToGrid, zoomTransform } from './formationsCanvas'
import { GATE_SVG, PLAY_SVG, TYPE_TAG, agentRole, agentState, harnessGlyph, initials } from './formationsCockpitVisuals'
import { useSessionOptional } from '../context/SessionContext'
import { resolveFormationsTextSize } from '../types'
import { connectionKind, findInputPortAt, findOutputPortAt, isTextEditingTarget, laneYFrom, splitList } from './formationsCockpitDom'
import { routeJudgeWire, routeOrthoWire } from './formationsRouting'
import type { ObstacleRect } from './formationsRouting'
import { findAddedByID } from './formationsBoardModel'
import { isSafeBeadsIssueID } from './formationsBeadId'
import { createFormationsInteractionOwner } from './formationsInteraction'
import type { FormationsInteractionOwner } from './formationsInteraction'
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
type LegacyVerificationState = {
  boardSlug: string
  boardRev: number
  boardETag: string
  formationId: string
  verificationId: string
  title: string
  kinds: string[]
  criterion: string
  onFail: string
  replacementGateIds: string[]
  replacementGateId: string
}
type MissionEditorState = { title: string; goal: string; beadId: string; x: number; y: number }
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
  | { kind: 'moveNodes'; nodes: { id: string; x: number; y: number }[] }
  | { kind: 'setLane'; edgeId: string; lane: string }
  | { kind: 'setGateJudge'; gateId: string; chain: string[] }
  | { kind: 'detachGateJudge'; gateId: string }
  | { kind: 'removePort'; formationId: string; portId: string }
  | { kind: 'makeController'; formationId: string; slotId: string }

export default function FormationsCockpit({ active = true }: { active?: boolean } = {}) {
  const session = useSessionOptional()
  const textSize = resolveFormationsTextSize(session?.settings.formationsTextSize)
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [selectedSlug, setSelectedSlug] = useState('')
  const [board, setBoard] = useState<BoardDocument | null>(null)
  const [layout, setLayout] = useState<LayoutDocument | null>(null)
  const [agents, setAgents] = useState<AgentProjection[]>([])
  const [view, setView] = useState<ViewTransform>({ x: 40, y: 40, scale: 1 })
  const [error, setError] = useState('')
  const [activeRun, setActiveRun] = useState<RunStatusProjection | null>(null)
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
  const [ghost, setGhost] = useState<{ x: number; y: number; agentId: string; harness?: string } | null>(null)
  const [hoverSlot, setHoverSlot] = useState<string | null>(null)
  const [dragPos, setDragPos] = useState<{ id: string; x: number; y: number } | null>(null)
  const [wires, setWires] = useState<WirePath[]>([])
  const [geometryTick, setGeometryTick] = useState(0)
  const [tempWire, setTempWire] = useState<{ ax: number; ay: number; bx: number; by: number; kind: WirePath['kind']; moving: 'a' | 'b' } | null>(null)
  const [hoverPort, setHoverPort] = useState<string | null>(null)
  const [gateGhost, setGateGhost] = useState<{ x: number; y: number } | null>(null)
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [missionEditor, setMissionEditor] = useState<MissionEditorState | null>(null)
  const [missionEditorError, setMissionEditorError] = useState('')
  const [missionEditorSaving, setMissionEditorSaving] = useState(false)
  const [briefEditor, setBriefEditor] = useState<BriefEditorState | null>(null)
  const [legacyVerification, setLegacyVerification] = useState<LegacyVerificationState | null>(null)
  const [hiddenWireId, setHiddenWireId] = useState<string | null>(null)
  const [judgeHover, setJudgeHover] = useState<string | null>(null)
  const [laneDraft, setLaneDraft] = useState<{ connectionId: string; y: number } | null>(null)

  const boardRef = useRef<BoardDocument | null>(null)
  const layoutRef = useRef<LayoutDocument | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const worldRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<ViewTransform>(view)
  const interactionOwnerRef = useRef<FormationsInteractionOwner | null>(null)
  if (!interactionOwnerRef.current) interactionOwnerRef.current = createFormationsInteractionOwner()
  const interactionOwner = interactionOwnerRef.current
  const undoStack = useRef<CockpitUndo[]>([])
  const fittedBoardRef = useRef<string | null>(null)
  const judgeHoverRef = useRef<string | null>(null)
  const openJudgePickerRef = useRef<((gate: GateNode, x: number, y: number) => void) | null>(null)

  viewRef.current = view
  useEffect(() => { boardRef.current = board }, [board])
  useEffect(() => { layoutRef.current = layout }, [layout])
  useEffect(() => { judgeHoverRef.current = judgeHover }, [judgeHover])

  useEffect(() => { setLegacyVerification(null) }, [selectedSlug])

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
        boardRef.current = null
        layoutRef.current = null
        setBoards([])
        setSelectedSlug('')
        setBoard(null)
        setLayout(null)
        setActiveRun(null)
        setRunEvents([])
      })
      .catch(err => !cancelled && setError(err instanceof Error ? err.message : 'Failed to load boards'))
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!selectedSlug) return
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
    if (!active || !selectedSlug || !board?.etag) return
    let cancelled = false
    const checkChanges = async () => {
      try {
        const changed = await fetchBoardChanged(selectedSlug, board.etag)
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
  }, [active, board?.etag, selectedSlug])

  useEffect(() => {
    if (!active) return
    let cancelled = false
    const load = () => fetchAgents().then(list => !cancelled && setAgents(list)).catch(() => undefined)
    load()
    const timer = window.setInterval(load, 8000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [active])

  useEffect(() => {
    if (!selectedSlug) return
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

  const placementForNewNode = useCallback((x: number, y: number) => (
    freeGridPosition({ x, y }, [...displayLayoutByNode.values()])
  ), [displayLayoutByNode])

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
  }, [board, layout, view, dragPos, agents, nodeStates, hiddenWireId, laneDraft, geometryTick])

  // Cards ease into place (left/top transitions) and FIT glides the world
  // transform; wires are measured from the DOM mid-flight, so re-measure once
  // motion settles or the endpoints stay visually stale.
  useEffect(() => {
    const world = worldRef.current
    if (!world) return
    const onTransitionEnd = (event: TransitionEvent) => {
      if (event.propertyName === 'left' || event.propertyName === 'top' || event.propertyName === 'transform') {
        setGeometryTick(tick => tick + 1)
      }
    }
    world.addEventListener('transitionend', onTransitionEnd)
    return () => world.removeEventListener('transitionend', onTransitionEnd)
  }, [])

  // ----- mutations -----
  const patchBoard = useCallback(async (patch: Record<string, unknown>): Promise<{ board: BoardDocument; layout: LayoutDocument | null } | null> => {
    const current = boardRef.current
    if (!current) return null
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
    try {
      const next = await patchBoardLayout(currentBoard.slug, currentLayout.etag, { edges: [{ id: edgeId, lane }] })
      layoutRef.current = next
      setLayout(next)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update wire routing')
    }
  }, [])

  const persistPositions = useCallback(async (nodes: { id: string; x: number; y: number }[]) => {
    const currentBoard = boardRef.current
    const currentLayout = layoutRef.current
    if (!currentBoard || !currentLayout || !nodes.length) return
    try {
      const next = await patchBoardLayout(currentBoard.slug, currentLayout.etag, { nodes })
      layoutRef.current = next
      setLayout(next)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save layout')
    }
  }, [])

  const persistPosition = useCallback(async (id: string, x: number, y: number) => {
    await persistPositions([{ id, x, y }])
  }, [persistPositions])

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
    if (action.kind === 'moveNodes') {
      await persistPositions(action.nodes)
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
      case 'removePort':
        patch = { removePort: { formationId: action.formationId, portId: action.portId } }
        break
      case 'makeController':
        patch = { makeController: { formationId: action.formationId, slotId: action.slotId } }
        break
    }
    const result = await patchBoard(patch)
    if (!result) retry()
  }, [patchBoard, patchLayoutEdge, persistPosition, persistPositions])

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
    const placement = placementForNewNode(worldX, worldY)
    const before = boardRef.current
    const result = await patchBoard({ createGate: { title: 'Review gate', kinds: ['code'], criterion: '', x: placement.x, y: placement.y } })
    if (!before || !result) return
    const gate = findAddedByID(before.gates || [], result.board.gates || [])
    if (gate) undoStack.current.push({ kind: 'deleteGate', id: gate.id })
  }, [patchBoard, placementForNewNode])

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
    const placement = placementForNewNode(x, y)
    const before = boardRef.current
    const result = await patchBoard({ createFormation: { type, title, x: placement.x, y: placement.y } })
    if (!before || !result) return null
    const created = findAddedByID(before.formations || [], result.board.formations || [])
    if (created) undoStack.current.push({ kind: 'deleteFormation', id: created.id })
    return created ?? null
  }, [patchBoard, placementForNewNode])

  const createMissionAt = useCallback((x: number, y: number) => {
    setMissionEditor({ title: 'New mission', goal: '', beadId: '', x, y })
    setMissionEditorError('')
    setMissionEditorSaving(false)
  }, [])

  const closeMissionEditor = useCallback(() => {
    if (missionEditorSaving) return
    setMissionEditor(null)
    setMissionEditorError('')
  }, [missionEditorSaving])

  const saveMissionEditor = useCallback(async () => {
    if (!missionEditor || missionEditorSaving) return
    const beadId = missionEditor.beadId.trim()
    if (!isSafeBeadsIssueID(beadId)) {
      setMissionEditorError('Enter a Beads issue ID such as ctx-ug7.25.')
      return
    }
    const placement = placementForNewNode(missionEditor.x, missionEditor.y)
    const before = boardRef.current
    if (!before) return
    setMissionEditorError('')
    setMissionEditorSaving(true)
    const result = await patchBoard({
      createMission: {
        title: missionEditor.title.trim() || 'New mission',
        goal: missionEditor.goal.trim(),
        beadId,
        x: placement.x,
        y: placement.y,
      },
    })
    setMissionEditorSaving(false)
    if (!result) return
    const created = findAddedByID(before.missions || [], result.board.missions || [])
    if (created) undoStack.current.push({ kind: 'deleteMission', id: created.id })
    setMissionEditor(null)
  }, [missionEditor, missionEditorSaving, patchBoard, placementForNewNode])

  useEffect(() => {
    if (!missionEditor || missionEditorSaving) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closeMissionEditor()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [closeMissionEditor, missionEditor, missionEditorSaving])

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

  const removeLegacyVerification = useCallback(async () => {
    if (!legacyVerification?.replacementGateId) return
    const current = boardRef.current
    if (!current || current.slug !== legacyVerification.boardSlug || current.rev !== legacyVerification.boardRev || current.etag !== legacyVerification.boardETag) {
      setError('Board changed while legacy verification was open. Reopen migration to continue.')
      return
    }
    const result = await patchBoard({
      removeVerification: {
        formationId: legacyVerification.formationId,
        replacementGateId: legacyVerification.replacementGateId,
      },
    })
    if (result) setLegacyVerification(null)
  }, [legacyVerification, patchBoard])

  const openLegacyVerification = useCallback((formation: FormationNode) => {
    const current = boardRef.current
    if (!formation.verification || !current) return
    const outputEndpoints = new Set(formation.outputs.map(output => `${formation.id}:${output.id}`))
    const replacementGateIds = (current.gates || [])
      .filter(gate => current.connections.some(connection => outputEndpoints.has(connection.from) && connection.to === `${gate.id}:in`))
      .map(gate => gate.id)
    setMenu(null)
    setLegacyVerification({
      boardSlug: current.slug,
      boardRev: current.rev,
      boardETag: current.etag,
      formationId: formation.id,
      verificationId: formation.verification.id || '',
      title: formation.title,
      kinds: formation.verification.kinds?.length ? [...formation.verification.kinds] : [],
      criterion: formation.verification.criterion || '',
      onFail: formation.verification.onFail || '',
      replacementGateIds,
      replacementGateId: replacementGateIds[0] || '',
    })
  }, [])

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
    // Land only the new card near the viewport center on free grid space.
    // Existing authored coordinates remain untouched.
    const rect = viewportRef.current?.getBoundingClientRect()
    const v = viewRef.current
    const center = rect
      ? { x: (rect.width / 2 - v.x) / (v.scale || 1) - 150, y: (rect.height / 2 - v.y) / (v.scale || 1) - 120 }
      : { x: 200, y: 200 }
    const placement = placementForNewNode(center.x, center.y)
    void patchBoard({
      createFormation: {
        type: 'solo',
        title: 'New formation',
        x: placement.x,
        y: placement.y,
      },
    })
  }, [patchBoard, placementForNewNode])

  // ----- pan + zoom -----
  const onViewportPointerDown = useCallback((event: ReactPointerEvent) => {
    if (event.button !== 0) return
    const target = event.target as HTMLElement
    if (target.closest('.formation,.gatecard,.missioncard,.zoomctl,.run-banner')) return
    const pan = { startX: event.clientX, startY: event.clientY, originX: viewRef.current.x, originY: viewRef.current.y }
    interactionOwner.begin({
      kind: 'pan',
      pointerId: event.pointerId,
      project: pointer => {
        setView(current => ({ ...current, x: pan.originX + (pointer.clientX - pan.startX), y: pan.originY + (pointer.clientY - pan.startY) }))
        viewportRef.current?.classList.add('panning')
      },
      finalize: () => undefined,
      cancel: () => viewportRef.current?.classList.remove('panning'),
    })
  }, [interactionOwner])

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

  const arrangeBoard = useCallback(async () => {
    const currentBoard = boardRef.current
    const currentLayout = layoutRef.current
    if (!currentBoard || !currentLayout) return
    const previous = currentLayout.nodes.map(node => ({ id: node.id, x: node.x, y: node.y }))
    undoStack.current.push({ kind: 'moveNodes', nodes: previous })
    try {
      const next = await patchBoardLayout(currentBoard.slug, currentLayout.etag, { arrange: true })
      layoutRef.current = next
      setLayout(next)
      setError('')
    } catch (err) {
      undoStack.current.pop()
      setError(err instanceof Error ? err.message : 'Failed to arrange layout')
    }
  }, [])

  const screenToWorld = useCallback((clientX: number, clientY: number) => {
    const rect = viewportRef.current?.getBoundingClientRect()
    const v = viewRef.current
    if (!rect) return { x: 0, y: 0 }
    return { x: (clientX - rect.left - v.x) / (v.scale || 1), y: (clientY - rect.top - v.y) / (v.scale || 1) }
  }, [])

  // ----- one global owner for pan, card/staff/wire/gate/lane interactions -----
  useLayoutEffect(() => {
    const onMove = (event: globalThis.PointerEvent) => { interactionOwner.project(event) }
    const onUp = (event: globalThis.PointerEvent) => { interactionOwner.finalize(event) }
    const onCancel = (event: globalThis.PointerEvent) => { interactionOwner.cancel(event.pointerId) }
    const onLostOwnership = () => { interactionOwner.cancel() }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
    window.addEventListener('lostpointercapture', onCancel)
    window.addEventListener('blur', onLostOwnership)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onCancel)
      window.removeEventListener('lostpointercapture', onCancel)
      window.removeEventListener('blur', onLostOwnership)
      interactionOwner.cancel()
    }
  }, [interactionOwner])

  const beginStaff = useCallback((event: ReactPointerEvent, agentId: string, harness: string, fromSlot?: DragStaff['fromSlot']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    const staff: DragStaff = { agentId, harness, fromSlot, startX: event.clientX, startY: event.clientY, moved: false }
    interactionOwner.begin({
      kind: 'staff',
      pointerId: event.pointerId,
      project: pointer => {
        if (!staff.moved && Math.abs(pointer.clientX - staff.startX) + Math.abs(pointer.clientY - staff.startY) >= 6) staff.moved = true
        setGhost({ x: pointer.clientX, y: pointer.clientY, agentId: staff.agentId, harness: staff.harness })
        const el = document.elementFromPoint(pointer.clientX, pointer.clientY) as HTMLElement | null
        const slotEl = el?.closest<HTMLElement>('.slot')
        setHoverSlot(slotEl ? `${slotEl.dataset.fid}:${slotEl.dataset.sid}` : null)
      },
      finalize: pointer => {
        const el = document.elementFromPoint(pointer.clientX, pointer.clientY) as HTMLElement | null
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
      },
      cancel: () => {
        setGhost(null)
        setHoverSlot(null)
      },
    })
    setGhost({ x: event.clientX, y: event.clientY, agentId, harness })
  }, [assignSlot, interactionOwner, unassignSlot])

  const beginNodeDrag = useCallback((event: ReactPointerEvent, id: string, index: number) => {
    if (event.button !== 0) return
    const target = event.target as HTMLElement
    if (target.closest('.port,.frun,.mrun')) return
    event.stopPropagation()
    const pos = positionOf(id, index)
    const drag: DragNode = { id, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, originX: pos.x, originY: pos.y, moved: false }
    interactionOwner.begin({
      kind: 'node',
      pointerId: event.pointerId,
      project: pointer => {
        // 3px movement threshold so plain clicks don't jiggle cards (reference feel).
        if (!drag.moved && Math.abs(pointer.clientX - drag.startX) + Math.abs(pointer.clientY - drag.startY) < 3) return
        if (!drag.moved) worldRef.current?.classList.add('nodedrag')
        drag.moved = true
        const scale = viewRef.current.scale || 1
        const nx = drag.originX + (pointer.clientX - drag.startX) / scale
        const ny = drag.originY + (pointer.clientY - drag.startY) / scale
        setDragPos({ id: drag.id, x: Math.round(nx), y: Math.round(ny) })
      },
      finalize: pointer => {
        if (!drag.moved) return
        const scale = viewRef.current.scale || 1
        undoStack.current.push({ kind: 'moveNode', id: drag.id, x: drag.originX, y: drag.originY })
        // Release snaps to the visible dot grid so hand-placed cards line up.
        void persistPosition(drag.id, snapToGrid(drag.originX + (pointer.clientX - drag.startX) / scale), snapToGrid(drag.originY + (pointer.clientY - drag.startY) / scale))
      },
      cancel: () => {
        worldRef.current?.classList.remove('nodedrag')
        setDragPos(null)
      },
    })
  }, [interactionOwner, persistPosition, positionOf])

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

  const ownWireDrag = useCallback((event: ReactPointerEvent<Element> | ReactMouseEvent<Element>, active: WireDrag) => {
    const pointerId = 'pointerId' in event ? event.pointerId : 1
    interactionOwner.begin({
      kind: 'wire',
      pointerId,
      project: pointer => {
        const w = screenToWorld(pointer.clientX, pointer.clientY)
        if (active.kind === 'judge') {
          if (!active.moved && Math.abs(pointer.clientX - active.startX) + Math.abs(pointer.clientY - active.startY) < 4) return
          active.moved = true
          setTempWire(prev => (prev ? { ...prev, ax: w.x, ay: w.y } : prev))
          const card = (document.elementFromPoint(pointer.clientX, pointer.clientY) as HTMLElement | null)?.closest<HTMLElement>('.formation')
          setJudgeHover(card?.dataset.node && card.dataset.node !== active.gate.id ? card.dataset.node : null)
          return
        }
        setTempWire(prev => (prev ? (prev.moving === 'a' ? { ...prev, ax: w.x, ay: w.y } : { ...prev, bx: w.x, by: w.y }) : prev))
        if (active.kind === 'reconnect-source') {
          const outPort = findOutputPortAt(pointer.clientX, pointer.clientY)
          setHoverPort(outPort?.dataset.portOut ?? null)
        } else {
          const inPort = findInputPortAt(pointer.clientX, pointer.clientY)
          setHoverPort(inPort?.dataset.portIn ?? (inPort?.dataset.gateJudgeSocket ? `${inPort.dataset.gateJudgeSocket}:judge` : null))
        }
      },
      finalize: pointer => {
        const hoveredJudge = judgeHoverRef.current
        if (active.kind === 'judge') {
          const gate = active.gate
          if (!active.moved) {
            openJudgePickerRef.current?.(gate, pointer.clientX, pointer.clientY)
          } else if (hoveredJudge) {
            attachJudge(gate, [hoveredJudge])
          } else {
            // Dropped on empty canvas inside the viewport → spawn a judge there (reference just-works).
            const rect = viewportRef.current?.getBoundingClientRect()
            const overCard = (pointer.target as HTMLElement | null)?.closest?.('.formation,.gatecard,.missioncard,.ctxmenu,.pop')
            const inView = rect && pointer.clientX > rect.left && pointer.clientX < rect.right && pointer.clientY > rect.top && pointer.clientY < rect.bottom
            if (!overCard && inView) {
              const w = screenToWorld(pointer.clientX, pointer.clientY)
              void createJudgeFor(gate, 'solo', 'Judge', w.x - 100, w.y - 58)
            }
          }
        } else if (active.kind === 'reconnect-source') {
          const outPort = findOutputPortAt(pointer.clientX, pointer.clientY)
          if (outPort?.dataset.portOut) void rewireSource(active.connection, outPort.dataset.portOut)
        } else {
          const target = findInputPortAt(pointer.clientX, pointer.clientY)
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
      },
      cancel: () => {
        setTempWire(null)
        setHoverPort(null)
        setHiddenWireId(null)
        setJudgeHover(null)
      },
    })
  }, [attachJudge, createJudgeFor, interactionOwner, removeWire, rewireSource, rewireTarget, screenToWorld, wire])

  const beginWire = useCallback((event: ReactPointerEvent, endpoint: string, kind: WirePath['kind']) => {
    if (event.button !== 0) return
    event.stopPropagation()
    ownWireDrag(event, { kind: 'new', from: endpoint, wireKind: kind })
    const start = portWorldCenter(endpoint)
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind, moving: 'b' } : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'b' })
  }, [ownWireDrag, portWorldCenter, screenToWorld])

  const beginReconnect = useCallback((event: ReactPointerEvent<HTMLElement> | ReactMouseEvent<HTMLElement>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (interactionOwner.projection()?.kind === 'wire') return
    event.stopPropagation()
    ownWireDrag(event, { kind: 'reconnect-target', connection })
    setHiddenWireId(connection.id)
    const kind = connectionKind(connection)
    const start = endpointWorldCenter(connection.from, 'out')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start
      ? { ax: start.x, ay: start.y, bx: w.x, by: w.y, kind, moving: 'b' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'b' })
  }, [endpointWorldCenter, interactionOwner, ownWireDrag, screenToWorld])

  const beginReconnectSource = useCallback((event: ReactPointerEvent<Element> | ReactMouseEvent<Element>, connection: BoardConnection) => {
    if (event.button !== 0) return
    if (interactionOwner.projection()?.kind === 'wire') return
    event.stopPropagation()
    ownWireDrag(event, { kind: 'reconnect-source', connection })
    setHiddenWireId(connection.id)
    const kind = connectionKind(connection)
    const fixed = endpointWorldCenter(connection.to, 'in')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(fixed
      ? { ax: w.x, ay: w.y, bx: fixed.x, by: fixed.y, kind, moving: 'a' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind, moving: 'a' })
  }, [endpointWorldCenter, interactionOwner, ownWireDrag, screenToWorld])

  const beginJudgeDrag = useCallback((event: ReactPointerEvent<HTMLElement>, gate: GateNode) => {
    if (event.button !== 0) return
    if (interactionOwner.projection()?.kind === 'wire') return
    event.stopPropagation()
    event.preventDefault()
    ownWireDrag(event, { kind: 'judge', gate, moved: false, startX: event.clientX, startY: event.clientY })
    const start = endpointWorldCenter(`${gate.id}:judge`, 'out')
    const w = screenToWorld(event.clientX, event.clientY)
    setTempWire(start
      ? { ax: w.x, ay: w.y, bx: start.x, by: start.y, kind: 'judge', moving: 'a' }
      : { ax: w.x, ay: w.y, bx: w.x, by: w.y, kind: 'judge', moving: 'a' })
  }, [endpointWorldCenter, interactionOwner, ownWireDrag, screenToWorld])

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
    const activeKind = interactionOwner.projection()?.kind
    if (activeKind === 'wire' || activeKind === 'lane') return
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
    const lane: LaneDrag = { connectionId: connection.id, previousLane, moved: false }
    interactionOwner.begin({
      kind: 'lane',
      pointerId: event.pointerId,
      project: pointer => {
        lane.moved = true
        const projected = screenToWorld(pointer.clientX, pointer.clientY)
        setLaneDraft({ connectionId: lane.connectionId, y: Math.round(projected.y) })
      },
      finalize: pointer => {
        if (!lane.moved) return
        const projected = screenToWorld(pointer.clientX, pointer.clientY)
        undoStack.current.push({ kind: 'setLane', edgeId: lane.connectionId, lane: lane.previousLane })
        void patchLayoutEdge(lane.connectionId, `y:${Math.round(projected.y)}`).then(() => setLaneDraft(null))
      },
      cancel: () => setLaneDraft(null),
    })
  }, [beginReconnect, beginReconnectSource, endpointWorldCenter, interactionOwner, patchLayoutEdge, screenToWorld])

  const beginGateToken = useCallback((event: ReactPointerEvent) => {
    if (event.button !== 0) return
    if (!boardRef.current) return
    event.preventDefault()
    interactionOwner.begin({
      kind: 'gate',
      pointerId: event.pointerId,
      project: pointer => setGateGhost({ x: pointer.clientX, y: pointer.clientY }),
      finalize: pointer => {
        const rect = viewportRef.current?.getBoundingClientRect()
        if (rect && pointer.clientX >= rect.left && pointer.clientX <= rect.right && pointer.clientY >= rect.top && pointer.clientY <= rect.bottom) {
          const w = screenToWorld(pointer.clientX, pointer.clientY)
          void createGateAt(w.x, w.y)
        }
      },
      cancel: () => setGateGhost(null),
    })
    setGateGhost({ x: event.clientX, y: event.clientY })
  }, [createGateAt, interactionOwner, screenToWorld])

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
      ...(formation.verification ? [
        { label: 'Migrate legacy verification', action: () => openLegacyVerification(formation) },
      ] : []),
      { label: 'Delete formation', destructive: true, action: () => deleteFormationOp(formation) },
    ])
  }, [addPortOp, deleteFormationOp, openBriefEditor, openLegacyVerification, openMenu, runFormation])

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
        { label: 'Mission', action: () => createMissionAt(w.x, w.y) },
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
            ? <span className="face">{harnessGlyph(slot.harness) ?? initials(slot.agentId as string)}</span>
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

  const rosterAgents = useMemo(() => agents.filter(agent => agent.assignable && !agent.unbound), [agents])
  const deployedAgentCount = useMemo(
    () => new Set((board?.formations || []).flatMap(f => f.slots.map(s => s.agentId).filter(Boolean))).size,
    [board?.formations],
  )
  const runBadgeClass = activeRun ? activeRun.status : ''
  const pendingHumanGateId = useMemo(() => openHumanGateId(runEvents), [runEvents])

  return (
    <div className="fmx" data-testid="formations-view" data-cockpit="d7" data-textsize={textSize}>
      <div className="topbar">
        <div className="boardpick">
          board
          <select value={selectedSlug} onChange={event => setSelectedSlug(event.target.value)} data-testid="board-picker" disabled={boards.length === 0}>
            {boards.length === 0 ? <option value="">No boards</option> : null}
            {boards.map(summary => <option key={summary.slug} value={summary.slug}>{summary.title || summary.slug}</option>)}
          </select>
          {board ? <span className="rev">rev {board.rev}</span> : null}
        </div>
        <div className="sep" />
        <button className="newbtn" onClick={createSolo} data-testid="new-formation" disabled={!board}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 5v14M5 12h14" /></svg>
          New formation
        </button>
        <div className="gatetoken" title="Drag onto the canvas to drop a gate" data-testid="gate-token" onPointerDown={beginGateToken}>
          {GATE_SVG}
          Gate
        </div>
        <div className="spacer" />
      </div>

      <div className="main">
        <aside className="roster" data-testid="agent-roster">
          <div className="roster-hd">
            <div className="t">Agents</div>
            <span className="s" data-testid="roster-count" title={`${rosterAgents.length} catalog agents · ${deployedAgentCount} deployed on this board`}>
              {rosterAgents.length}{deployedAgentCount ? ` · ${deployedAgentCount} deployed` : ''}
            </span>
          </div>
          <div className="roster-list">
            {rosterAgents.length === 0
              ? <div className="roster-empty">No assignable catalog agents. Add persona cards in ~/agents to staff formations.</div>
              : rosterAgents.map(agent => {
                const deployed = (board?.formations || []).some(f => f.slots.some(s => s.agentId === agent.id))
                return (
                  <div
                    key={agent.id}
                    className={`ragent${deployed ? ' deployed' : ''}${agent.unbound ? ' unbound' : ''}`}
                    data-agent={agent.id}
                    data-testid={`roster-agent-${agent.id}`}
                    onPointerDown={event => beginStaff(event, agent.id, agent.harnessDefault || '')}
                  >
                    <span className="av">{harnessGlyph(agent.harnessDefault) ?? initials(agent.id)}</span>
                    <div className="ri">
                      <div className="n">{agent.displayName || agent.id}</div>
                      <div className="r">{agentRole(agent)}{agentState(agent) === 'idle' ? ' · idle' : ''}</div>
                    </div>
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

            {!board ? (
              <div className="empty-board" data-testid="formations-empty-board">
                <div className="empty-title">No persisted formation boards</div>
                <div className="empty-copy">Seed a real board with Archon or create one through the API. The cockpit no longer shows fake starter missions.</div>
              </div>
            ) : null}

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
                  {formation.verification ? (
                    <button
                      type="button"
                      className="verify-band legacy"
                      data-gate={formation.verification.id}
                      data-testid={`verify-band-${formation.id}`}
                      aria-label={`Inspect legacy verification for ${formation.title}`}
                      onClick={() => openLegacyVerification(formation)}
                      onContextMenu={event => openMenu(event, 'Legacy verification', [
                        { label: 'Migrate legacy verification', action: () => openLegacyVerification(formation) },
                      ])}
                    >
                      <span className="vico"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9"><path d="M12 3l7 3v5c0 4.4-3 7.6-7 9-4-1.4-7-4.6-7-9V6z" /><path d="M12 8v5M12 16h.01" /></svg></span>
                      <span className="vlabel">legacy verify</span>
                      <span className="vkinds">{[
                        formation.verification.kinds?.length ? formation.verification.kinds.join(' · ') : 'checks not recorded',
                        formation.verification.criterion || 'criterion not recorded',
                        formation.verification.onFail || 'failure policy not recorded',
                      ].join(' · ')}</span>
                    </button>
                  ) : null}
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
            <button onClick={() => void arrangeBoard()} title="Arrange cards by graph flow (persists layout, Ctrl+Z to undo)" data-testid="arrange-layout">ARRANGE</button>
            <button onClick={() => fitView({ smooth: true })} title="Fit">FIT</button>
          </div>
          {error ? <div className="errbar" data-testid="formations-error">{error}</div> : null}
        </div>
      </div>

      {missionEditor ? (
        <div
          className="pop"
          role="dialog"
          aria-label="Create mission"
          onPointerDown={event => event.stopPropagation()}
        >
          <div className="pop-head">
            <span className="pt">Create mission</span>
            <button className="x" type="button" aria-label="Close mission creator" disabled={missionEditorSaving} onClick={closeMissionEditor}>x</button>
          </div>
          <form
            className="pop-body"
            onSubmit={event => {
              event.preventDefault()
              void saveMissionEditor()
            }}
          >
            <label htmlFor="cockpit-mission-title">Title</label>
            <input
              id="cockpit-mission-title"
              className="f"
              aria-label="Mission title"
              value={missionEditor.title}
              onChange={event => setMissionEditor(current => current ? { ...current, title: event.target.value } : current)}
            />
            <label htmlFor="cockpit-mission-goal">Goal</label>
            <textarea
              id="cockpit-mission-goal"
              aria-label="Mission goal"
              value={missionEditor.goal}
              onChange={event => setMissionEditor(current => current ? { ...current, goal: event.target.value } : current)}
            />
            <label htmlFor="cockpit-mission-bead">Bead ID</label>
            <input
              id="cockpit-mission-bead"
              className="f"
              aria-label="Mission Bead ID"
              aria-invalid={missionEditorError ? true : undefined}
              aria-describedby="cockpit-mission-bead-help"
              autoCapitalize="none"
              spellCheck={false}
              value={missionEditor.beadId}
              onChange={event => {
                setMissionEditor(current => current ? { ...current, beadId: event.target.value } : current)
                if (missionEditorError) setMissionEditorError('')
              }}
            />
            <p
              id="cockpit-mission-bead-help"
              className={`field-note${missionEditorError ? ' error' : ''}`}
              role={missionEditorError ? 'alert' : undefined}
            >
              {missionEditorError || 'Required. Copy a Bead ID from the Beads tab, for example ctx-ug7.25.'}
            </p>
            <div className="pop-actions">
              <button className="cancel" type="button" aria-label="Cancel mission creation" disabled={missionEditorSaving} onClick={closeMissionEditor}>Cancel</button>
              <button className="save" type="submit" disabled={missionEditorSaving}>{missionEditorSaving ? 'Creating…' : 'Create mission'}</button>
            </div>
          </form>
        </div>
      ) : null}

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

      {legacyVerification ? (
        <div
          className="pop"
          role="dialog"
          aria-label={`Legacy verification · ${legacyVerification.title}`}
          onPointerDown={event => event.stopPropagation()}
        >
          <div className="pop-head">
            <span className="pt">Legacy verification · {legacyVerification.title}</span>
            <button className="x" type="button" aria-label="Close legacy verification" onClick={() => setLegacyVerification(null)}>x</button>
          </div>
          <div className="pop-body">
            <label>Checks</label>
            <div className="legacy-value">{legacyVerification.kinds.length ? legacyVerification.kinds.join(' · ') : 'No checks recorded'}</div>
            <label>Criterion</label>
            <div className="legacy-value criterion">{legacyVerification.criterion || 'No criterion recorded'}</div>
            <label>Legacy failure policy</label>
            <div className="legacy-value">{legacyVerification.onFail || 'No failure policy recorded'}</div>
            <p className="legacy-note">Inline verification is retired because its verdict cannot be tied safely to an exact Formation attempt and output. Create and wire an explicit Gate to make the check, result, and route visible, then remove this legacy block.</p>
            {legacyVerification.replacementGateIds.length ? (
              <>
                <label htmlFor="legacy-replacement-gate">Replacement Gate</label>
                <select
                  id="legacy-replacement-gate"
                  className="legacy-select"
                  value={legacyVerification.replacementGateId}
                  onChange={event => setLegacyVerification(current => current ? { ...current, replacementGateId: event.target.value } : null)}
                >
                  {legacyVerification.replacementGateIds.map(gateId => (
                    <option key={gateId} value={gateId}>{board?.gates?.find(gate => gate.id === gateId)?.title || gateId}</option>
                  ))}
                </select>
              </>
            ) : (
              <p className="legacy-missing-gate">Wire an explicit Gate from a Formation output before removal.</p>
            )}
            {runEvents.some(event => event.type === 'verification_verdict' && event.nodeId === legacyVerification.formationId) ? (
              <div className="legacy-evidence">
                <div className="legacy-evidence-title">Legacy verification evidence · non-authorizing</div>
                {runEvents
                  .filter(event => event.type === 'verification_verdict' && event.nodeId === legacyVerification.formationId)
                  .map(event => <div key={event.seq}>seq {event.seq} · {typeof event.data?.verdict === 'string' ? event.data.verdict : 'verdict not recorded'}</div>)}
              </div>
            ) : null}
            <div className="pop-actions">
              <button className="cancel" type="button" onClick={() => setLegacyVerification(null)}>Keep for inspection</button>
              <button
                className="retire"
                type="button"
                disabled={!legacyVerification.replacementGateId}
                onClick={() => void removeLegacyVerification()}
              >
                Remove legacy verification
              </button>
            </div>
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
        <div className="fmx-ghost" style={{ left: ghost.x, top: ghost.y }}>{harnessGlyph(ghost.harness) ?? initials(ghost.agentId)}</div>
      ) : null}
      {gateGhost ? (
        <div className="gateghost" style={{ left: gateGhost.x, top: gateGhost.y }}>{GATE_SVG}</div>
      ) : null}
    </div>
  )
}
