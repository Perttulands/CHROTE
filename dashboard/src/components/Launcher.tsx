/* The launcher: what a new session is, where it starts, and who owns it.
   The operator picks a harness, a folder and a Unix user; the name follows
   from those three until he types over it. The same panel serves an empty
   window and the Sessions plus, because there is one way to start a session. */

import { useCallback, useEffect, useId, useMemo, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useTheme } from '../theme/ThemeContext'
import { identityColorFor } from '../theme/theme'
import { HarnessMark, harnessShortName, type HarnessId } from './harnessMarks'
import FolderField from './FolderField'
import FlagPanel from './FlagPanel'
import type { LaunchFlag } from './launchFlags'
import { getTerminalUserInitial, resolveLaunchUser } from '../types'
import type { CreateSessionAttachTarget, LaunchUser, TmuxSession, WorkspaceId } from '../types'
import './Launcher.css'

/**
 * One harness the server offers. The binary is the first word of the command
 * and the only part of it that leaves the host; the flags are what that
 * binary's own `--help` reported, so an older server that has never been asked
 * simply offers none.
 */
export interface LaunchHarnessOption {
  id: string
  label: string
  binary: string
  defaultFlags: string
  flags: LaunchFlag[]
}

export interface LaunchOptions {
  harnesses: LaunchHarnessOption[]
  folders: string[]
}

/** What a browser that never heard from /api/launch may still offer: a shell at home. */
export const FALLBACK_LAUNCH_OPTIONS: LaunchOptions = {
  harnesses: [{ id: 'shell', label: 'Shell', binary: '', defaultFlags: '', flags: [] }],
  folders: ['~'],
}

const HOME_TOKEN = '~'
const SHELL_HARNESS = 'shell'
const KNOWN_HARNESSES: readonly string[] = ['claude-code', 'codex', 'shell']
const RECENT_FOLDER_LIMIT = 5
/** Device-local: whether a launch installs the harness's completion hooks. On unless turned off. */
const NOTIFY_STORAGE_KEY = 'chrote-launcher-notify'

function readNotifyPreference(): boolean {
  try {
    return window.localStorage.getItem(NOTIFY_STORAGE_KEY) !== '0'
  } catch {
    return true
  }
}

function storeNotifyPreference(enabled: boolean): void {
  try {
    window.localStorage.setItem(NOTIFY_STORAGE_KEY, enabled ? '1' : '0')
  } catch {
    // A device that cannot remember still launches as asked.
  }
}

/** The word a derived session name starts with: the harness, said short. */
export function launchShortName(id: string): string {
  return KNOWN_HARNESSES.includes(id) ? harnessShortName(id as HarnessId) : id
}

/**
 * The folder said in one word, for a session name. tmux takes
 * `^[a-zA-Z0-9_-]+$`, so anything else in the last path segment becomes a
 * hyphen.
 */
export function folderBasename(folder: string): string {
  const trimmed = folder.trim().replace(/\/+$/, '')
  if (trimmed === HOME_TOKEN) return 'home'
  const last = trimmed.split('/').pop() ?? ''
  const cleaned = last.replace(/[^a-zA-Z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return cleaned || 'root'
}

function sessionsOfUser(sessions: readonly TmuxSession[], unixUser: LaunchUser): TmuxSession[] {
  const user = unixUser.trim()
  return sessions.filter(session => (session.unixUser ?? '').trim() === user)
}

/**
 * The name to offer, free of collision with a live session of the same user.
 * tmux numbers a duplicate from 2, the way a second window of the same thing
 * is the second one.
 */
export function derivedSessionName(
  harnessId: string,
  folder: string,
  sessions: readonly TmuxSession[],
  unixUser: LaunchUser,
): string {
  const base = `${launchShortName(harnessId)}-${folderBasename(folder)}`
  const taken = new Set(sessionsOfUser(sessions, unixUser).map(session => session.name))
  if (!taken.has(base)) return base
  for (let suffix = 2; suffix <= taken.size + 2; suffix++) {
    const candidate = `${base}-${suffix}`
    if (!taken.has(candidate)) return candidate
  }
  return base
}

/**
 * tmux hands out session ids in creation order, so the highest id is the
 * newest session. That is the only recency the inventory reports, and it is
 * enough to put the folder the operator was last in at the front.
 */
function sessionRecency(session: TmuxSession): number {
  const parsed = Number.parseInt((session.id ?? '').replace(/^\$/, ''), 10)
  return Number.isFinite(parsed) ? parsed : -1
}

/**
 * The folders this user's live sessions are sitting in, newest first, minus
 * the ones the configuration already pins as a chip.
 */
export function recentFolders(
  sessions: readonly TmuxSession[],
  unixUser: LaunchUser,
  pinnedFolders: readonly string[],
): string[] {
  const pinned = new Set(pinnedFolders.map(folder => folder.trim()))
  const ordered = [...sessionsOfUser(sessions, unixUser)].sort((a, b) => sessionRecency(b) - sessionRecency(a))
  const folders: string[] = []
  for (const session of ordered) {
    const cwd = session.cwd?.trim()
    if (!cwd || pinned.has(cwd) || folders.includes(cwd)) continue
    folders.push(cwd)
    if (folders.length === RECENT_FOLDER_LIMIT) break
  }
  return folders
}

/** A string field the server may not have written yet, read as absent rather than wrong. */
function optionalText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/**
 * The harness's catalogue. A server that predates the flags route, or a
 * harness whose `--help` was never read, has none; an entry missing the two
 * things a row needs is dropped rather than taking the whole payload down.
 */
function parseFlags(value: unknown): LaunchFlag[] {
  if (!Array.isArray(value)) return []
  const flags: LaunchFlag[] = []
  for (const entry of value) {
    if (typeof entry !== 'object' || entry === null) continue
    const flag = entry as { name?: unknown; short?: unknown; value?: unknown; description?: unknown; values?: unknown }
    if (typeof flag.name !== 'string' || flag.name === '') continue
    const values = Array.isArray(flag.values)
      ? flag.values.filter((candidate): candidate is string => typeof candidate === 'string')
      : undefined
    flags.push({
      name: flag.name,
      description: optionalText(flag.description),
      ...(typeof flag.short === 'string' && flag.short !== '' ? { short: flag.short } : {}),
      ...(typeof flag.value === 'string' && flag.value !== '' ? { value: flag.value } : {}),
      ...(values && values.length > 0 ? { values } : {}),
    })
  }
  return flags
}

function parseLaunchOptions(value: unknown): LaunchOptions | null {
  if (typeof value !== 'object' || value === null) return null
  const record = value as { harnesses?: unknown; folders?: unknown }
  if (!Array.isArray(record.harnesses) || !Array.isArray(record.folders)) return null
  const harnesses: LaunchHarnessOption[] = []
  for (const entry of record.harnesses) {
    if (typeof entry !== 'object' || entry === null) return null
    const harness = entry as { id?: unknown; label?: unknown; binary?: unknown; defaultFlags?: unknown; flags?: unknown }
    if (typeof harness.id !== 'string' || harness.id === '') return null
    if (typeof harness.label !== 'string' || harness.label === '') return null
    harnesses.push({
      id: harness.id,
      label: harness.label,
      binary: optionalText(harness.binary),
      defaultFlags: optionalText(harness.defaultFlags),
      flags: parseFlags(harness.flags),
    })
  }
  if (harnesses.length === 0) return null
  const folders = record.folders.filter((folder): folder is string => typeof folder === 'string' && folder.trim() !== '')
  return { harnesses, folders: folders.length > 0 ? folders : [HOME_TOKEN] }
}

/**
 * The launcher's choices, read once. They are a file on the host that changes
 * when the operator edits it, so a dashboard that missed an edit is one reload
 * away from having it; there is no poll and no retry.
 */
export function useLaunchOptions(): LaunchOptions {
  const [options, setOptions] = useState<LaunchOptions>(FALLBACK_LAUNCH_OPTIONS)

  useEffect(() => {
    let current = true
    const load = async () => {
      try {
        const response = await fetch('/api/launch', { signal: AbortSignal.timeout(10000) })
        if (!response.ok) {
          console.warn(`Launch options request failed (${response.status}); offering a shell`)
          return
        }
        const parsed = parseLaunchOptions(await response.json())
        if (!parsed) {
          console.warn('Launch options did not match the contract; offering a shell')
          return
        }
        if (current) setOptions(parsed)
      } catch (error) {
        console.warn('Launch options request failed; offering a shell', error)
      }
    }
    void load()
    return () => { current = false }
  }, [])

  return options
}

interface LauncherProps {
  workspaceId: WorkspaceId
  /** The window the new session binds to, when one is launching it. */
  attachTo?: CreateSessionAttachTarget
  /**
   * The folder to start in, when the surface that opened the launcher already
   * knows it. A desk offering to launch its own agent knows exactly one folder.
   */
  initialFolder?: string
  /**
   * The harness to start with, by its launch id, when the surface knows which
   * agent it is asking for. The operator can still choose another.
   */
  initialHarness?: string
  /**
   * Called once a session was created, with what it was created as. A popover
   * uses it to close itself; the Send drawer uses it to reach the session it
   * just launched.
   */
  onLaunched?: (created: { name: string; unixUser: LaunchUser }) => void
}

export default function Launcher({ workspaceId, attachTo, initialFolder, initialHarness, onLaunched }: LauncherProps) {
  const { sessions, settings, terminalUsers, createSession } = useSession()
  const theme = useTheme()
  const options = useLaunchOptions()
  const nameFieldId = useId()

  // Each choice is null until the operator makes it, so the defaults keep
  // following the configuration, the session list and the name derivation
  // instead of freezing whatever was true on first render.
  const [chosenHarness, setChosenHarness] = useState<string | null>(null)
  const [chosenFolder, setChosenFolder] = useState<string | null>(null)
  const [chosenUser, setChosenUser] = useState<LaunchUser | null>(null)
  const [typedName, setTypedName] = useState<string | null>(null)
  const [launching, setLaunching] = useState(false)
  // A flags line the operator edited, kept per harness so switching to Codex
  // and back does not throw away what he wrote for Claude Code. It lives as
  // long as the launcher does, and no longer: the defaults are the host's.
  const [flagEdits, setFlagEdits] = useState<Record<string, string>>({})
  const [flagsOpen, setFlagsOpen] = useState(false)
  const flagsFieldId = useId()
  const [notify, setNotify] = useState(readNotifyPreference)
  const folderFieldId = useId()

  const harness = options.harnesses.find(entry => entry.id === chosenHarness) ??
    options.harnesses.find(entry => entry.id === initialHarness) ??
    options.harnesses[0]
  const folder = chosenFolder ?? initialFolder ?? options.folders[0] ?? HOME_TOKEN
  const defaultUser = resolveLaunchUser(settings, workspaceId, terminalUsers)
  const user = chosenUser ?? defaultUser

  // What the Folder field offers before anything is typed: the pinned
  // folders, then where this user's sessions already are.
  const folderSuggestions = useMemo(
    () => [...options.folders, ...recentFolders(sessions, user, options.folders)],
    [sessions, user, options.folders],
  )
  const derivedName = useMemo(
    () => derivedSessionName(harness.id, folder, sessions, user),
    [harness.id, folder, sessions, user],
  )
  const name = typedName ?? derivedName

  const short = launchShortName(harness.id)
  const launchLabel = harness.id === SHELL_HARNESS
    ? `Open shell in ${folderBasename(folder)}`
    : `Launch ${short} in ${folderBasename(folder)}`

  // A shell takes no flags, and a harness whose binary the server did not name
  // cannot be previewed honestly, so neither offers a line to edit.
  const flagsOffered = harness.id !== SHELL_HARNESS && harness.binary !== ''
  const flagLine = flagEdits[harness.id] ?? harness.defaultFlags
  const flagsEdited = flagLine !== harness.defaultFlags
  const commandPreview = flagLine === '' ? harness.binary : `${harness.binary} ${flagLine}`

  const setFlagLine = useCallback((next: string) => {
    setFlagEdits(edits => ({ ...edits, [harness.id]: next }))
  }, [harness.id])

  const resetFlags = useCallback(() => {
    setFlagEdits(edits => {
      const next = { ...edits }
      delete next[harness.id]
      return next
    })
  }, [harness.id])

  // The Folder field's Enter launches with what it just chose, which the
  // state has not caught up with yet; everything else launches where it is.
  const launch = useCallback(async (inFolder: string = folder) => {
    const sessionName = name.trim()
    const cwd = inFolder.trim()
    if (!sessionName || !cwd || launching) return
    setLaunching(true)
    try {
      const created = await createSession({
        name: sessionName,
        unixUser: user,
        cwd,
        harness: harness.id,
        workspaceId,
        ...(flagsOffered ? { flags: flagLine, notify } : {}),
        ...(attachTo ? { attachTo } : {}),
      })
      if (created) onLaunched?.({ name: created, unixUser: user })
    } finally {
      setLaunching(false)
    }
  }, [attachTo, createSession, flagLine, flagsOffered, folder, harness.id, launching, name, notify, onLaunched, user, workspaceId])

  const setNotifyPreference = useCallback((enabled: boolean) => {
    setNotify(enabled)
    storeNotifyPreference(enabled)
  }, [])

  return (
    <div
      className={`launcher${flagsOpen && flagsOffered ? ' flags-open' : ''}`}
      data-ui="launcher"
      onClick={event => event.stopPropagation()}
    >
      {/* The frame is the container the flags panel measures: it docks beside
          the body when the launcher has the room and stacks under it when it
          does not. */}
      <div className="launcher-frame">
        <div className="launcher-body">
          <div className="launcher-title">Launch</div>
          {options.harnesses.map(entry => {
            const selected = entry.id === harness.id
            return (
              <button
                key={entry.id}
                type="button"
                className={`launcher-row${selected ? ' selected' : ''}`}
                data-ui="launcher.harness"
                aria-pressed={selected}
                onClick={() => setChosenHarness(entry.id)}
              >
                <span className="launcher-mark" aria-hidden="true">
                  {entry.id === SHELL_HARNESS
                    ? '>_'
                    : KNOWN_HARNESSES.includes(entry.id) && <HarnessMark id={entry.id as HarnessId} />}
                </span>
                <span className="launcher-row-label">{entry.label}</span>
              </button>
            )
          })}

          <label className="launcher-label" htmlFor={folderFieldId}>Folder</label>
          <div className="launcher-folder" data-ui="launcher.folder">
            <FolderField
              id={folderFieldId}
              value={folder}
              onChange={setChosenFolder}
              onSubmit={path => { void launch(path) }}
              recents={folderSuggestions}
              ariaLabel="Folder"
              inputClassName="launcher-name"
            />
          </div>

          {/* A server with no configured Unix users has no user to choose: the
              session runs as the one account CHROTE was given. */}
          {terminalUsers.length > 0 && <div className="launcher-label">User</div>}
          <div className="launcher-pick" data-ui="launcher.user">
            {terminalUsers.map(candidate => {
              const selected = candidate === user
              return (
                <button
                  key={candidate}
                  type="button"
                  className={`launcher-option${selected ? ' selected' : ''}`}
                  aria-pressed={selected}
                  onClick={() => setChosenUser(candidate)}
                >
                  <span
                    className="launcher-badge"
                    style={{ background: identityColorFor(candidate, terminalUsers, theme) }}
                    aria-hidden="true"
                  >
                    {getTerminalUserInitial(candidate)}
                  </span>
                  {candidate}
                </button>
              )
            })}
          </div>

          {/* The flags line is the whole of what will be typed after the
              binary: the catalogue writes into it, and so may the operator.
              A shell takes none and keeps the block all the same, greyed and
              inert, so the harness buttons above it stay where they are
              while the operator clicks through them; Reset keeps its slot
              for the same reason. */}
          <label className="launcher-label" htmlFor={flagsFieldId}>Flags</label>
          <input
            id={flagsFieldId}
            type="text"
            className="launcher-name launcher-flags"
            data-ui="launcher.flags"
            aria-label="Launch flags"
            value={flagsOffered ? flagLine : ''}
            disabled={!flagsOffered}
            onChange={event => setFlagLine(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') void launch()
            }}
          />
          <div className="launcher-preview" title={commandPreview}>{flagsOffered ? commandPreview : '\u00a0'}</div>
          <div className="launcher-pick">
            <button
              type="button"
              className="launcher-quiet"
              aria-expanded={flagsOpen && flagsOffered}
              disabled={!flagsOffered}
              onClick={() => setFlagsOpen(open => !open)}
            >
              Flags…
            </button>
            <button
              type="button"
              className="launcher-quiet launcher-reset"
              disabled={!flagsEdited}
              onClick={resetFlags}
            >
              Reset
            </button>
          </div>
          {/* The harness's own completion hooks, installed through its
              flags by the server: the session then reports when its
              agent finishes or waits. Off means the command runs as typed.
              A shell keeps the row, disabled, so nothing moves. */}
          <label className="launcher-notify">
            <input
              type="checkbox"
              checked={flagsOffered && notify}
              disabled={!flagsOffered}
              onChange={event => setNotifyPreference(event.target.checked)}
            />
            Notify on completion
          </label>

          <label className="launcher-label" htmlFor={nameFieldId}>Name</label>
          <input
            id={nameFieldId}
            type="text"
            className="launcher-name"
            data-ui="launcher.name"
            aria-label="Session name"
            value={name}
            onChange={event => setTypedName(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') void launch()
            }}
          />

          <div className="launcher-actions">
            <button
              type="button"
              className="launcher-quiet launcher-launch"
              onClick={() => { void launch() }}
              disabled={launching || name.trim() === '' || folder.trim() === ''}
            >
              {launchLabel}
            </button>
          </div>
        </div>

        {flagsOpen && flagsOffered && (
          <FlagPanel
            harnessLabel={harness.label}
            flags={harness.flags}
            line={flagLine}
            onChange={setFlagLine}
            onClose={() => setFlagsOpen(false)}
          />
        )}
      </div>
    </div>
  )
}
