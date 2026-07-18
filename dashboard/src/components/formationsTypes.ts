export interface BoardSummary {
  id: string
  slug: string
  title: string
  rev: number
  etag: string
}

export interface FormationPort {
  id: string
  label: string
}

export interface FormationSlot {
  id: string
  label: string
  controller: boolean
  agentId?: string
  harness?: string
}

export interface FormationBrief {
  goal?: string
  beadId?: string
  files?: string[]
  links?: string[]
}

export interface FormationVerification {
  id?: string
  kinds?: string[]
  criterion?: string
  onFail?: string
}

export interface FormationNode {
  id: string
  type: 'solo' | 'peer' | 'flow' | 'orchestrated'
  title: string
  brief?: FormationBrief
  inputs: FormationPort[]
  outputs: FormationPort[]
  slots: FormationSlot[]
  verification?: FormationVerification
}

export interface BoardConnection {
  id: string
  from: string
  to: string
}

export interface BoardDocument {
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

export interface MissionNode {
  id: string
  title: string
  goal: string
  beadId: string
}

export interface GateNode {
  id: string
  title: string
  kinds: string[]
  criterion: string
}

export interface RunStatusProjection {
  runId: string
  status: string
  final: boolean
  boardSlug: string
  missionId: string
  eventCount: number
  resumeAllowed?: boolean
}

export interface RunEvent {
  seq: number
  type: string
  runId: string
  nodeId?: string
  gateId?: string
  data?: Record<string, unknown>
}

export interface RunStartResult {
  runId: string
  status: RunStatusProjection
}

export interface RunStatusResult {
  status: RunStatusProjection
}

export interface LayoutNode {
  id: string
  x: number
  y: number
}

export interface LayoutEdge {
  id: string
  lane: string
}

export interface LayoutDocument {
  boardId: string
  boardRev: number
  etag: string
  nodes: LayoutNode[]
  edges?: LayoutEdge[]
}

export interface ViewTransform {
  x: number
  y: number
  scale: number
}

export interface ContextMenuItem {
  label: string
  action?: () => void
  destructive?: boolean
  disabled?: boolean
}

export interface ContextMenuState {
  label: string
  x: number
  y: number
  items: ContextMenuItem[]
}

export interface AgentProjection {
  id: string
  displayName?: string
  harnessDefault?: string
  assignable: boolean
  unbound?: boolean
  liveness?: string
}

export interface BriefDraft {
  goal: string
  beadId: string
  files: string
  links: string
}

export interface TerminalPopup {
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

export type WireDragState =
  | { kind: 'new'; from: string }
  | { kind: 'reconnect-target'; connection: BoardConnection }
  | { kind: 'judge'; gateId: string }

export type UndoAction =
  | { kind: 'deleteFormation'; formationId: string }
  | { kind: 'deleteGate'; gateId: string }
  | { kind: 'deleteMission'; missionId: string }
  | { kind: 'assignSlot'; formationId: string; slotId: string; agentId: string; harness: string }
  | { kind: 'makeController'; formationId: string; slotId: string }
  | { kind: 'setBrief'; formationId: string; brief?: FormationBrief }
  | { kind: 'removePort'; formationId: string; portId: string }
  | { kind: 'unwireConnection'; from: string; to: string }
  | { kind: 'moveNode'; node: LayoutNode }

export type BoardUndoAction = Exclude<UndoAction, { kind: 'moveNode'; node: LayoutNode }>
