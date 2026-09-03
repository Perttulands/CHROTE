/**
 * Send to Session: the one hand-off in CHROTE.
 *
 * Every surface that can name something an agent should act on — a tile, a
 * session row, Peek, the file viewer, the Bead on the table — opens this
 * drawer and hands it a reference. The drawer overlays the right edge of the
 * workspace at 380px, above the table's column, and nothing beneath it moves:
 * the card that opened it stays mounted where it was, and closing the drawer
 * reveals it unchanged with the focus back where it was taken from.
 *
 * The reference is shown, not editable: it names the thing the operator was
 * looking at, and an edited reference names nothing. The note beneath it is
 * his, and it is where the cursor goes.
 *
 * Enter sends — paste and submit. Shift+Enter pastes without submitting, for
 * an agent that is still thinking. A send that succeeded closes the drawer and
 * scrolls the target tile to the bottom, because the answer arrives there. A
 * send that failed keeps the drawer open with the server's own words above the
 * actions, because the note is still worth something and the operator is the
 * one who decides what to do with it.
 */

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { useTerminalPool } from './TerminalPool'
import { useTheme } from '../theme/ThemeContext'
import { identityColorFor } from '../theme/theme'
import { isSessionEnded } from '../terminal/tileState'
import { useSessionEvidence } from '../context/useSessionEvidence'
import {
  getSessionKey,
  getSessionNameFromKey,
  getSessionUserFromKey,
  getTerminalUserInitial,
  isTerminalWorkspaceId,
  type LaunchUser,
  type SendSessionPane,
  type TmuxSession,
  type WorkspaceId,
} from '../types'
import { SessionCommandMark, SessionLabel } from './sessionLabel'
import Launcher from './Launcher'
import './SendDrawer.css'

/** The picker's own row for a session that does not exist yet. */
const NEW_AGENT = 'new-agent'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function filesFromList(list: FileList | File[] | null | undefined): File[] {
  if (!list) return []
  return Array.from(list).filter(file => file.size >= 0)
}

function paneLabel(pane: SendSessionPane): string {
  const details = [pane.windowName, pane.currentCommand, pane.currentPath].filter(Boolean)
  return `${pane.pane}${pane.active ? ' · active' : ''}${details.length > 0 ? ` · ${details.join(' · ')}` : ''}`
}

/** The message as the session receives it: the reference, then the note. */
export function composeMessage(reference: string | undefined, note: string): string {
  const line = reference?.trim() ?? ''
  if (!line) return note
  return note ? `${line}\n\n${note}` : line
}

interface DrawerTarget {
  key: string
  name: string
  unixUser: LaunchUser
  session: TmuxSession | undefined
  /** A bare name that matches no session, or more than one. */
  unresolvedBare: boolean
  ended: boolean
}

function resolveTarget(
  sessionKey: string,
  sessions: readonly TmuxSession[],
  evidence: ReturnType<typeof useSessionEvidence>,
): DrawerTarget {
  const name = getSessionNameFromKey(sessionKey)
  const keyUser = getSessionUserFromKey(sessionKey)
  const matching = keyUser
    ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === sessionKey)
    : sessions.filter(item => item.name === name)
  const session = matching.length === 1 ? matching[0] : undefined
  return {
    key: sessionKey,
    name,
    unixUser: session?.unixUser ?? keyUser,
    session,
    unresolvedBare: !keyUser && matching.length !== 1,
    ended: isSessionEnded(sessionKey, evidence),
  }
}

export default function SendDrawer() {
  const {
    sendToSessionRequest,
    sendToSessionRequestId,
    sessions,
    workspaces,
    workspaceIds,
    focusedWindowKey,
    terminalUsers,
    closeSendToSession,
    listSessionPanes,
    sendToSession,
  } = useSession()
  const pool = useTerminalPool()
  const theme = useTheme()
  const evidence = useSessionEvidence()

  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [note, setNote] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [sending, setSending] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const [panes, setPanes] = useState<SendSessionPane[]>([])
  const [selectedPane, setSelectedPane] = useState('')
  const [panesLoading, setPanesLoading] = useState(false)
  const [panesFailed, setPanesFailed] = useState(false)
  const [deliveryUnknown, setDeliveryUnknown] = useState(false)
  const noteRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const activeSendRef = useRef<symbol | null>(null)

  const open = sendToSessionRequest !== null
  const reference = sendToSessionRequest?.reference
  const launch = sendToSessionRequest?.launch

  /** The session in the focused tile: what "Send" means with nothing in hand. */
  const focusedSessionKey = useMemo(() => {
    if (!focusedWindowKey) return null
    for (const [workspaceId, workspace] of Object.entries(workspaces)) {
      const found = workspace.windows.find(window => `${workspaceId}-${window.id}` === focusedWindowKey)
      if (found?.activeSession) return found.activeSession
    }
    return null
  }, [focusedWindowKey, workspaces])

  const focusedWorkspaceId: WorkspaceId = useMemo(() => {
    const prefix = focusedWindowKey?.split('-')[0]
    if (prefix && isTerminalWorkspaceId(prefix)) return prefix
    return workspaceIds[0] ?? 'terminal1'
  }, [focusedWindowKey, workspaceIds])

  // A fresh opening is a fresh message: the previous note belonged to whatever
  // the operator was looking at then, and carrying it over would send it to
  // something else.
  useLayoutEffect(() => {
    if (!open) return
    activeSendRef.current = null
    setSelected(sendToSessionRequest.targetSessionKey ?? focusedSessionKey ?? null)
    setSearch('')
    setNote(sendToSessionRequest.note ?? '')
    setFiles([])
    setSending(false)
    setFailure(null)
    setDeliveryUnknown(false)
    if (fileInputRef.current) fileInputRef.current.value = ''
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sendToSessionRequestId, open])

  // The control that opened the drawer gets the focus back when it closes,
  // whichever way it closed: the card's Send button, the tile, the row. With
  // that control gone, the table's column is the next best place to land.
  useEffect(() => {
    if (!open) return
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    return () => {
      const landing = opener?.isConnected && opener !== document.body
        ? opener
        : document.querySelector<HTMLElement>('.table-column')
      landing?.focus()
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    noteRef.current?.focus()
  }, [open, sendToSessionRequestId])

  useEffect(() => {
    if (!open) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      closeSendToSession()
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [open, closeSendToSession])

  const target = useMemo(
    () => (selected && selected !== NEW_AGENT ? resolveTarget(selected, sessions, evidence) : null),
    [selected, sessions, evidence],
  )

  // The exact pane is resolved before the operator can press Send, so a target
  // that cannot receive says so here rather than failing at delivery.
  useEffect(() => {
    if (!open || !target) {
      setPanes([])
      setSelectedPane('')
      setPanesLoading(false)
      setPanesFailed(false)
      return
    }
    if (target.ended || target.unresolvedBare) {
      setPanes([])
      setSelectedPane('')
      setPanesLoading(false)
      setPanesFailed(target.unresolvedBare)
      return
    }
    setPanesLoading(true)
    setPanesFailed(false)
    let current = true
    void listSessionPanes(target.name, target.unixUser).then(result => {
      if (!current) return
      setPanesLoading(false)
      if (!result) {
        setPanesFailed(true)
        return
      }
      setPanes(result)
      setSelectedPane(result.length === 1 ? result[0].pane : '')
    })
    return () => { current = false }
  }, [open, listSessionPanes, sendToSessionRequestId, target?.key, target?.name, target?.unixUser, target?.ended, target?.unresolvedBare]) // eslint-disable-line react-hooks/exhaustive-deps

  const candidates = useMemo(() => {
    const needle = search.trim().toLowerCase()
    const matches = sessions.filter(session => {
      if (!needle) return true
      return session.name.toLowerCase().includes(needle) ||
        (session.unixUser ?? '').toLowerCase().includes(needle)
    })
    return matches.map(session => ({ session, key: getSessionKey(session.name, session.unixUser) }))
  }, [sessions, search])

  const addFiles = useCallback((incoming: File[]) => {
    if (incoming.length === 0) return
    setFiles(previous => [...previous, ...incoming])
  }, [])

  const handlePaste = useCallback((event: React.ClipboardEvent) => {
    const pasted = filesFromList(event.clipboardData.files)
    if (pasted.length === 0) return
    event.preventDefault()
    addFiles(pasted)
  }, [addFiles])

  const selectedPaneTarget = useMemo(
    () => panes.find(pane => pane.pane === selectedPane),
    [panes, selectedPane],
  )

  const message = composeMessage(reference, note)
  const canSend = Boolean(target) && Boolean(message.trim() || files.length > 0) &&
    Boolean(selectedPaneTarget) && !panesLoading && !sending && !deliveryUnknown

  const deliver = useCallback(async (submit: boolean, session: DrawerTarget, pane: SendSessionPane, text: string) => {
    const token = Symbol(session.key)
    activeSendRef.current = token
    setSending(true)
    setFailure(null)
    try {
      const report = await sendToSession(session.name, {
        text,
        files,
        submit,
        pane: pane.pane,
        sessionId: pane.sessionId,
        panePid: pane.panePid,
        serverPid: pane.serverPid,
      }, session.unixUser)
      if (activeSendRef.current !== token) return
      if (report.outcome === 'sent') {
        // The answer arrives at the bottom of the tile, so that is where the
        // operator is put before the drawer gets out of his way.
        pool.terminals.get(session.key)?.scrollToBottom()
        closeSendToSession()
        return
      }
      setFailure(report.message)
      if (report.outcome === 'unknown') setDeliveryUnknown(true)
    } finally {
      if (activeSendRef.current === token) {
        activeSendRef.current = null
        setSending(false)
      }
    }
  }, [closeSendToSession, files, pool.terminals, sendToSession])

  const send = useCallback((submit: boolean) => {
    if (!canSend || !target || !selectedPaneTarget) return
    void deliver(submit, target, selectedPaneTarget, message)
  }, [canSend, deliver, message, selectedPaneTarget, target])

  // A session that did not exist a moment ago is not ready for a submitted
  // prompt: the harness may still be starting. The message is pasted and left
  // on the agent's line, so the operator presses Enter when it is awake.
  const handleLaunched = useCallback(async (created: { name: string; unixUser: LaunchUser }) => {
    const key = getSessionKey(created.name, created.unixUser)
    const resolved = await listSessionPanes(created.name, created.unixUser)
    if (!resolved || resolved.length === 0) {
      setFailure(`Launched '${created.name}', but no pane answered; send to it from its tile.`)
      return
    }
    const pane = resolved.find(item => item.active) ?? resolved[0]
    await deliver(false, {
      key,
      name: created.name,
      unixUser: created.unixUser,
      session: undefined,
      unresolvedBare: false,
      ended: false,
    }, pane, message)
  }, [deliver, listSessionPanes, message])

  const handleNoteKeyDown = useCallback((event: React.KeyboardEvent) => {
    if (event.key !== 'Enter') return
    event.preventDefault()
    send(!event.shiftKey)
  }, [send])

  if (!open) return null

  const targetRow = (key: string, session: TmuxSession) => {
    const badgeColor = session.unixUser ? identityColorFor(session.unixUser, terminalUsers, theme) : undefined
    return (
      <button
        key={key}
        type="button"
        role="option"
        aria-selected={selected === key}
        // The name is spelt once, whole: the label is split across spans for
        // truncation, and a reader that walks those spans says it in pieces.
        aria-label={session.unixUser ? `${session.name} · ${session.unixUser}` : session.name}
        className={`send-drawer-target${selected === key ? ' selected' : ''}`}
        onClick={() => setSelected(key)}
      >
        {session.unixUser && (
          <span
            className="unix-user-badge"
            style={badgeColor ? { backgroundColor: badgeColor, borderColor: badgeColor } : undefined}
            aria-label={`Unix user ${session.unixUser}`}
          >
            {getTerminalUserInitial(session.unixUser)}
          </span>
        )}
        <SessionCommandMark command={session.currentCommand} />
        <SessionLabel name={session.name} className="send-drawer-target-name" />
        {key === focusedSessionKey && <span className="send-drawer-target-note">focused tile</span>}
      </button>
    )
  }

  return (
    <aside className="send-drawer" role="dialog" aria-label="Send to session">
      <div className="send-drawer-header">
        <span className="send-drawer-title">Send to session</span>
        <button
          type="button"
          className="send-drawer-close"
          onClick={closeSendToSession}
          aria-label="Close Send to Session"
          aria-keyshortcuts="Escape"
        >
          Close<span className="send-drawer-chord" aria-hidden="true">Esc</span>
        </button>
      </div>
        <div className="send-drawer-body" data-ui="send.drawer">
          <div className="send-drawer-section">Target</div>
          <input
            type="search"
            className="send-drawer-search"
            placeholder="Search sessions"
            aria-label="Search sessions"
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
          <div className="send-drawer-targets" role="listbox" aria-label="Target session">
            {/* The live sessions scroll; the row for one that does not exist yet
                does not, so the new agent is reachable however long the list is. */}
            <div className="send-drawer-target-list" role="presentation">
              {candidates.map(({ key, session }) => targetRow(key, session))}
            </div>
            <button
              type="button"
              role="option"
              aria-selected={selected === NEW_AGENT}
              className={`send-drawer-target${selected === NEW_AGENT ? ' selected' : ''}`}
              onClick={() => setSelected(NEW_AGENT)}
            >
              <span className="send-drawer-target-name">{launch?.label ?? 'New agent…'}</span>
            </button>
          </div>

          {selected === NEW_AGENT && (
            <div className="send-drawer-launcher">
              <Launcher
                workspaceId={focusedWorkspaceId}
                initialFolder={launch?.folder}
                initialHarness={launch?.harness}
                onLaunched={handleLaunched}
              />
            </div>
          )}

          {target?.ended && (
            <p className="send-drawer-status error" role="alert">
              {target.name} ended. There is no session left to send to; restart it from its tile first.
            </p>
          )}
          {panesFailed && (
            <p className="send-drawer-status error" role="alert">
              {target?.unresolvedBare
                ? 'Session target is ambiguous or missing; pick a user-qualified session.'
                : 'Unable to resolve a safe target pane.'}
            </p>
          )}
          {panesLoading && <p className="send-drawer-status" role="status">Resolving target panes…</p>}
          {!panesLoading && !panesFailed && panes.length > 1 && (
            <label className="send-drawer-section" htmlFor="send-drawer-pane">
              Pane
              <select
                id="send-drawer-pane"
                className="send-drawer-pane"
                value={selectedPane}
                onChange={event => setSelectedPane(event.target.value)}
              >
                <option value="">Pick the exact pane…</option>
                {panes.map(pane => <option key={pane.pane} value={pane.pane}>{paneLabel(pane)}</option>)}
              </select>
            </label>
          )}

          <div className="send-drawer-section">Message</div>
          <div
            className="send-drawer-message"
            onDragOver={event => event.preventDefault()}
            onDrop={event => {
              event.preventDefault()
              addFiles(filesFromList(event.dataTransfer.files))
            }}
          >
            {reference && <div className="send-drawer-reference">{reference}</div>}
            <textarea
              ref={noteRef}
              id="send-drawer-note"
              className="send-drawer-note"
              aria-label="Message to send"
              value={note}
              onChange={event => setNote(event.target.value)}
              onPaste={handlePaste}
              onKeyDown={handleNoteKeyDown}
            />
          </div>

          {files.length > 0 && (
            <ul className="send-drawer-files" aria-label="Files queued for Send to Session">
              {files.map((file, index) => (
                <li key={`${file.name}-${file.size}-${index}`}>
                  <span>{file.name}</span>
                  <em>{formatBytes(file.size)}</em>
                  <button
                    type="button"
                    onClick={() => setFiles(previous => previous.filter((_, at) => at !== index))}
                    aria-label={`Remove ${file.name}`}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          )}
          <button
            type="button"
            className="send-drawer-attach"
            onClick={() => fileInputRef.current?.click()}
          >
            drop files to attach
          </button>
          <input
            ref={fileInputRef}
            className="send-drawer-file-input"
            type="file"
            multiple
            onChange={event => {
              addFiles(filesFromList(event.target.files))
              event.currentTarget.value = ''
            }}
          />

          {failure !== null && (
            <p className="send-drawer-status error" role="alert">{failure}</p>
          )}

          <div className="send-drawer-actions">
            <button
              type="button"
              className="send-drawer-secondary"
              onClick={() => send(false)}
              disabled={!canSend}
            >
              Paste
            </button>
            <button
              type="button"
              className="send-drawer-primary"
              onClick={() => send(true)}
              disabled={!canSend}
            >
              {sending ? 'Sending…' : 'Send'}
            </button>
          </div>
          <p className="send-drawer-hint">Enter sends · Shift+Enter pastes</p>
        </div>
    </aside>
  )
}
