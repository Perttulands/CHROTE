import type {
  SessionBankEntry,
  WorkloadRecoveryDescriptor,
  WorkloadRecoveryOwner,
  WorkloadRecoveryOwnerKind,
} from './types'

export type SessionBankRecoveryCapabilityKind =
  | 'workload-recoverable'
  | 'topology-only'
  | 'externally-managed'
  | 'unresolved-unsafe'
  | 'legacy-no-plan'

export interface SessionBankRecoveryCapability {
  kind: SessionBankRecoveryCapabilityKind
  badgeLabel: string
  description: string
  canRecoverWorkload: boolean
  canRestoreTopologyOnly: boolean
  isReadOnly: boolean
  descriptorCount: number
  owner?: WorkloadRecoveryOwner
  unresolvedReasons: string[]
}

export interface SessionBankCapabilitySummary {
  total: number
  workloadRecoverable: number
  topologyOnly: number
  externallyManaged: number
  unresolvedUnsafe: number
  legacyNoPlan: number
}

const RECOVERY_MODES = new Set(['topology', 'agent', 'command', 'managed', 'unresolved'])
const OWNER_KINDS = new Set(['session_bank', 'persistent_agent', 'external_manager'])
const EVIDENCE_SOURCES = new Set(['argv', 'transcript', 'state_db', 'topology', 'manager', 'process'])
const CONFIDENCE_LEVELS = new Set(['high', 'medium', 'low'])
const AGENT_KINDS = new Set(['codex', 'claude', 'hermes'])
const UNRESOLVED_REASONS = new Set([
  'unknown_process',
  'ambiguous_candidates',
  'unsafe_evidence',
  'unsupported_workload',
  'missing_evidence',
  'conflicting_owners',
  'conflicting_evidence',
])
const LOOPBACK_BINDS = new Set(['127.0.0.1', 'localhost', '::1'])

const UUID_RE = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/
const NATIVE_ID_RE = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/
const HERMES_PROFILE_RE = /^[a-z][a-z0-9_-]{0,63}$/
const SAFE_REF_RE = /^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,239}$/

interface DescriptorValidation {
  owner?: WorkloadRecoveryOwner
  ownerKey?: string
  targetKey?: string
  windowIndex?: number
  windowName?: string
  windowLayout?: string
  paneIndex?: number
  paneId?: string
  reasons: string[]
}

interface RecoveryPlanRead {
  present: boolean
  descriptors: WorkloadRecoveryDescriptor[]
  malformedReasons: string[]
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
}

function isAbsoluteSafePath(value: unknown): boolean {
  const path = text(value)
  return path.startsWith('/') && !/[\0\n\r#]/.test(path)
}


function readRecoveryPlan(entry: SessionBankEntry): RecoveryPlanRead {
  const plan = (entry as { recoveryPlan?: unknown }).recoveryPlan
  if (plan === undefined) {
    return { present: false, descriptors: [], malformedReasons: [] }
  }
  if (!Array.isArray(plan)) {
    return { present: true, descriptors: [], malformedReasons: ['malformed_recovery_plan'] }
  }
  if (plan.length === 0) {
    return { present: true, descriptors: [], malformedReasons: ['empty_recovery_plan'] }
  }

  const descriptors = plan.filter(isRecord) as unknown as WorkloadRecoveryDescriptor[]
  const malformedReasons = descriptors.length === plan.length ? [] : ['malformed_recovery_descriptor']
  return { present: true, descriptors, malformedReasons }
}

function firstOwner(descriptors: WorkloadRecoveryDescriptor[]): WorkloadRecoveryOwner | undefined {
  return descriptors.find(descriptor => isRecord(descriptor.owner))?.owner
}

function expectedSessionBankOwnerRef(entry: SessionBankEntry): string {
  const user = entry.unixUser?.trim()
  return user ? `${user}/${entry.name}` : entry.name
}

function ownerFromDescriptor(descriptor: WorkloadRecoveryDescriptor): WorkloadRecoveryOwner | undefined {
  const owner = descriptor.owner
  if (!isRecord(owner)) return undefined
  const kind = text(owner.kind)
  const ref = text(owner.ref)
  if (!OWNER_KINDS.has(kind) || !SAFE_REF_RE.test(ref) || typeof owner.mayRestart !== 'boolean') {
    return undefined
  }
  return {
    kind: kind as WorkloadRecoveryOwnerKind,
    ref,
    mayRestart: owner.mayRestart,
  }
}

function isExternallyManagedDescriptor(descriptor: WorkloadRecoveryDescriptor): boolean {
  const owner = ownerFromDescriptor(descriptor)
  return text(descriptor.mode) === 'managed' ||
    owner?.kind === 'external_manager' ||
    owner?.kind === 'persistent_agent'
}

function unresolvedReason(descriptor: WorkloadRecoveryDescriptor): string {
  return text(descriptor.unresolvedReason) ||
    text(descriptor.evidenceSource) ||
    text(descriptor.mode) ||
    'unsafe_evidence'
}

function externalOwnerCapability(descriptors: WorkloadRecoveryDescriptor[]): SessionBankRecoveryCapability {
  return {
    kind: 'externally-managed',
    badgeLabel: 'Managed read-only',
    description: 'Read-only: another manager owns this workload.',
    canRecoverWorkload: false,
    canRestoreTopologyOnly: false,
    isReadOnly: true,
    descriptorCount: descriptors.length,
    owner: descriptors.find(isExternallyManagedDescriptor)?.owner ?? firstOwner(descriptors),
    unresolvedReasons: [],
  }
}

function validateOwner(descriptor: WorkloadRecoveryDescriptor, entry: SessionBankEntry, reasons: string[]): WorkloadRecoveryOwner | undefined {
  const owner = ownerFromDescriptor(descriptor)
  if (!owner) {
    reasons.push('recovery_owner_malformed')
    return undefined
  }

  const mode = text(descriptor.mode)
  if (mode === 'managed') {
    if (owner.kind !== 'external_manager' || owner.mayRestart) {
      reasons.push('managed_owner_must_be_external_read_only')
    }
    return owner
  }

  if (owner.kind !== 'session_bank') {
    reasons.push('recovery_owner_must_be_session_bank')
  }
  if (owner.ref !== expectedSessionBankOwnerRef(entry)) {
    reasons.push(`recovery_owner_ref_mismatch:${owner.ref}`)
  }

  if (mode === 'unresolved') {
    if (owner.mayRestart) reasons.push('unresolved_owner_must_not_restart')
  } else if (!owner.mayRestart) {
    reasons.push('session_bank_owner_cannot_restart')
  }

  return owner
}

interface TopologyValidation {
  targetKey?: string
  windowIndex?: number
  windowName?: string
  windowLayout?: string
  paneIndex?: number
  paneId?: string
}

function validateTopology(descriptor: WorkloadRecoveryDescriptor, entry: SessionBankEntry, reasons: string[]): TopologyValidation {
  const topology = descriptor.topology
  if (!isRecord(topology)) {
    reasons.push('topology_malformed')
    return {}
  }

  const sessionName = text(topology.sessionName)
  if (sessionName !== entry.name) {
    reasons.push(`topology_session_mismatch:${sessionName || '<empty>'}`)
  }

  const windowIndex = topology.windowIndex
  const paneIndex = topology.paneIndex
  if (!isNonNegativeInteger(windowIndex)) {
    reasons.push('topology_window_index_invalid')
  }
  if (!isNonNegativeInteger(paneIndex)) {
    reasons.push('topology_pane_index_invalid')
  }
  if (!isAbsoluteSafePath(topology.paneCurrentPath)) {
    reasons.push('topology_pane_cwd_invalid')
  }

  if (isNonNegativeInteger(windowIndex) && isNonNegativeInteger(paneIndex)) {
    return {
      targetKey: `${windowIndex}.${paneIndex}`,
      windowIndex,
      windowName: text(topology.windowName),
      windowLayout: text(topology.windowLayout),
      paneIndex,
      paneId: text(topology.paneId),
    }
  }
  return {}
}

function validateAgent(descriptor: WorkloadRecoveryDescriptor, reasons: string[]): void {
  const agent = descriptor.agent
  if (!isRecord(agent)) {
    reasons.push('agent_malformed')
    return
  }

  const kind = text(agent.kind)
  const nativeSessionId = text(agent.nativeSessionId)
  if (!AGENT_KINDS.has(kind)) {
    reasons.push(`agent_kind_unsupported:${kind || '<empty>'}`)
    return
  }

  if (text(descriptor.workloadKind) !== kind) {
    reasons.push('agent_workload_kind_mismatch')
  }

  if (kind === 'codex' || kind === 'claude') {
    if (!UUID_RE.test(nativeSessionId)) {
      reasons.push('agent_native_session_id_invalid')
    }
    if (text(agent.hermesProfile)) {
      reasons.push('agent_profile_unexpected')
    }
    return
  }

  if (!NATIVE_ID_RE.test(nativeSessionId)) {
    reasons.push('hermes_native_session_id_invalid')
  }
  if (!HERMES_PROFILE_RE.test(text(agent.hermesProfile))) {
    reasons.push('hermes_profile_invalid')
  }
}

function validateCommand(descriptor: WorkloadRecoveryDescriptor, reasons: string[]): void {
  const command = descriptor.command
  if (!isRecord(command)) {
    reasons.push('command_malformed')
    return
  }
  if (text(descriptor.workloadKind) !== 'python-http-server' || text(command.kind) !== 'python-http-server') {
    reasons.push('command_kind_unsupported')
    return
  }

  const pythonHTTPServer = command.pythonHTTPServer
  if (!isRecord(pythonHTTPServer)) {
    reasons.push('python_http_server_malformed')
    return
  }
  if (!LOOPBACK_BINDS.has(text(pythonHTTPServer.bind))) {
    reasons.push('python_http_server_bind_not_loopback')
  }
  if (!isNonNegativeInteger(pythonHTTPServer.port) || pythonHTTPServer.port < 1 || pythonHTTPServer.port > 65535) {
    reasons.push('python_http_server_port_invalid')
  }
  if (!isAbsoluteSafePath(pythonHTTPServer.directory)) {
    reasons.push('python_http_server_directory_invalid')
  }
}

function validateDescriptor(descriptor: WorkloadRecoveryDescriptor, entry: SessionBankEntry): DescriptorValidation {
  const reasons: string[] = []
  const mode = text(descriptor.mode)
  if (!RECOVERY_MODES.has(mode)) {
    reasons.push(`recovery_mode_unsupported:${mode || '<empty>'}`)
  }

  const owner = validateOwner(descriptor, entry, reasons)
  const topology = validateTopology(descriptor, entry, reasons)

  if (!EVIDENCE_SOURCES.has(text(descriptor.evidenceSource))) {
    reasons.push(`evidence_source_unsupported:${text(descriptor.evidenceSource) || '<empty>'}`)
  }
  if (!CONFIDENCE_LEVELS.has(text(descriptor.confidence))) {
    reasons.push(`confidence_unsupported:${text(descriptor.confidence) || '<empty>'}`)
  }

  switch (mode) {
    case 'agent':
      validateAgent(descriptor, reasons)
      break
    case 'command':
      validateCommand(descriptor, reasons)
      break
    case 'topology':
      if (text(descriptor.workloadKind) !== 'shell') {
        reasons.push('topology_workload_kind_must_be_shell')
      }
      break
    case 'managed':
      if (text(descriptor.workloadKind) !== 'managed') {
        reasons.push('managed_workload_kind_invalid')
      }
      break
    case 'unresolved':
      if (text(descriptor.workloadKind) !== 'unknown') {
        reasons.push('unresolved_workload_kind_invalid')
      }
      if (!UNRESOLVED_REASONS.has(text(descriptor.unresolvedReason))) {
        reasons.push(`unresolved_reason_unsupported:${text(descriptor.unresolvedReason) || '<empty>'}`)
      }
      break
  }

  return {
    owner,
    ownerKey: owner ? `${owner.kind}:${owner.ref}` : undefined,
    ...topology,
    reasons,
  }
}

function planConsistencyReasons(validations: DescriptorValidation[]): string[] {
  const reasons: string[] = []
  if (validations.length > 128) {
    reasons.push('recovery_plan_descriptor_count_exceeds_128')
  }

  const ownerKeys = new Set(validations.map(validation => validation.ownerKey).filter(Boolean))
  if (ownerKeys.size > 1) {
    reasons.push('conflicting_recovery_owners')
  }

  const paneIds = new Set<string>()
  const targetKeys = new Set<string>()
  const windows = new Map<number, DescriptorValidation[]>()
  validations.forEach(validation => {
    if (validation.targetKey) {
      if (targetKeys.has(validation.targetKey)) {
        reasons.push(`duplicate_recovery_pane_target:${validation.targetKey}`)
      }
      targetKeys.add(validation.targetKey)
    }

    if (validation.paneId) {
      if (paneIds.has(validation.paneId)) {
        reasons.push(`duplicate_recovery_pane_id:${validation.paneId}`)
      }
      paneIds.add(validation.paneId)
    }

    if (validation.windowIndex === undefined) return
    const panes = windows.get(validation.windowIndex) ?? []
    panes.push(validation)
    windows.set(validation.windowIndex, panes)
  })

  if (windows.size > 32) {
    reasons.push('recovery_plan_window_count_exceeds_32')
  }

  const windowIndexes = Array.from(windows.keys()).sort((a, b) => a - b)
  if (windowIndexes.length > 0) {
    const firstWindowIndex = windowIndexes[0]
    windowIndexes.forEach((windowIndex, ordinal) => {
      if (windowIndex !== firstWindowIndex + ordinal) {
        reasons.push('recovery_windows_not_contiguous')
      }
    })
  }

  windows.forEach((panes, windowIndex) => {
    if (panes.length > 32) {
      reasons.push(`recovery_window_${windowIndex}_pane_count_exceeds_32`)
    }

    const [firstPane] = panes
    panes.slice(1).forEach(pane => {
      if (pane.windowName !== firstPane.windowName) {
        reasons.push(`conflicting_recovery_window_name:${windowIndex}`)
      }
      if (pane.windowLayout !== firstPane.windowLayout) {
        reasons.push(`conflicting_recovery_window_layout:${windowIndex}`)
      }
    })

    const paneIndexes = panes
      .map(pane => pane.paneIndex)
      .filter((paneIndex): paneIndex is number => paneIndex !== undefined)
      .sort((a, b) => a - b)
    if (paneIndexes.length === 0) return
    const firstPaneIndex = paneIndexes[0]
    paneIndexes.forEach((paneIndex, ordinal) => {
      if (paneIndex !== firstPaneIndex + ordinal) {
        reasons.push(`recovery_window_${windowIndex}_panes_not_contiguous`)
      }
    })
  })

  return reasons
}

function uniqueReasons(reasons: string[]): string[] {
  return Array.from(new Set(reasons.filter(Boolean)))
}

function unresolvedCapability(
  descriptors: WorkloadRecoveryDescriptor[],
  reasons: string[],
): SessionBankRecoveryCapability {
  return {
    kind: 'unresolved-unsafe',
    badgeLabel: 'Unresolved / unsafe',
    description: 'Unsafe recovery evidence is kept for cleanup only; restore is disabled.',
    canRecoverWorkload: false,
    canRestoreTopologyOnly: false,
    isReadOnly: false,
    descriptorCount: descriptors.length,
    owner: firstOwner(descriptors),
    unresolvedReasons: uniqueReasons(reasons),
  }
}

export function getSessionBankRecoveryCapability(entry: SessionBankEntry): SessionBankRecoveryCapability {
  const plan = readRecoveryPlan(entry)
  const descriptors = plan.descriptors
  if (!plan.present) {
    return {
      kind: 'legacy-no-plan',
      badgeLabel: 'Legacy no plan',
      description: 'Legacy entry: no typed recovery plan is available.',
      canRecoverWorkload: false,
      canRestoreTopologyOnly: false,
      isReadOnly: false,
      descriptorCount: 0,
      unresolvedReasons: [],
    }
  }

  if (plan.malformedReasons.length > 0) {
    return unresolvedCapability(descriptors, plan.malformedReasons)
  }

  if (descriptors.some(isExternallyManagedDescriptor)) {
    return externalOwnerCapability(descriptors)
  }

  const validations = descriptors.map(descriptor => validateDescriptor(descriptor, entry))
  const reasons = uniqueReasons([
    ...validations.flatMap(validation => validation.reasons),
    ...planConsistencyReasons(validations),
  ])
  if (reasons.length > 0 || descriptors.some(descriptor => text(descriptor.mode) === 'unresolved')) {
    return unresolvedCapability(descriptors, [
      ...reasons,
      ...descriptors
        .filter(descriptor => text(descriptor.mode) === 'unresolved')
        .map(unresolvedReason),
    ])
  }

  if (descriptors.every(descriptor => text(descriptor.mode) === 'topology')) {
    return {
      kind: 'topology-only',
      badgeLabel: 'Topology only',
      description: 'Typed topology is available, but no workload restart contract exists.',
      canRecoverWorkload: false,
      canRestoreTopologyOnly: true,
      isReadOnly: false,
      descriptorCount: descriptors.length,
      owner: firstOwner(descriptors),
      unresolvedReasons: [],
    }
  }

  if (descriptors.some(descriptor => text(descriptor.mode) === 'agent' || text(descriptor.mode) === 'command')) {
    return {
      kind: 'workload-recoverable',
      badgeLabel: 'Workload recoverable',
      description: 'Typed backend-owned workload descriptors are available.',
      canRecoverWorkload: true,
      canRestoreTopologyOnly: true,
      isReadOnly: false,
      descriptorCount: descriptors.length,
      owner: firstOwner(descriptors),
      unresolvedReasons: [],
    }
  }

  return unresolvedCapability(descriptors, ['unsafe_evidence'])
}

export function summarizeSessionBankCapabilities(entries: SessionBankEntry[]): SessionBankCapabilitySummary {
  return entries.reduce((summary, entry) => {
    summary.total += 1
    const capability = getSessionBankRecoveryCapability(entry)
    switch (capability.kind) {
      case 'workload-recoverable':
        summary.workloadRecoverable += 1
        break
      case 'topology-only':
        summary.topologyOnly += 1
        break
      case 'externally-managed':
        summary.externallyManaged += 1
        break
      case 'unresolved-unsafe':
        summary.unresolvedUnsafe += 1
        break
      case 'legacy-no-plan':
        summary.legacyNoPlan += 1
        break
    }
    return summary
  }, {
    total: 0,
    workloadRecoverable: 0,
    topologyOnly: 0,
    externallyManaged: 0,
    unresolvedUnsafe: 0,
    legacyNoPlan: 0,
  })
}
