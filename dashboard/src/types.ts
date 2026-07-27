// Chrote Dashboard Types

// tmux appearance customization (hot-reloadable)
export interface TmuxAppearance {
  statusBg: string           // Status bar background color (e.g., "#000000", "black")
  statusFg: string           // Status bar foreground color (e.g., "#00ff41", "green")
  paneBorderActive: string   // Active pane border color
  paneBorderInactive: string // Inactive pane border color
  modeStyleBg: string        // Copy-mode/selection background
  modeStyleFg: string        // Copy-mode/selection foreground
}

export const DEFAULT_TMUX_APPEARANCE: TmuxAppearance = {
  statusBg: 'default',
  statusFg: '#6b9fff',
  paneBorderActive: '#6b9fff',
  paneBorderInactive: '#3a3a3a',
  modeStyleBg: '#6b9fff',
  modeStyleFg: '#0f0f0f',
}

// tmux appearance presets matching dashboard themes
export const TMUX_PRESETS: Record<string, TmuxAppearance> = {
  matrix: {
    statusBg: 'default',
    statusFg: '#00ff41',
    paneBorderActive: '#00ff41',
    paneBorderInactive: '#333333',
    modeStyleBg: '#00ff41',
    modeStyleFg: '#000000',
  },
  dark: {
    ...DEFAULT_TMUX_APPEARANCE,
  },
  gastown: {
    statusBg: 'default',
    statusFg: '#f6cd54',
    paneBorderActive: '#f6cd54',
    paneBorderInactive: '#4a2518',
    modeStyleBg: '#f6cd54',
    modeStyleFg: '#32160f',
  },
}

// Terminal workspaces and launch-user settings
export type WorkspaceId = 'terminal1' | 'terminal2' | 'terminal3'

export const TERMINAL_WORKSPACE_IDS = ['terminal1', 'terminal2', 'terminal3'] as const

export const TERMINAL_LABELS: Record<WorkspaceId, string> = {
  terminal1: 'Terminal',
  terminal2: 'Terminal 2',
  terminal3: 'Terminal 3',
}

export type LaunchUser = string

const SESSION_KEY_SEPARATOR = ':'

export function getSessionKey(sessionName: string, unixUser?: LaunchUser): string {
  const user = unixUser?.trim()
  return user ? `${encodeURIComponent(user)}${SESSION_KEY_SEPARATOR}${sessionName}` : sessionName
}

export function getSessionNameFromKey(sessionKey: string): string {
  const separator = sessionKey.indexOf(SESSION_KEY_SEPARATOR)
  return separator === -1 ? sessionKey : sessionKey.slice(separator + 1)
}

export function getSessionUserFromKey(sessionKey: string): LaunchUser {
  const separator = sessionKey.indexOf(SESSION_KEY_SEPARATOR)
  if (separator === -1) return ''
  return decodeURIComponent(sessionKey.slice(0, separator))
}

export const DEFAULT_TERMINAL_SESSION_PREFIXES: Record<LaunchUser, string> = {}

export const DEFAULT_TERMINAL_LAUNCH_USERS: Record<WorkspaceId, LaunchUser> = {
  terminal1: '',
  terminal2: '',
  terminal3: '',
}

export const TERMINAL_USER_COLOR_PALETTE = [
  '#4a9eff',
  '#ffb347',
  '#8bd450',
  '#c084fc',
  '#ff6b9d',
  '#45d6d6',
] as const

export const DEFAULT_TERMINAL_USER_COLORS: Record<LaunchUser, string> = {}

export function normalizeTerminalUsers(users: readonly string[] | undefined): LaunchUser[] {
  const seen = new Set<string>()
  return (users ?? [])
    .map(user => user.trim())
    .filter(user => {
      if (!user || seen.has(user)) return false
      seen.add(user)
      return true
    })
}

export function getDefaultLaunchUser(workspaceId: WorkspaceId, terminalUsers: readonly string[] | undefined): LaunchUser {
  const users = normalizeTerminalUsers(terminalUsers)
  if (workspaceId === 'terminal3' && users[1]) return users[1]
  return users[0] ?? ''
}

export function resolveLaunchUser(settings: UserSettings, workspaceId: WorkspaceId, terminalUsers: readonly string[] | undefined): LaunchUser {
  const users = normalizeTerminalUsers(terminalUsers)
  const configured = settings.terminalLaunchUsers[workspaceId]?.trim() ?? ''
  if (configured && users.includes(configured)) return configured
  return getDefaultLaunchUser(workspaceId, users)
}

export function defaultSessionPrefixForUser(user: LaunchUser, terminalUsers: readonly string[] | undefined): string {
  const users = normalizeTerminalUsers(terminalUsers)
  const trimmed = user.trim()
  if (!trimmed) return 'shell'
  return users[0] === trimmed ? 'shell' : trimmed
}

export function getSessionPrefixForUser(settings: UserSettings, user: LaunchUser, terminalUsers: readonly string[] | undefined): string {
  const stored = settings.terminalSessionPrefixes[user]?.trim()
  if (stored) return stored
  const users = normalizeTerminalUsers(terminalUsers)
  const legacy = settings.defaultSessionPrefix?.trim()
  if (legacy && users[0] === user.trim()) return legacy
  return defaultSessionPrefixForUser(user, users)
}

export function getTerminalUserInitial(user: LaunchUser): string {
  return Array.from(user.trim())[0]?.toUpperCase() ?? '?'
}

export function defaultTerminalUserColor(user: LaunchUser): string {
  const name = user.trim()
  if (!name) return TERMINAL_USER_COLOR_PALETTE[0]
  let hash = 0
  for (const char of name) hash = ((hash << 5) - hash) + char.charCodeAt(0)
  return TERMINAL_USER_COLOR_PALETTE[Math.abs(hash) % TERMINAL_USER_COLOR_PALETTE.length]
}

export function getTerminalUserColor(settings: UserSettings, user: LaunchUser): string {
  return settings.terminalUserColors[user] || defaultTerminalUserColor(user)
}

// User settings for persistent configuration
export interface UserSettings {
  terminalMode: 'tmux'              // Terminal mode (tmux only)
  fontSize: number                   // Terminal font size (12-20)
  theme: 'matrix' | 'dark' | 'gastown' // Color theme
  autoRefreshInterval: number        // Session refresh interval in ms (1000-30000)
  defaultSessionPrefix: string       // Legacy fallback for stored settings before per-user prefixes
  terminalSessionPrefixes: Record<LaunchUser, string> // Prefix for new sessions per Unix user
  terminalLaunchUsers: Record<WorkspaceId, LaunchUser> // Unix user for new shells per terminal tab
  terminalLabels: Partial<Record<WorkspaceId, string>> // Optional terminal tab display labels
  terminalUserColors: Record<LaunchUser, string> // Session panel badge colors per Unix user
  musicVolume: number                // Music volume (0-1)
  musicEnabled: boolean              // Whether music is playing
  tmuxAppearance: TmuxAppearance     // tmux color customization
  mouseScroll: boolean               // tmux mouse mode: scroll wheel scrolls history
  hideScrollbar: boolean             // Hide the dead xterm scrollbar gutter in terminal iframes
  beadsProjectPaths?: string[]       // Manually added beads project paths
  formationsTextSize?: FormationsTextSize // Formations cockpit text scale
}

export type FormationsTextSize = 'default' | 'large' | 'xlarge'

export function resolveFormationsTextSize(value: unknown): FormationsTextSize {
  return value === 'large' || value === 'xlarge' ? value : 'default'
}

export const DEFAULT_SETTINGS: UserSettings = {
  terminalMode: 'tmux',
  fontSize: 14,
  theme: 'dark',
  autoRefreshInterval: 5000,
  defaultSessionPrefix: 'shell',
  terminalSessionPrefixes: DEFAULT_TERMINAL_SESSION_PREFIXES,
  terminalLaunchUsers: DEFAULT_TERMINAL_LAUNCH_USERS,
  terminalLabels: {},
  terminalUserColors: DEFAULT_TERMINAL_USER_COLORS,
  musicVolume: 0.5,
  musicEnabled: false,
  tmuxAppearance: DEFAULT_TMUX_APPEARANCE,
  mouseScroll: true,
  hideScrollbar: true,
  formationsTextSize: 'default',
}

export interface TmuxSession {
  id?: string
  name: string
  windows: number
  attached: boolean
  group: string
  unixUser?: LaunchUser
  persistent?: boolean
  persistentIdentity?: string
  persistentAgentKind?: 'codex' | 'claude' | string
  persistentAgentSessionId?: string
  persistentHermesProfile?: string
  persistentResumeCommand?: string
  persistentState?: PersistentAgentState
  persistentConsecutiveLaunchFailures?: number
  persistentNextRetryAt?: string
  persistentLastCheckAt?: string
  persistentLastRestartAt?: string
  persistentLastError?: string
}

export interface PersistentAgentPayload {
  identity?: string
  agentKind?: 'codex' | 'claude' | string
  agentSessionId?: string
  newName?: string
  cwd?: string
  transcriptPath?: string
  recoveryDescriptor?: WorkloadRecoveryDescriptor
}

export type PersistentAgentState =
  | 'starting'
  | 'healthy'
  | 'needs_interaction'
  | 'wrong_identity'
  | 'backoff'
  | 'failed'

export type WorkloadRecoveryMode =
  | 'topology'
  | 'agent'
  | 'command'
  | 'managed'
  | 'unresolved'

export type WorkloadRecoveryOwnerKind =
  | 'session_bank'
  | 'persistent_agent'
  | 'external_manager'

export type WorkloadRecoveryEvidenceSource =
  | 'argv'
  | 'transcript'
  | 'state_db'
  | 'topology'
  | 'manager'
  | 'process'

export type WorkloadRecoveryConfidence = 'high' | 'medium' | 'low'

export interface WorkloadRecoveryOwner {
  kind: WorkloadRecoveryOwnerKind
  ref: string
  mayRestart: boolean
}

export interface WorkloadRecoveryTopology {
  sessionName?: string
  sessionId?: string
  windowIndex: number
  windowName?: string
  windowLayout?: string
  paneIndex: number
  paneId?: string
  paneCurrentPath?: string
}

export interface WorkloadRecoveryAgent {
  kind: 'codex' | 'claude' | 'hermes' | string
  nativeSessionId: string
  hermesProfile?: string
}

export interface PythonHTTPServerRecoveryCommand {
  bind: string
  port: number
  directory: string
}

export interface WorkloadRecoveryCommand {
  kind: 'python-http-server' | string
  pythonHTTPServer?: PythonHTTPServerRecoveryCommand
}

export interface WorkloadRecoveryDescriptor {
  mode: WorkloadRecoveryMode
  owner: WorkloadRecoveryOwner
  topology: WorkloadRecoveryTopology
  workloadKind: string
  agent?: WorkloadRecoveryAgent
  command?: WorkloadRecoveryCommand
  evidenceSource: WorkloadRecoveryEvidenceSource
  confidence: WorkloadRecoveryConfidence
  unresolvedReason?: string
}

export interface ManagedRecoveryHealthStatus {
  ok: boolean
  activeState: string
  checkedAt: string
}

export interface ManagedRecoveryStatusEntry {
  name: string
  sessionName: string
  unixUser?: LaunchUser
  owner: WorkloadRecoveryOwner
  managerKind: string
  managerRef: string
  status: ManagedRecoveryHealthStatus
  storageKind: string
  sourceKind: string
}

export type SendToSessionPayload = {
  text: string
  files: File[]
  submit: boolean
} & ({
  pane: string
  sessionId: string
  panePid: string
  serverPid: string
} | {
  pane?: undefined
  sessionId?: undefined
  panePid?: undefined
  serverPid?: undefined
})

export type SendToSessionOutcome = 'sent' | 'failed' | 'unknown'

export interface SendSessionPane {
  sessionId: string
  pane: string
  panePid: string
  serverPid: string
  windowId?: string
  windowName?: string
  currentPath?: string
  currentCommand?: string
  active: boolean
}

export type SendToSessionResult = {
  success: true
  transport: 'pasted'
  session: string
  sessionId: string
  pane: string
  panePid: string
  serverPid: string
  unixUser: string
  submissionRequested: boolean
  submitted: boolean
  bufferCleaned: boolean
  targetVerified: boolean
  warning: string
} | {
  success: false
  transport: 'unknown'
  retryable: false
  deliveryConfirmed: false
  session: string
  sessionId: string
  pane: string
  panePid: string
  serverPid: string
  unixUser: string
  submissionRequested: boolean
  submitted: false
  bufferCleaned: boolean
  targetVerified: false
  warning: string
}

export interface SessionBankEntry extends TmuxSession {
  live: boolean
  firstSeen: string
  lastSeen: string
  recoveryKind?: 'agent' | 'shell' | 'topology' | 'unresolved' | 'descriptor-plan' | string
  agentKind?: 'codex' | 'claude' | string
  agentSessionId?: string
  resumeCommand?: string
  cwd?: string
  transcriptPath?: string
  recoveryPlan?: WorkloadRecoveryDescriptor[]
}

export interface SessionsResponse {
  sessions: TmuxSession[]
  grouped: Record<string, TmuxSession[]>
  banked?: SessionBankEntry[]
  managed?: ManagedRecoveryStatusEntry[]
  terminalUsers?: LaunchUser[]
  timestamp: string
  error?: string
}

export interface TerminalWindow {
  id: string
  boundSessions: string[] // Session names bound to this window
  activeSession: string | null // Currently displayed session
  colorIndex: number // 0-3 for window color theme
}

export interface TerminalWorkspace {
  windows: TerminalWindow[]
  windowCount: number // 1-4
}

export interface WindowRevealRequest {
  workspaceId: WorkspaceId
  windowId: string
  requestId: number
}

export interface CreateSessionAttachTarget {
  workspaceId: WorkspaceId
  windowId: string
}

export interface CreateSessionOptions {
  workspaceId?: WorkspaceId
  unixUser?: LaunchUser
  name?: string
  mouseScroll?: boolean
  attachTo?: CreateSessionAttachTarget
}

// Layout preset for saving/restoring window configurations
export interface LayoutPreset {
  id: string
  name: string
  createdAt: number
  workspaces: Record<WorkspaceId, TerminalWorkspace>
}

export const MAX_PRESETS = 10

export interface DashboardState {
  // Session data from API
  sessions: TmuxSession[]
  groupedSessions: Record<string, TmuxSession[]>
  sessionBank: SessionBankEntry[]
  managedSessions: ManagedRecoveryStatusEntry[]
  loading: boolean
  error: string | null

  // Window configuration
  workspaces: Record<WorkspaceId, TerminalWorkspace>

  // UI state
  sidebarCollapsed: boolean
  floatingSession: string | null // Session shown in floating modal
  sendToSessionTarget: string | null // Session targeted by the Send to Session modal
  sendToSessionPrefill: string // Optional caller-provided draft for the current Send modal opening
  sendToSessionRequestId: number // Distinguishes deliberate reopenings of the same target

  // Computed: which sessions are assigned to any window
  assignedSessions: Map<string, { workspaceId: WorkspaceId; windowId: string; colorIndex: number; windowIndex: number }>

  // User settings
  settings: UserSettings

  // Terminal users exposed by the server for user-scoped session controls
  terminalUsers: LaunchUser[]

  // Focused window for keyboard navigation (workspaceId-windowId format)
  focusedWindowKey: string | null

  // Canonical window requested for reveal; TerminalArea consumes this for mobile selection.
  windowRevealRequest: WindowRevealRequest | null

  // Layout presets
  layoutPresets: LayoutPreset[]
}

export interface DashboardActions {
  // Window management
  setWindowCount: (workspaceId: WorkspaceId, count: number) => void
  clearWorkspaceAssignments: (workspaceId: WorkspaceId) => void
  clearStaleSessionsFromWindow: (workspaceId: WorkspaceId, windowId: string) => void
  addSessionToWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string, unixUser?: LaunchUser) => void
  removeSessionFromWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  setActiveSession: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  cycleSession: (workspaceId: WorkspaceId, windowId: string, direction: 'prev' | 'next') => void

  // UI actions
  toggleSidebar: () => void
  openFloatingModal: (sessionName: string) => void
  closeFloatingModal: () => void
  openSendToSession: (sessionName: string, prefill?: string) => void
  closeSendToSession: () => void
  listSessionPanes: (sessionName: string, unixUser?: LaunchUser) => Promise<SendSessionPane[] | null>
  sendToSession: (sessionName: string, payload: SendToSessionPayload, unixUser?: LaunchUser) => Promise<SendToSessionOutcome>

  // Session row clicks always preview; assignment navigation is an explicit secondary action.
  handleSessionClick: (sessionName: string) => void
  focusSessionAssignment: (sessionName: string) => void

  // Refresh sessions from API
  refreshSessions: () => Promise<void>

  // Create a tmux session, optionally attaching it to a terminal window
  createSession: (options?: CreateSessionOptions) => Promise<string | null>

  // Delete a session
  deleteSession: (sessionName: string, unixUser?: LaunchUser) => Promise<boolean>

  // Rename a session
  renameSession: (oldName: string, newName: string, unixUser?: LaunchUser) => Promise<boolean>

  // Mark a live Codex/Claude session as persistent desired state, or make it mortal again
  makeSessionPersistent: (sessionName: string, payload: PersistentAgentPayload, unixUser?: LaunchUser) => Promise<boolean>
  makeSessionMortal: (sessionName: string, unixUser?: LaunchUser) => Promise<boolean>

  // Settings
  updateSettings: (settings: Partial<UserSettings>) => void

  // Focus tracking for keyboard navigation
  setFocusedWindowKey: (key: string | null) => void
  revealWindow: (workspaceId: WorkspaceId, windowId: string) => void

  // Layout presets
  saveCurrentLayout: (name: string) => boolean
  loadPreset: (presetId: string) => void
  deletePreset: (presetId: string) => void
  renamePreset: (presetId: string, newName: string) => void
}

export type DashboardContextType = DashboardState & DashboardActions

// Window color themes
export const WINDOW_COLORS = [
  { name: 'blue', bg: 'rgba(10, 10, 26, 0.85)', border: '#4a9eff', accent: '#4a9eff' },
  { name: 'purple', bg: 'rgba(15, 10, 26, 0.85)', border: '#9966ff', accent: '#9966ff' },
  { name: 'green', bg: 'rgba(10, 26, 10, 0.85)', border: '#00ff41', accent: '#00ff41' },
  { name: 'orange', bg: 'rgba(26, 20, 10, 0.85)', border: '#ff9933', accent: '#ff9933' },
] as const

// Group display names and priorities
export const GROUP_CONFIG: Record<string, { displayName: string; priority: number }> = {
  'hq': { displayName: 'HQ', priority: 0 },
  'main': { displayName: 'Main', priority: 1 },
  'other': { displayName: 'Other', priority: 100 },
}

export function getGroupDisplayName(group: string): string {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].displayName
  if (group.startsWith('gt-')) {
    // gt-gastown → Gastown
    const rigName = group.slice(3)
    return rigName.charAt(0).toUpperCase() + rigName.slice(1)
  }
  return group
}

export function getGroupPriority(group: string): number {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].priority
  if (group.startsWith('gt-')) return 3 // Rigs after main
  return 99
}
