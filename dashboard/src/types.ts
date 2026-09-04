// Chrote Dashboard Types

// Terminal workspaces and launch-user settings.
// Workspace ids are always terminal1..terminalN and terminalWorkspaceIds() is
// the only derivation. Persisted layouts, dock state, and settings key off
// these ids, so the scheme is load-bearing: existing localStorage must keep
// resolving unchanged at the default count.
export type WorkspaceId = `terminal${number}`

export const DEFAULT_TERMINAL_TAB_COUNT = 3
export const MIN_TERMINAL_TAB_COUNT = 1
export const MAX_TERMINAL_TAB_COUNT = 6

// Single owner of terminalTabCount ingestion: settings merge spreads raw
// stored values into a typed object, so every non-numeric, fractional, or
// out-of-range shape must resolve here, not at the UI write site.
export function normalizeTerminalTabCount(value: unknown): number {
  if (typeof value !== 'number' || Number.isNaN(value)) return DEFAULT_TERMINAL_TAB_COUNT
  return Math.max(MIN_TERMINAL_TAB_COUNT, Math.min(MAX_TERMINAL_TAB_COUNT, Math.floor(value)))
}

export function terminalWorkspaceIds(count: number = DEFAULT_TERMINAL_TAB_COUNT): WorkspaceId[] {
  return Array.from({ length: normalizeTerminalTabCount(count) }, (_, i) => `terminal${i + 1}` as WorkspaceId)
}

export function terminalWorkspaceIndex(workspaceId: WorkspaceId): number {
  return Number(workspaceId.slice('terminal'.length))
}

export function sortTerminalWorkspaceIds(ids: readonly WorkspaceId[]): WorkspaceId[] {
  return [...ids].sort((a, b) => terminalWorkspaceIndex(a) - terminalWorkspaceIndex(b))
}

export const TERMINAL_WORKSPACE_IDS: readonly WorkspaceId[] = terminalWorkspaceIds()

export function isTerminalWorkspaceId(value: unknown, ids: readonly WorkspaceId[] = TERMINAL_WORKSPACE_IDS): value is WorkspaceId {
  return typeof value === 'string' && (ids as readonly string[]).includes(value)
}

export function getTerminalLabel(workspaceId: WorkspaceId): string {
  return workspaceId === 'terminal1' ? 'Terminal' : `Terminal ${workspaceId.slice('terminal'.length)}`
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

export const DEFAULT_TERMINAL_LAUNCH_USERS: Record<WorkspaceId, LaunchUser> = {}

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

// User settings for persistent configuration
export interface UserSettings {
  terminalMode: 'tmux'              // Terminal mode (tmux only)
  terminalTabCount: number           // Visible terminal tabs (1-6); shrinking hides, never deletes
  fontSize: number                   // Terminal font size (12-20)
  autoRefreshInterval: number        // Session refresh interval in ms (1000-30000)
  defaultSessionPrefix: string       // Legacy fallback for stored settings before per-user prefixes
  terminalSessionPrefixes: Record<LaunchUser, string> // Prefix for new sessions per Unix user
  terminalLaunchUsers: Record<WorkspaceId, LaunchUser> // Unix user for new shells per terminal tab
  terminalLabels: Partial<Record<WorkspaceId, string>> // Optional terminal tab display labels
  mouseScroll: boolean               // tmux mouse mode: scroll wheel scrolls history
  hideScrollbar: boolean             // Hide the dead xterm scrollbar gutter in terminals
  keysEnabled: boolean               // Leader chords intercept keys; off hands every key to the pty
  tableWidth: number                 // The table's column at the right, in px; device-local
  agentEventTones: boolean           // A short tone when an agent finishes or needs input; device-local, off
  agentEventNotifications: boolean   // A browser notification for the same while this tab is hidden; device-local, off
  residentWidth: number              // A resident's column, in px; 0 means 44 terminal columns at the tile font
  beadsProjectPaths?: string[]       // Manually added beads project paths
}

export const DEFAULT_SETTINGS: UserSettings = {
  terminalMode: 'tmux',
  terminalTabCount: DEFAULT_TERMINAL_TAB_COUNT,
  fontSize: 14,
  autoRefreshInterval: 5000,
  defaultSessionPrefix: 'shell',
  terminalSessionPrefixes: DEFAULT_TERMINAL_SESSION_PREFIXES,
  terminalLaunchUsers: DEFAULT_TERMINAL_LAUNCH_USERS,
  terminalLabels: {},
  mouseScroll: true,
  hideScrollbar: true,
  keysEnabled: true,
  tableWidth: 400,
  agentEventTones: false,
  agentEventNotifications: false,
  residentWidth: 0,
}

export interface TmuxSession {
  id?: string
  name: string
  windows: number
  attached: boolean
  group: string
  unixUser?: LaunchUser
  cwd?: string
  currentCommand?: string
  /** Panes in the current window. */
  panes?: number
  /** Current window size in cells. */
  width?: number
  height?: number
  /** tmux window-size is manual, so CHROTE cannot resize this session. */
  sizePinned?: boolean
  /** tmux mouse mode. Absent when the server did not report it. */
  mouseEnabled?: boolean
  /** ttys of attached clients CHROTE did not create, such as an SSH login. */
  foreignClients?: string[]
  /** Clients attached to this session, CHROTE's own and foreign alike. */
  viewers?: number
  /**
   * What the agent inside last reported through its own completion hook.
   * Absent when nothing was reported; kept, marked seen, once the operator
   * focused the session.
   */
  lastEvent?: AgentEvent
}

export type AgentEventKind = 'finished' | 'needs-input'

export interface AgentEvent {
  event: AgentEventKind
  /** When the report arrived, RFC 3339 with milliseconds, UTC. */
  time: string
  /** What the harness said about it, if anything. */
  summary?: string
  /** True once the operator focused the session after the event. */
  seen: boolean
}

export type SessionBadgeId = 'pinned-size' | 'foreign-client' | 'shared-view' | 'structure' | 'mouse-off'

export interface SessionBadge {
  id: SessionBadgeId
  /** Compact marker shown in the session list. */
  marker: string
  /** Short name, read out to assistive technology. */
  label: string
  /** The full fact, shown on hover. */
  detail: string
}

/**
 * A badge means this session is not what you would assume from looking at it.
 * That rule is the whole membership test: anything a glance already tells the
 * operator does not belong here, or the set decays into decoration.
 */
export function getSessionBadges(session: TmuxSession): SessionBadge[] {
  const badges: SessionBadge[] = []

  if (session.sizePinned) {
    const size = session.width && session.height ? ` at ${session.width}x${session.height}` : ''
    badges.push({
      id: 'pinned-size',
      marker: '⊡',
      label: 'Fixed size',
      detail: `Pinned${size}. tmux window-size is manual on this window, so CHROTE cannot resize it.`,
    })
  }

  const foreign = session.foreignClients ?? []
  if (foreign.length > 0) {
    const plural = foreign.length === 1 ? 'client' : 'clients'
    badges.push({
      id: 'foreign-client',
      marker: '◈',
      label: 'Foreign client attached',
      detail: `Attached by ${foreign.length} ${plural} CHROTE did not create (${foreign.join(', ')}). Opening this session watches alongside them; Claim takes its size without disconnecting them.`,
    })
  }

  // tmux draws a window once, at one size, however many clients are watching.
  // So a second viewer means this pane is showing somebody else's dimensions,
  // which is exactly the kind of thing a glance cannot tell the operator.
  if ((session.viewers ?? 0) > 1) {
    const size = session.width && session.height ? ` at ${session.width}x${session.height}` : ''
    badges.push({
      id: 'shared-view',
      marker: '◎',
      label: `Watched by ${session.viewers}`,
      detail: `${session.viewers} clients are watching this session. tmux draws it once${size}, so every viewer sees the size the claiming one set. Claim to make it fit this device.`,
    })
  }

  const structure: string[] = []
  if (session.windows > 1) structure.push(`${session.windows} tmux windows`)
  if ((session.panes ?? 1) > 1) structure.push(`${session.panes} panes in the current window`)
  if (structure.length > 0) {
    badges.push({
      id: 'structure',
      marker: '⊞',
      label: 'More than one window or pane',
      detail: `This session has ${structure.join(' and ')}. A terminal shows the current window only.`,
    })
  }

  if (session.mouseEnabled === false) {
    badges.push({
      id: 'mouse-off',
      marker: '⊗',
      label: 'Mouse off',
      detail: 'tmux mouse mode is off for this session, so scrolling and clicking reach the running program instead of tmux.',
    })
  }

  return badges
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

/**
 * What the send did, and what the host said about it. The drawer keeps a
 * failure on screen and prints this message beside the note that failed, so
 * the operator reads the server's own words where he can act on them rather
 * than on a status line under a surface that has already closed.
 */
export interface SendToSessionReport {
  outcome: SendToSessionOutcome
  message: string
}

/**
 * What a surface hands the Send drawer when it opens it.
 *
 * `reference` is the one line the agent can act on — "path /srv/x/y.ts",
 * "bead chrote-abc: title" — and it is shown read-only above the note so it
 * cannot be edited into something that no longer names the thing it came from.
 */
export interface SendToSessionRequest {
  /** The session to preselect; the focused tile's session when absent. */
  targetSessionKey?: string
  /** The one line naming what the operator was looking at. */
  reference?: string
  /** A draft note under the reference. */
  note?: string
  /** A new agent worth offering for this message, when the surface knows one. */
  launch?: SendToSessionLaunch
}

/**
 * The new agent a surface can name for the message it is opening the drawer
 * with. Dev mode knows the complaint belongs to whoever works on CHROTE, so it
 * says so on the picker's row and preselects that agent's harness rather than
 * making the operator assemble it again for every annotation.
 */
export interface SendToSessionLaunch {
  /** What the picker's row reads, in place of the bare "New agent…". */
  label: string
  /** The folder to start in, when the surface knows it. */
  folder?: string
  /** The harness to preselect, by its launch id. */
  harness?: string
}

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
  submitKeyDispatched: boolean
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
  submitKeyDispatched: false
  bufferCleaned: boolean
  targetVerified: false
  warning: string
}

export interface SessionsResponse {
  sessions: TmuxSession[]
  grouped: Record<string, TmuxSession[]>
  terminalUsers?: LaunchUser[]
  timestamp: string
  error?: string
  partial?: boolean
  successfulUsers?: LaunchUser[]
  failedUsers?: LaunchUser[]
}

export interface TerminalWindow {
  id: string
  boundSessions: string[] // Session names bound to this window
  activeSession: string | null // Currently displayed session
  colorIndex: number // 0-3; a stable per-window index kept across the colour cut
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
  /** Folder the session starts in: absolute, or `~`/`~/...` for the user's home. */
  cwd?: string
  /** Harness id from GET /api/launch. Absent or `shell` starts nothing. */
  harness?: string
  /**
   * The flags typed after the harness's binary. Absent leaves the server its
   * configured defaults; an empty string means this launch takes none.
   */
  flags?: string
  /**
   * Whether the harness's completion hooks are installed, so the session
   * reports when its agent finishes or needs input. Absent means yes.
   */
  notify?: boolean
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
  loading: boolean
  error: string | null

  // Unix users whose tmux answered, when the last response was partial. Null
  // when it was whole, failed outright, or has not landed yet. Consumers scope
  // their trust in `sessions` by it rather than discarding the whole list.
  partialAnsweringUsers: LaunchUser[] | null

  // Window configuration
  workspaces: Record<WorkspaceId, TerminalWorkspace>

  // Resolved terminal workspace id list (settings-derived; fixed at the
  // default count until the tab-count setting lands)
  workspaceIds: readonly WorkspaceId[]

  // UI state
  sidebarCollapsed: boolean
  floatingSession: string | null // Session shown in floating modal
  sendToSessionRequest: SendToSessionRequest | null // What the Send drawer was opened with; null while it is closed
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
  addSessionToWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string, unixUser?: LaunchUser) => void
  removeSessionFromWindow: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void
  setActiveSession: (workspaceId: WorkspaceId, windowId: string, sessionName: string) => void

  // UI actions
  toggleSidebar: () => void
  openFloatingModal: (sessionName: string) => void
  closeFloatingModal: () => void
  openSendToSession: (request?: SendToSessionRequest) => void
  closeSendToSession: () => void
  listSessionPanes: (sessionName: string, unixUser?: LaunchUser) => Promise<SendSessionPane[] | null>
  sendToSession: (sessionName: string, payload: SendToSessionPayload, unixUser?: LaunchUser) => Promise<SendToSessionReport>

  // Session row clicks always preview.
  handleSessionClick: (sessionName: string) => void

  // Refresh sessions from API
  refreshSessions: () => Promise<void>

  // Create a tmux session, optionally attaching it to a terminal window
  createSession: (options?: CreateSessionOptions) => Promise<string | null>

  // Recreate an ended binding's session in place, under the same name and tile
  restartSession: (workspaceId: WorkspaceId, windowId: string, sessionKey: string) => Promise<boolean>

  // Delete a session
  deleteSession: (sessionName: string, unixUser?: LaunchUser) => Promise<boolean>

  // Rename a session
  renameSession: (oldName: string, newName: string, unixUser?: LaunchUser) => Promise<boolean>

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

// Group display names and priorities
export const GROUP_CONFIG: Record<string, { displayName: string; priority: number }> = {
  'main': { displayName: 'Main', priority: 1 },
  'other': { displayName: 'Other', priority: 100 },
}

export function getGroupDisplayName(group: string): string {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].displayName
  return group
}

export function getGroupPriority(group: string): number {
  if (GROUP_CONFIG[group]) return GROUP_CONFIG[group].priority
  return 99
}
