import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { Plus, RefreshCw, X } from 'lucide-react'
import {
  ApiRequestError,
  abortRunRequest,
  fetchAgents,
  fetchApi,
  fetchBoardDocument,
  fetchBoardLayout,
  fetchBoardSummaries,
  fetchRunEvents,
  fetchRunStatus,
  patchBoardDocument,
  recordGateVerdict,
  resumeRunRequest,
  startRun,
} from './formationsApi'
import {
  activeRunStorageKey,
  openHumanGateId,
  projectNodeStates,
  runEventResumeAllowed,
  runEventText,
  runStatusFromResponse,
  upsertRunEvent,
} from './formationsRunState'
import { agentRole, initials } from './formationsCockpitVisuals'
import type {
  AgentProjection as FormationAgentProjection,
  BoardDocument,
  BoardSummary,
  FormationNode,
  FormationSlot,
  GateNode,
  LayoutDocument,
  RunEvent,
  RunStatusProjection,
} from './formationsTypes'

interface HarnessVariant {
  id: string
  sessionStem?: string
  launch?: string
  source?: string
}

interface PersonaNote {
  ts: string
  actor: string
  text: string
}

interface RosterAgent {
  id: string
  displayName?: string
  kind?: string
  tags?: string[]
  harnessDefault?: string
  liveness?: string
  sessionId?: string
  status?: string
  contextPct?: number
  beadId?: string
  attached?: boolean
  assignable: boolean
  unbound?: boolean
}

interface PersonaCard {
  id: string
  displayName?: string
  kind: string
  summary?: string
  tags: string[]
  status?: string
  harnessDefault: string
  harnessVariants: HarnessVariant[]
  notes?: PersonaNote[]
  etag?: string
  toml?: string
}

type CachedPersona = {
  card?: PersonaCard
  etag: string
  error?: string
}

type Selection =
  | { kind: 'agent'; agentId: string }
  | { kind: 'unbound'; agentId: string }
  | { kind: 'slot'; formationId: string; slotId: string }

export type ReachableMissionItem = {
  kind: 'formation' | 'gate'
  id: string
  via?: BranchProvenance
}

type BranchProvenance = {
  gateId: string
  branch: 'pass' | 'fail'
}

type CreateDraft = {
  id: string
  displayName: string
  kind: string
  harness: string
  sessionStem: string
  summary: string
  launch: string
  source: string
  capabilities: string
}

type AgentStatus = {
  liveness: 'live' | 'ambiguous' | 'offline'
  chips: string[]
  deployedSlots: number
}

const EMPTY_CREATE: CreateDraft = {
  id: '',
  displayName: '',
  kind: 'specialist',
  harness: 'claude-code',
  sessionStem: '',
  summary: '',
  launch: '',
  source: '',
  capabilities: '',
}

export function reachableMissionItems(board: BoardDocument, missionId: string): ReachableMissionItem[] {
  const formationIds = new Set(board.formations.map(formation => formation.id))
  const gateIds = new Set((board.gates || []).map(gate => gate.id))
  const formationById = new Map(board.formations.map(formation => [formation.id, formation]))
  const outgoing = new Map<string, string[]>()
  for (const connection of board.connections || []) {
    const list = outgoing.get(connection.from) || []
    list.push(connection.to)
    outgoing.set(connection.from, list)
  }

  const queue: Array<{ endpoint: string; via?: BranchProvenance }> = [{ endpoint: `${missionId}:out` }]
  const seenEndpoints = new Set<string>()
  const result: ReachableMissionItem[] = []
  const seenNodes = new Map<string, ReachableMissionItem>()

  const recordNode = (kind: ReachableMissionItem['kind'], id: string, via?: BranchProvenance) => {
    const key = `${kind}:${id}`
    const existing = seenNodes.get(key)
    if (!existing) {
      const item = via ? { kind, id, via } : { kind, id }
      seenNodes.set(key, item)
      result.push(item)
      return
    }
    if (!sameProvenance(existing.via, via)) {
      delete existing.via
    }
  }

  while (queue.length > 0) {
    const nextEndpoint = queue.shift()
    if (!nextEndpoint) continue
    const endpointKey = `${nextEndpoint.endpoint}|${provenanceKey(nextEndpoint.via)}`
    if (seenEndpoints.has(endpointKey)) continue
    seenEndpoints.add(endpointKey)

    for (const next of outgoing.get(nextEndpoint.endpoint) || []) {
      const nodeId = endpointNode(next)
      if (formationIds.has(nodeId)) {
        recordNode('formation', nodeId, nextEndpoint.via)
        const formation = formationById.get(nodeId)
        const outputs = formation?.outputs?.length ? formation.outputs : [{ id: 'out' }]
        outputs.forEach(output => queue.push({ endpoint: `${nodeId}:${output.id}`, via: nextEndpoint.via }))
        continue
      }
      if (gateIds.has(nodeId)) {
        recordNode('gate', nodeId, nextEndpoint.via)
        queue.push(
          { endpoint: `${nodeId}:pass`, via: { gateId: nodeId, branch: 'pass' } },
          { endpoint: `${nodeId}:fail`, via: { gateId: nodeId, branch: 'fail' } },
        )
      }
    }
  }

  return result
}

function provenanceKey(via?: BranchProvenance): string {
  return via ? `${via.gateId}:${via.branch}` : 'main'
}

function sameProvenance(a?: BranchProvenance, b?: BranchProvenance): boolean {
  if (!a && !b) return true
  return Boolean(a && b && a.gateId === b.gateId && a.branch === b.branch)
}

function endpointNode(endpoint: string): string {
  return endpoint.split(':')[0] || endpoint
}

export function agentStatus(agent: RosterAgent, deployedSlots: number, details?: CachedPersona): AgentStatus {
  const rawLiveness = agent.liveness === 'live' || agent.liveness === 'ambiguous' ? agent.liveness : 'offline'
  const chips: string[] = []
  if (agent.unbound) chips.push('no persona')
  if (!agent.unbound && details?.card?.status === 'retired') chips.push('retired')
  if (!agent.unbound && !agent.assignable && details?.card?.status !== 'retired') chips.push('not assignable')
  if (deployedSlots > 0) chips.push(`in ${deployedSlots} slot${deployedSlots === 1 ? '' : 's'}`)
  if (agent.attached) chips.push('attached')
  return { liveness: rawLiveness, chips, deployedSlots }
}

export default function AgentsView() {
  const [agents, setAgents] = useState<RosterAgent[]>([])
  const [boards, setBoards] = useState<BoardSummary[]>([])
  const [selectedSlug, setSelectedSlug] = useState('')
  const [board, setBoard] = useState<BoardDocument | null>(null)
  const [layout, setLayout] = useState<LayoutDocument | null>(null)
  const [selectedMissionId, setSelectedMissionId] = useState('')
  const [details, setDetails] = useState<Record<string, CachedPersona>>({})
  const [selection, setSelection] = useState<Selection | null>(null)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [boardLoading, setBoardLoading] = useState(false)
  const [error, setError] = useState('')
  const [boardError, setBoardError] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [createDraft, setCreateDraft] = useState<CreateDraft>(EMPTY_CREATE)
  const [noteDraft, setNoteDraft] = useState('')
  const [drawer, setDrawer] = useState<'roster' | 'details' | ''>('')
  const [activeRun, setActiveRun] = useState<RunStatusProjection | null>(null)
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])

  const selectedBoardSummary = boards.find(next => next.slug === selectedSlug)
  const selectedMission = board?.missions?.find(mission => mission.id === selectedMissionId) || null
  const nodeStates = useMemo(() => projectNodeStates(runEvents, activeRun), [activeRun, runEvents])
  const openGateId = useMemo(() => openHumanGateId(runEvents), [runEvents])
  const resumeAllowed = Boolean(activeRun?.resumeAllowed || runEvents.some(event => runEventResumeAllowed(event, false)))
  const activeRunMission = board?.missions?.find(mission => mission.id === activeRun?.missionId) || null

  const reachableItems = useMemo(() => {
    if (!board || !selectedMissionId) return []
    return orderReachableItems(reachableMissionItems(board, selectedMissionId), layout)
  }, [board, layout, selectedMissionId])

  const reachableViaByFormation = useMemo(() => {
    const map = new Map<string, BranchProvenance>()
    for (const item of reachableItems) {
      if (item.kind === 'formation' && item.via) map.set(item.id, item.via)
    }
    return map
  }, [reachableItems])

  const gateBranchLabels = useMemo(() => {
    const map = new Map<string, { pass: string[]; fail: string[] }>()
    if (!board) return map
    const formationTitles = new Map(board.formations.map(formation => [formation.id, formation.title]))
    for (const item of reachableItems) {
      if (item.kind !== 'formation' || !item.via) continue
      const labels = map.get(item.via.gateId) || { pass: [], fail: [] }
      labels[item.via.branch].push(formationTitles.get(item.id) || item.id)
      map.set(item.via.gateId, labels)
    }
    return map
  }, [board, reachableItems])

  const reachableFormations = useMemo(() => {
    if (!board) return []
    const byId = new Map(board.formations.map(formation => [formation.id, formation]))
    return reachableItems
      .filter(item => item.kind === 'formation')
      .map(item => byId.get(item.id))
      .filter((formation): formation is FormationNode => Boolean(formation))
  }, [board, reachableItems])

  const assignmentsByAgent = useMemo(() => {
    const map = new Map<string, Array<{ formation: FormationNode; slot: FormationSlot }>>()
    for (const formation of reachableFormations) {
      for (const slot of formation.slots || []) {
        if (!slot.agentId) continue
        const list = map.get(slot.agentId) || []
        list.push({ formation, slot })
        map.set(slot.agentId, list)
      }
    }
    return map
  }, [reachableFormations])

  const slotCounts = useMemo(() => {
    let total = 0
    let staffed = 0
    for (const formation of reachableFormations) {
      for (const slot of formation.slots || []) {
        total += 1
        if (slot.agentId) staffed += 1
      }
    }
    return { total, staffed, open: Math.max(total - staffed, 0) }
  }, [reachableFormations])

  const startDisabledReason = useMemo(() => {
    if (!board || !selectedMissionId) return 'Select a mission before starting'
    if (slotCounts.total === 0) return 'No slots on this mission'
    if (slotCounts.open > 0) return 'Staff all slots before starting'
    if (activeRun && !activeRun.final) {
      return activeRun.missionId === selectedMissionId ? 'Run already active' : 'Another mission is already running'
    }
    return ''
  }, [activeRun, board, selectedMissionId, slotCounts.open, slotCounts.total])

  const rosterCounts = useMemo(() => ({
    total: agents.length,
    live: agents.filter(agent => agent.liveness === 'live').length,
    assignable: agents.filter(agent => !agent.unbound && agent.assignable).length,
  }), [agents])

  const filteredAgents = useMemo(() => {
    const needle = search.trim().toLowerCase()
    if (!needle) return agents
    return agents.filter(agent => [
      agent.id,
      agent.displayName || '',
      agent.kind || '',
      ...(agent.tags || []),
    ].some(value => value.toLowerCase().includes(needle)))
  }, [agents, search])

  const personas = filteredAgents.filter(agent => !agent.unbound)
  const unbound = filteredAgents.filter(agent => agent.unbound)

  const selectedSlot = useMemo(() => {
    if (selection?.kind !== 'slot') return null
    return findSlot(board, selection.formationId, selection.slotId)
  }, [board, selection])

  const loadAgents = useCallback(async () => {
    const nextAgents = await fetchAgents()
    setAgents(nextAgents as RosterAgent[])
  }, [])

  const loadBoards = useCallback(async () => {
    const nextBoards = await fetchBoardSummaries()
    setBoards(nextBoards)
    setSelectedSlug(current => current || nextBoards[0]?.slug || '')
  }, [])

  const loadBoard = useCallback(async (slug: string) => {
    setBoardLoading(true)
    try {
      const [nextBoard, nextLayout] = await Promise.all([
        fetchBoardDocument(slug),
        fetchBoardLayout(slug),
      ])
      setBoard(nextBoard)
      setLayout(nextLayout)
      setSelectedMissionId(current => {
        if (current && nextBoard.missions?.some(mission => mission.id === current)) return current
        return nextBoard.missions?.[0]?.id || ''
      })
      setBoardError('')
      setError('')
    } catch (err) {
      setBoardError(err instanceof Error ? err.message : 'Failed to load board')
    } finally {
      setBoardLoading(false)
    }
  }, [])

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([loadAgents(), loadBoards()])
      if (selectedSlug) await loadBoard(selectedSlug)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agents request failed')
    } finally {
      setLoading(false)
    }
  }, [loadAgents, loadBoard, loadBoards, selectedSlug])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      try {
        const [nextAgents, nextBoards] = await Promise.all([fetchAgents(), fetchBoardSummaries()])
        if (cancelled) return
        setAgents(nextAgents as RosterAgent[])
        setBoards(nextBoards)
        setSelectedSlug(current => current || nextBoards[0]?.slug || '')
        setError('')
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Agents request failed')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!selectedSlug) {
      setBoard(null)
      setLayout(null)
      return
    }
    void loadBoard(selectedSlug)
  }, [loadBoard, selectedSlug])

  useEffect(() => {
    if (!selectedSlug) return
    const runId = window.localStorage.getItem(activeRunStorageKey(selectedSlug))
    if (!runId || activeRun?.runId === runId) return
    let cancelled = false
    const restoreRun = async () => {
      try {
        const status = runStatusFromResponse(await fetchRunStatus(runId))
        const events = await fetchRunEvents(runId)
        if (cancelled) return
        setActiveRun(status)
        setRunEvents(events)
        if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to restore active run')
      }
    }
    void restoreRun()
    return () => { cancelled = true }
  }, [activeRun?.runId, selectedSlug])

  useEffect(() => {
    if (!activeRun?.runId || activeRun.final) return
    let cancelled = false
    const tick = async () => {
      try {
        const status = runStatusFromResponse(await fetchRunStatus(activeRun.runId))
        const events = await fetchRunEvents(activeRun.runId)
        if (cancelled) return
        setActiveRun(status)
        setRunEvents(prev => events.reduce((acc, event) => upsertRunEvent(acc, event), prev))
        if (status.final && selectedSlug) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
      } catch {
        /* transient run polling failure */
      }
    }
    const timer = window.setInterval(() => { void tick() }, 1200)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [activeRun?.final, activeRun?.runId, selectedSlug])

  const loadAgentDetail = useCallback(async (agentId: string, force = false): Promise<CachedPersona> => {
    if (!force && details[agentId]) return details[agentId]
    const result = await fetchApi<PersonaCard>(`/api/agents/${encodeURIComponent(agentId)}`)
    const cached: CachedPersona = {
      card: { ...result.data, etag: result.etag || result.data.etag },
      etag: result.etag || result.data.etag || '',
    }
    setDetails(current => ({ ...current, [agentId]: cached }))
    return cached
  }, [details])

  const inspectAgent = useCallback(async (agent: RosterAgent) => {
    if (agent.unbound) {
      setSelection({ kind: 'unbound', agentId: agent.id })
      setNoteDraft('')
      return
    }
    setSelection({ kind: 'agent', agentId: agent.id })
    setNoteDraft('')
    try {
      await loadAgentDetail(agent.id, Boolean(details[agent.id]?.error))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agent inspect request failed')
    }
  }, [details, loadAgentDetail])

  const inspectSlot = useCallback(async (formation: FormationNode, slot: FormationSlot) => {
    setSelection({ kind: 'slot', formationId: formation.id, slotId: slot.id })
    if (!slot.harness) {
      setError('')
      return
    }
    const detailAgents = agents.filter(agent => !agent.unbound && agent.assignable)
    const settled = await Promise.allSettled(detailAgents.map(agent => loadAgentDetail(agent.id, Boolean(details[agent.id]?.error))))
    setDetails(current => {
      const next = { ...current }
      settled.forEach((result, index) => {
        if (result.status === 'rejected') {
          next[detailAgents[index].id] = { etag: '', error: 'failed detail load' }
        }
      })
      return next
    })
    setError('')
  }, [agents, details, loadAgentDetail])

  const updateBoardWithPatch = useCallback(async (patch: Record<string, unknown>) => {
    if (!board) return
    try {
      const result = await patchBoardDocument(board.slug, board.etag, board.rev, patch)
      setBoard(result.board)
      if (result.layout) setLayout(result.layout)
      setError('')
      await loadAgents()
    } catch (err) {
      if (err instanceof ApiRequestError && (err.status === 409 || err.status === 428)) {
        setError('Board changed; reload and retry')
        return
      }
      setError(err instanceof Error ? err.message : 'Board update failed')
    }
  }, [board, loadAgents])

  const assignSlot = useCallback(async (formation: FormationNode, slot: FormationSlot, agent: RosterAgent, harness: string) => {
    await updateBoardWithPatch({
      assignSlot: {
        formationId: formation.id,
        slotId: slot.id,
        agentId: agent.id,
        harness,
      },
    })
  }, [updateBoardWithPatch])

  const unassignSlot = useCallback(async (formation: FormationNode, slot: FormationSlot) => {
    await updateBoardWithPatch({
      assignSlot: {
        formationId: formation.id,
        slotId: slot.id,
        agentId: '',
        harness: '',
      },
    })
  }, [updateBoardWithPatch])

  const createPersona = useCallback(async (event: FormEvent) => {
    event.preventDefault()
    const capabilities = splitCommaList(createDraft.capabilities)
    try {
      const result = await fetchApi<PersonaCard>('/api/agents', {
        method: 'POST',
        body: JSON.stringify({
          id: createDraft.id.trim(),
          displayName: createDraft.displayName.trim(),
          kind: createDraft.kind.trim(),
          harness: createDraft.harness.trim(),
          sessionStem: createDraft.sessionStem.trim(),
          summary: createDraft.summary.trim(),
          launch: createDraft.launch.trim(),
          source: createDraft.source.trim(),
          capabilities,
        }),
      })
      const cached = { card: { ...result.data, etag: result.etag || result.data.etag }, etag: result.etag || result.data.etag || '' }
      setDetails(current => ({ ...current, [result.data.id]: cached }))
      setSelection({ kind: 'agent', agentId: result.data.id })
      setCreateDraft(EMPTY_CREATE)
      setCreateOpen(false)
      setError('')
      await loadAgents()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Agent create request failed')
    }
  }, [createDraft, loadAgents])

  const saveNote = useCallback(async (agentId: string) => {
    const note = noteDraft.trim()
    if (!note) return
    try {
      const cached = details[agentId]?.card ? details[agentId] : await loadAgentDetail(agentId, Boolean(details[agentId]?.error))
      if (!cached.card) {
        setError('Agent detail unavailable; reload and retry')
        return
      }
      const result = await fetchApi<PersonaCard>(`/api/agents/${encodeURIComponent(agentId)}`, {
        method: 'PATCH',
        headers: { 'If-Match': cached.etag },
        body: JSON.stringify({ note }),
      })
      setDetails(current => ({
        ...current,
        [agentId]: {
          card: { ...result.data, etag: result.etag || result.data.etag },
          etag: result.etag || result.data.etag || '',
        },
      }))
      setNoteDraft('')
      setError('')
      await loadAgents()
    } catch (err) {
      if (err instanceof ApiRequestError && err.status === 428) {
        setError(`Programming error: ${err.message}`)
        return
      }
      if (err instanceof ApiRequestError && err.status === 409) {
        setError(err.message || 'Agent card changed; reload and retry')
        return
      }
      setError(err instanceof Error ? err.message : 'Agent update request failed')
    }
  }, [details, loadAgentDetail, loadAgents, noteDraft])

  const handleStartMission = useCallback(async () => {
    if (!board || !selectedMissionId || startDisabledReason) return
    try {
      const result = await startRun(board.etag, {
        board: board.slug,
        missionId: selectedMissionId,
        actor: 'agent:ui',
      })
      const status = runStatusFromResponse(result.status)
      const runId = result.runId || status.runId
      setActiveRun(status)
      if (runId) {
        window.localStorage.setItem(activeRunStorageKey(board.slug), runId)
        const events = await fetchRunEvents(runId)
        setRunEvents(events)
      }
      setError('')
    } catch (err) {
      if (err instanceof ApiRequestError && (err.status === 409 || err.status === 428)) {
        setError('Board changed; reload and retry')
        return
      }
      setError(err instanceof Error ? err.message : 'Run start request failed')
    }
  }, [board, selectedMissionId, startDisabledReason])

  const createFromUnbound = useCallback((agent: RosterAgent) => {
    const sessionStem = agent.sessionId || agent.id
    setCreateDraft({
      ...EMPTY_CREATE,
      id: agent.id,
      displayName: agent.displayName || agent.id,
      harness: inferHarnessFromSession(sessionStem),
      sessionStem,
      capabilities: (agent.tags || []).join(', '),
    })
    setCreateOpen(true)
  }, [])

  const handleGateVerdict = useCallback(async (verdict: 'pass' | 'fail') => {
    if (!activeRun?.runId || !openGateId || !selectedSlug) return
    try {
      const status = runStatusFromResponse(await recordGateVerdict(activeRun.runId, openGateId, {
        actor: 'agent:ui',
        verdict,
        reason: `Recorded from Agents tab: ${verdict}`,
      }))
      setActiveRun(status)
      const events = await fetchRunEvents(activeRun.runId)
      setRunEvents(events)
      if (status.final) window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Gate verdict request failed')
    }
  }, [activeRun?.runId, openGateId, selectedSlug])

  const handleResume = useCallback(async () => {
    if (!activeRun?.runId || !selectedSlug) return
    try {
      const status = runStatusFromResponse(await resumeRunRequest(activeRun.runId, {
        actor: 'agent:ui',
        mode: 'continue',
        reason: 'Resumed from Agents tab',
      }))
      setActiveRun(status)
      const events = await fetchRunEvents(activeRun.runId)
      setRunEvents(events)
      if (!status.final) window.localStorage.setItem(activeRunStorageKey(selectedSlug), status.runId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Run resume request failed')
    }
  }, [activeRun?.runId, selectedSlug])

  const handleAbort = useCallback(async () => {
    if (!activeRun?.runId || !selectedSlug) return
    try {
      const status = runStatusFromResponse(await abortRunRequest(activeRun.runId, {
        requestedBy: 'agent:ui',
        reason: 'Stopped from Agents tab',
      }))
      setActiveRun(status)
      const events = await fetchRunEvents(activeRun.runId)
      setRunEvents(events)
      window.localStorage.removeItem(activeRunStorageKey(selectedSlug))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Run abort request failed')
    }
  }, [activeRun?.runId, selectedSlug])

  return (
    <div className="agents-view agx" data-testid="agents-view">
      <header className="agx-controlbar">
        <label className="agx-control">
          <span>Board</span>
          <select value={selectedSlug} onChange={event => setSelectedSlug(event.target.value)} disabled={loading || boards.length === 0}>
            {boards.length === 0 && <option value="">No boards</option>}
            {boards.map(next => (
              <option key={next.slug} value={next.slug}>{next.title || next.slug}</option>
            ))}
          </select>
        </label>
        <label className="agx-control agx-control-wide">
          <span>Mission</span>
          <select
            value={selectedMissionId}
            onChange={event => setSelectedMissionId(event.target.value)}
            disabled={boardLoading || !board?.missions?.length}
          >
            {!board?.missions?.length && <option value="">No missions</option>}
            {(board?.missions || []).map(mission => (
              <option key={mission.id} value={mission.id}>{mission.title}</option>
            ))}
          </select>
        </label>
        <div className="agx-counts" aria-label="Roster and slot counts">
          <span>Roster {loading ? '--' : rosterCounts.total}</span>
          <span>{rosterCounts.live} live</span>
          <span>{rosterCounts.assignable} assignable</span>
          <span>Slots {slotCounts.staffed}/{slotCounts.total || 0} staffed</span>
          <span>{slotCounts.open} open</span>
        </div>
        <button className="agx-drawer-toggle" type="button" onClick={() => setDrawer(current => current === 'roster' ? '' : 'roster')}>
          Roster
        </button>
        <button className="agx-drawer-toggle" type="button" onClick={() => setDrawer(current => current === 'details' ? '' : 'details')}>
          Details
        </button>
        <button className="agx-icon-button" type="button" onClick={refresh} disabled={loading || boardLoading}>
          <RefreshCw size={14} aria-hidden="true" />
          Refresh
        </button>
        <button className="agx-primary-button" type="button" onClick={() => setCreateOpen(true)}>
          <Plus size={14} aria-hidden="true" />
          Add Agent
        </button>
      </header>

      {error && <div className="agx-alert" role="alert">{error}</div>}

      <div className="agx-shell">
        <aside className={`agx-roster ${drawer === 'roster' ? 'is-open' : ''}`} aria-label="Agent roster">
          <div className="agx-panel-head">
            <h2>Roster</h2>
            <span>{personas.length} personas</span>
          </div>
          <input
            className="agx-search"
            aria-label="Filter agents"
            value={search}
            onChange={event => setSearch(event.target.value)}
            placeholder="filter agents"
          />
          <RosterGroup
            title="Personas"
            agents={personas}
            details={details}
            assignmentsByAgent={assignmentsByAgent}
            onInspect={inspectAgent}
          />
          <RosterGroup
            title="Unbound"
            agents={unbound}
            details={details}
            assignmentsByAgent={assignmentsByAgent}
            onInspect={inspectAgent}
          />
        </aside>

        <main className="agx-staffing" aria-label="Mission staffing">
          {boardError && (
            <div className="agx-alert agx-board-alert" role="alert">
              <span>Board load failed: {boardError}</span>
              <button className="agx-icon-button" type="button" onClick={() => selectedSlug && loadBoard(selectedSlug)}>
                Retry board
              </button>
            </div>
          )}
          {activeRun && (
            <RunBanner
              status={activeRun}
              missionTitle={activeRunMission?.title || activeRun.missionId}
              selectedMissionTitle={selectedMission?.title || selectedMissionId}
              missionMismatch={Boolean(activeRun.missionId && activeRun.missionId !== selectedMissionId)}
              events={runEvents}
              openGateId={openGateId}
              resumeAllowed={resumeAllowed}
              onViewMission={() => setSelectedMissionId(activeRun.missionId)}
              onVerdict={handleGateVerdict}
              onResume={handleResume}
              onAbort={handleAbort}
            />
          )}
          <div className="agx-mission-head">
            <div>
              <p className="agx-eyebrow">Mission staffing</p>
              <h1>{selectedMission?.title || selectedBoardSummary?.title || 'No mission selected'}</h1>
            </div>
            <div className="agx-mission-actions">
              <span className={slotCounts.open > 0 ? 'agx-status agx-status-open' : 'agx-status agx-status-ready'}>
                {slotCounts.total === 0 ? 'no slots' : slotCounts.open > 0 ? `${slotCounts.open} slots open` : 'ready'}
              </span>
              <button
                className="agx-primary-button"
                type="button"
                onClick={handleStartMission}
                disabled={Boolean(startDisabledReason)}
                title={startDisabledReason || 'Start mission'}
              >
                Start mission
              </button>
              {startDisabledReason && <span className="agx-muted">{startDisabledReason}</span>}
            </div>
          </div>

          {!selectedMission && (
            <div className="agx-empty">Select a mission to inspect staffing readiness.</div>
          )}
          {selectedMission && reachableItems.length === 0 && (
            <div className="agx-empty">No reachable formations from this mission.</div>
          )}
          {selectedMission && reachableItems.map(item => (
            item.kind === 'gate'
              ? (
                <GateRow
                  key={item.id}
                  gate={(board?.gates || []).find(gate => gate.id === item.id) || null}
                  state={nodeStates.get(item.id) || ''}
                  branchLabels={gateBranchLabels.get(item.id)}
                />
              )
              : (
                <FormationStaffingCard
                  key={item.id}
                  formation={board?.formations.find(formation => formation.id === item.id) || null}
                  via={reachableViaByFormation.get(item.id)}
                  viaGateTitle={(board?.gates || []).find(gate => gate.id === item.via?.gateId)?.title || item.via?.gateId || ''}
                  agents={agents}
                  nodeState={nodeStates.get(item.id) || ''}
                  selectedSlot={selectedSlot}
                  onSlotClick={inspectSlot}
                />
              )
          ))}
        </main>

        <aside className={`agx-inspector ${drawer === 'details' ? 'is-open' : ''}`} aria-label="Inspector">
          <Inspector
            selection={selection}
            agents={agents}
            details={details}
            assignmentsByAgent={assignmentsByAgent}
            selectedSlot={selectedSlot}
            noteDraft={noteDraft}
            onNoteDraft={setNoteDraft}
            onSaveNote={saveNote}
            onAssign={assignSlot}
            onUnassign={unassignSlot}
            onCreateFromUnbound={createFromUnbound}
          />
        </aside>
      </div>

      {createOpen && (
        <CreatePersonaPopover
          draft={createDraft}
          onDraft={setCreateDraft}
          onSubmit={createPersona}
          onClose={() => setCreateOpen(false)}
        />
      )}
    </div>
  )
}

function orderReachableItems(items: ReachableMissionItem[], layout: LayoutDocument | null): ReachableMissionItem[] {
  if (!layout?.nodes?.length) return items
  const position = new Map(layout.nodes.map(node => [node.id, node]))
  return [...items].sort((a, b) => {
    const ap = position.get(a.id)
    const bp = position.get(b.id)
    if (!ap && !bp) return items.indexOf(a) - items.indexOf(b)
    if (!ap) return 1
    if (!bp) return -1
    return ap.y === bp.y ? ap.x - bp.x : ap.y - bp.y
  })
}

function RosterGroup({
  title,
  agents,
  details,
  assignmentsByAgent,
  onInspect,
}: {
  title: string
  agents: RosterAgent[]
  details: Record<string, CachedPersona>
  assignmentsByAgent: Map<string, Array<{ formation: FormationNode; slot: FormationSlot }>>
  onInspect: (agent: RosterAgent) => void
}) {
  return (
    <section className="agx-roster-group">
      <div className="agx-group-title">{title}</div>
      {agents.length === 0 && <div className="agx-muted">{title === 'Unbound' ? 'No unbound sessions' : 'No personas'}</div>}
      {agents.map(agent => {
        const status = agentStatus(agent, assignmentsByAgent.get(agent.id)?.length || 0, details[agent.id])
        const name = agent.displayName || agent.id
        return (
          <button
            key={agent.id}
            type="button"
            className={`ragent ${agent.unbound ? 'unbound' : ''} ${status.deployedSlots > 0 ? 'deployed' : ''}`}
            aria-label={`Inspect ${name}`}
            onClick={() => onInspect(agent)}
          >
            <span className="av">{initials(name)}</span>
            <span className="ri">
              <span className="n">{name}</span>
              <span className="r">{agentRole(agent as FormationAgentProjection)}</span>
            </span>
            <span className="agx-chip-row">
              <span className={`agx-chip agx-chip-${status.liveness}`}>{status.liveness}</span>
              {status.chips.map(chip => <span className="agx-chip" key={chip}>{chip}</span>)}
            </span>
          </button>
        )
      })}
    </section>
  )
}

function FormationStaffingCard({
  formation,
  via,
  viaGateTitle,
  agents,
  nodeState,
  selectedSlot,
  onSlotClick,
}: {
  formation: FormationNode | null
  via?: BranchProvenance
  viaGateTitle: string
  agents: RosterAgent[]
  nodeState: string
  selectedSlot: { formation: FormationNode; slot: FormationSlot } | null
  onSlotClick: (formation: FormationNode, slot: FormationSlot) => void
}) {
  if (!formation) return null
  const open = formation.slots.filter(slot => !slot.agentId).length
  const fallbackLabel = via?.branch === 'fail' ? `fallback on ${viaGateTitle || via.gateId} fail` : ''
  return (
    <section className={`agx-staff-card ${nodeState ? `is-${nodeState}` : ''}`}>
      <header className="agx-staff-card-head">
        <div>
          <p className="agx-eyebrow">{formation.type}</p>
          <h2>{formation.title}</h2>
          {fallbackLabel && <span className="agx-branch-badge">{fallbackLabel}</span>}
        </div>
        <span className={open > 0 ? 'agx-status agx-status-open' : 'agx-status agx-status-ready'}>
          {open > 0 ? `${open} open` : 'staffed'}
        </span>
      </header>
      <div className="agx-slot-row">
        {formation.slots.map(slot => {
          const assigned = slot.agentId ? agents.find(agent => agent.id === slot.agentId) : null
          const assignedName = assigned?.displayName || slot.agentId || ''
          const active = selectedSlot?.formation.id === formation.id && selectedSlot.slot.id === slot.id
          return (
            <button
              key={slot.id}
              type="button"
              className={`slot ${slot.agentId ? 'filled' : 'empty'} ${slot.controller ? 'ctrl' : ''} ${active ? 'active' : ''}`}
              aria-label={slot.agentId ? `Inspect ${slot.label} slot assigned to ${assignedName}` : `Assign ${slot.label} slot`}
              onClick={() => onSlotClick(formation, slot)}
            >
              <span className="slot-ring">
                {slot.controller && <span className="badge">C</span>}
                {slot.agentId ? <span className="face">{initials(assignedName || slot.agentId)}</span> : <span className="plus">+</span>}
              </span>
              <span className="slot-label">{slot.label}</span>
              <span className="who">{slot.agentId ? assignedName : 'open'}</span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function GateRow({
  gate,
  state,
  branchLabels,
}: {
  gate: GateNode | null
  state: string
  branchLabels?: { pass: string[]; fail: string[] }
}) {
  if (!gate) return null
  return (
    <section className={`agx-gate-row ${state ? `is-${state}` : ''}`}>
      <div className="agx-gate-icon">G</div>
      <div>
        <p className="agx-eyebrow">Read-only gate</p>
        <h2>{gate.title}</h2>
        <p>{gate.kinds.join(', ') || 'gate'} - {gate.criterion}</p>
        {Boolean(branchLabels?.pass.length || branchLabels?.fail.length) && (
          <div className="agx-branch-row" aria-label={`${gate.title} branch targets`}>
            {branchLabels?.pass.length ? <span>pass: {branchLabels.pass.join(', ')}</span> : null}
            {branchLabels?.fail.length ? <span>fail: {branchLabels.fail.join(', ')}</span> : null}
          </div>
        )}
      </div>
    </section>
  )
}

function Inspector({
  selection,
  agents,
  details,
  assignmentsByAgent,
  selectedSlot,
  noteDraft,
  onNoteDraft,
  onSaveNote,
  onAssign,
  onUnassign,
  onCreateFromUnbound,
}: {
  selection: Selection | null
  agents: RosterAgent[]
  details: Record<string, CachedPersona>
  assignmentsByAgent: Map<string, Array<{ formation: FormationNode; slot: FormationSlot }>>
  selectedSlot: { formation: FormationNode; slot: FormationSlot } | null
  noteDraft: string
  onNoteDraft: (value: string) => void
  onSaveNote: (agentId: string) => void
  onAssign: (formation: FormationNode, slot: FormationSlot, agent: RosterAgent, harness: string) => void
  onUnassign: (formation: FormationNode, slot: FormationSlot) => void
  onCreateFromUnbound: (agent: RosterAgent) => void
}) {
  if (!selection) {
    return <div className="agx-empty">Select an agent or slot.</div>
  }

  if (selection.kind === 'unbound') {
    const agent = agents.find(next => next.id === selection.agentId)
    return (
      <section className="agx-inspector-section">
        <p className="agx-eyebrow">Unbound session</p>
        <h2>{agent?.displayName || selection.agentId}</h2>
        <p className="agx-muted">This live session has no persona card. It cannot be assigned until a persona exists.</p>
        <KeyValue label="Liveness" value={agent?.liveness || 'live'} />
        <KeyValue label="Session" value={agent?.sessionId || agent?.id || selection.agentId} />
        {agent && (
          <button className="agx-primary-button" type="button" onClick={() => onCreateFromUnbound(agent)}>
            Create persona from this session
          </button>
        )}
      </section>
    )
  }

  if (selection.kind === 'agent') {
    const agent = agents.find(next => next.id === selection.agentId)
    const detail = details[selection.agentId]
    const card = detail?.card
    const assignments = assignmentsByAgent.get(selection.agentId) || []
    return (
      <section className="agx-inspector-section">
        <p className="agx-eyebrow">Agent inspector</p>
        <h2>{card?.displayName || agent?.displayName || selection.agentId}</h2>
        <div className="agx-inspector-chips">
          <span className="agx-chip">{agent?.liveness || 'offline'}</span>
          {card?.status && <span className="agx-chip">{card.status}</span>}
          {agent?.attached && <span className="agx-chip">attached</span>}
          {!agent?.assignable && !card?.status && <span className="agx-chip">not assignable</span>}
        </div>
        <KeyValue label="Kind" value={card?.kind || agent?.kind || 'agent'} />
        <KeyValue label="Default harness" value={card?.harnessDefault || agent?.harnessDefault || ''} />
        <KeyValue label="Session" value={agent?.sessionId || ''} />
        <KeyValue label="Context" value={typeof agent?.contextPct === 'number' ? `${agent.contextPct}%` : ''} />
        <KeyValue label="Bead" value={agent?.beadId || ''} />
        {card?.summary && <p className="agx-muted">{card.summary}</p>}
        <TagList tags={card?.tags || agent?.tags || []} />
        <section className="agx-detail-block">
          <h3>Harness variants</h3>
          {(card?.harnessVariants || []).map(variant => (
            <div className="agx-line" key={variant.id}>
              <span>{variant.id}</span>
              <span>{variant.sessionStem || card?.id}</span>
              {variant.launch && <span>{variant.launch}</span>}
              {variant.source && <span>{variant.source}</span>}
            </div>
          ))}
        </section>
        <section className="agx-detail-block">
          <h3>Current slots</h3>
          {assignments.length === 0 && <p className="agx-muted">No slots on this mission.</p>}
          {assignments.map(({ formation, slot }) => (
            <div className="agx-line" key={`${formation.id}:${slot.id}`}>{formation.title} / {slot.label}{slot.controller ? ' / controller' : ''}</div>
          ))}
        </section>
        <form
          className="agx-note-form"
          onSubmit={event => {
            event.preventDefault()
            onSaveNote(selection.agentId)
          }}
        >
          <label htmlFor="agx-note">Add note</label>
          <textarea id="agx-note" value={noteDraft} onChange={event => onNoteDraft(event.target.value)} />
          <button className="agx-primary-button" type="submit">Save note</button>
        </form>
      </section>
    )
  }

  if (selection.kind === 'slot' && selectedSlot) {
    return (
      <SlotInspector
        agents={agents}
        details={details}
        formation={selectedSlot.formation}
        slot={selectedSlot.slot}
        onAssign={onAssign}
        onUnassign={onUnassign}
      />
    )
  }

  return <div className="agx-empty">Select an agent or slot.</div>
}

function SlotInspector({
  agents,
  details,
  formation,
  slot,
  onAssign,
  onUnassign,
}: {
  agents: RosterAgent[]
  details: Record<string, CachedPersona>
  formation: FormationNode
  slot: FormationSlot
  onAssign: (formation: FormationNode, slot: FormationSlot, agent: RosterAgent, harness: string) => void
  onUnassign: (formation: FormationNode, slot: FormationSlot) => void
}) {
  const assigned = slot.agentId ? agents.find(agent => agent.id === slot.agentId) : null
  return (
    <section className="agx-inspector-section">
      <p className="agx-eyebrow">Slot inspector</p>
      <h2>{formation.title} / {slot.label}</h2>
      <KeyValue label="Controller" value={slot.controller ? 'yes' : 'no'} />
      <KeyValue label="Harness" value={slot.harness || 'default'} />
      <KeyValue label="Current" value={assigned?.displayName || slot.agentId || 'open'} />
      {slot.agentId && (
        <button className="agx-danger-button" type="button" onClick={() => onUnassign(formation, slot)}>
          Unassign {assigned?.displayName || slot.agentId}
        </button>
      )}
      <section className="agx-detail-block">
        <h3>Eligible agents</h3>
        {agents.map(agent => {
          const eligibility = slotEligibility(agent, slot, details[agent.id])
          const name = agent.displayName || agent.id
          return (
            <div className="agx-candidate" key={agent.id}>
              <button
                type="button"
                disabled={!eligibility.eligible}
                aria-label={`Assign ${name}`}
                onClick={() => eligibility.eligible && onAssign(formation, slot, agent, eligibility.harness)}
              >
                {name}
              </button>
              <span>{eligibility.eligible ? eligibility.harness : eligibility.reason}</span>
            </div>
          )
        })}
      </section>
    </section>
  )
}

function slotEligibility(agent: RosterAgent, slot: FormationSlot, detail?: CachedPersona): { eligible: true; harness: string } | { eligible: false; reason: string } {
  if (agent.unbound) return { eligible: false, reason: 'unbound session (no persona)' }
  if (detail?.error) return { eligible: false, reason: 'failed detail load' }
  if (detail?.card?.status === 'retired') return { eligible: false, reason: 'retired persona' }
  if (!agent.assignable) return { eligible: false, reason: 'not assignable' }
  const requiredHarness = slot.harness || ''
  if (requiredHarness) {
    if (!detail) return { eligible: false, reason: 'loading detail' }
    if (!detail.card) return { eligible: false, reason: 'failed detail load' }
    const hasHarness = detail.card.harnessVariants.some(variant => variant.id === requiredHarness)
    if (!hasHarness) return { eligible: false, reason: `missing harness variant ${requiredHarness}` }
    return { eligible: true, harness: requiredHarness }
  }
  return { eligible: true, harness: detail?.card?.harnessDefault || agent.harnessDefault || 'claude-code' }
}

function RunBanner({
  status,
  missionTitle,
  selectedMissionTitle,
  missionMismatch,
  events,
  openGateId,
  resumeAllowed,
  onViewMission,
  onVerdict,
  onResume,
  onAbort,
}: {
  status: RunStatusProjection
  missionTitle: string
  selectedMissionTitle: string
  missionMismatch: boolean
  events: RunEvent[]
  openGateId: string
  resumeAllowed: boolean
  onViewMission: () => void
  onVerdict: (verdict: 'pass' | 'fail') => void
  onResume: () => void
  onAbort: () => void
}) {
  const recent = [...events].slice(-2)
  return (
    <section className="run-banner agx-run-banner" data-testid="run-banner">
      <span className={`badge ${status.status}`}>{status.status}</span>
      <span>Run: {missionTitle}</span>
      {missionMismatch && (
        <>
          <span>This run belongs to {missionTitle}, not {selectedMissionTitle}.</span>
          <button type="button" onClick={onViewMission}>View run mission</button>
        </>
      )}
      {openGateId && <span>gate {openGateId}</span>}
      {recent.map(event => <span key={`${event.runId}:${event.seq}`}>{runEventText(event) || event.type}</span>)}
      {openGateId && (
        <>
          <button type="button" onClick={() => onVerdict('pass')}>Pass</button>
          <button type="button" onClick={() => onVerdict('fail')}>Fail</button>
        </>
      )}
      {resumeAllowed && <button type="button" onClick={onResume}>Resume</button>}
      {!status.final && <button type="button" onClick={onAbort}>Stop</button>}
    </section>
  )
}

function CreatePersonaPopover({
  draft,
  onDraft,
  onSubmit,
  onClose,
}: {
  draft: CreateDraft
  onDraft: (draft: CreateDraft) => void
  onSubmit: (event: FormEvent) => void
  onClose: () => void
}) {
  const set = (key: keyof CreateDraft, value: string) => onDraft({ ...draft, [key]: value })
  return (
    <div className="pop agx-pop" role="dialog" aria-label="Create persona">
      <div className="pop-head">
        <span className="pt">Create persona</span>
        <button className="x" type="button" onClick={onClose} aria-label="Close">
          <X size={16} aria-hidden="true" />
        </button>
      </div>
      <form className="pop-body" onSubmit={onSubmit}>
        <label htmlFor="agx-create-id">Agent id</label>
        <input id="agx-create-id" className="f" value={draft.id} onChange={event => set('id', event.target.value)} />
        <label htmlFor="agx-create-display">Display name</label>
        <input id="agx-create-display" className="f" value={draft.displayName} onChange={event => set('displayName', event.target.value)} />
        <label htmlFor="agx-create-kind">Kind</label>
        <input id="agx-create-kind" className="f" value={draft.kind} onChange={event => set('kind', event.target.value)} />
        <label htmlFor="agx-create-harness">Harness</label>
        <select id="agx-create-harness" className="f" value={draft.harness} onChange={event => set('harness', event.target.value)}>
          <option value="claude-code">claude-code</option>
          <option value="openai-codex">openai-codex</option>
          <option value="hermes">hermes</option>
        </select>
        <label htmlFor="agx-create-stem">Session stem</label>
        <input id="agx-create-stem" className="f" value={draft.sessionStem} onChange={event => set('sessionStem', event.target.value)} />
        <label htmlFor="agx-create-summary">Summary</label>
        <input id="agx-create-summary" className="f" value={draft.summary} onChange={event => set('summary', event.target.value)} />
        <label htmlFor="agx-create-launch">Launch</label>
        <input id="agx-create-launch" className="f" value={draft.launch} onChange={event => set('launch', event.target.value)} />
        <label htmlFor="agx-create-source">Source</label>
        <input id="agx-create-source" className="f" value={draft.source} onChange={event => set('source', event.target.value)} />
        <label htmlFor="agx-create-capabilities">Capabilities</label>
        <input id="agx-create-capabilities" className="f" value={draft.capabilities} onChange={event => set('capabilities', event.target.value)} />
        <button className="save" type="submit">Create persona</button>
      </form>
    </div>
  )
}

function KeyValue({ label, value }: { label: string; value: string }) {
  if (!value) return null
  return (
    <div className="agx-kv">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function TagList({ tags }: { tags: string[] }) {
  if (tags.length === 0) return null
  return (
    <div className="agx-chip-row">
      {tags.map(tag => <span className="agx-chip" key={tag}>{tag}</span>)}
    </div>
  )
}

function findSlot(board: BoardDocument | null, formationId: string, slotId: string): { formation: FormationNode; slot: FormationSlot } | null {
  const formation = board?.formations.find(next => next.id === formationId)
  const slot = formation?.slots.find(next => next.id === slotId)
  return formation && slot ? { formation, slot } : null
}

function splitCommaList(value: string): string[] {
  return value.split(',').map(part => part.trim()).filter(Boolean)
}

function inferHarnessFromSession(value: string): string {
  const lower = value.toLowerCase()
  if (lower.includes('hermes')) return 'hermes'
  if (lower.includes('codex') || lower.includes('openai')) return 'openai-codex'
  if (lower.includes('claude')) return 'claude-code'
  return EMPTY_CREATE.harness
}
