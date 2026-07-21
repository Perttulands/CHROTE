import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey, type SendSessionPane, type TmuxSession } from '../types'
import { detectAgentRole } from '../utils/roleDetection'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function filesFromList(list: FileList | File[] | null | undefined): File[] {
  if (!list) return []
  return Array.from(list).filter(file => file.size >= 0)
}

function defaultSubmitForSession(session: TmuxSession | undefined): boolean {
  if (!session) return false
  return Boolean(session.persistent || session.persistentAgentKind || detectAgentRole(session.name))
}

function paneLabel(pane: SendSessionPane): string {
  const details = [pane.windowName, pane.currentCommand, pane.currentPath].filter(Boolean)
  return `${pane.pane}${pane.active ? ' · active' : ''}${details.length > 0 ? ` · ${details.join(' · ')}` : ''}`
}

function SendToSessionModal() {
  const {
    sendToSessionTarget,
    sendToSessionPrefill,
    sendToSessionRequestId,
    sessions,
    closeSendToSession,
    listSessionPanes,
    sendToSession,
  } = useSession()
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [submit, setSubmit] = useState(false)
  const [sending, setSending] = useState(false)
  const [panes, setPanes] = useState<SendSessionPane[]>([])
  const [selectedPane, setSelectedPane] = useState('')
  const [panesLoading, setPanesLoading] = useState(false)
  const [panesFailed, setPanesFailed] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const activeTargetKeyRef = useRef<string | null>(null)
  const activeSendRef = useRef<symbol | null>(null)
  const defaultSubmitRef = useRef(false)
  const submitTouchedRef = useRef(false)

  const target = useMemo(() => {
    if (!sendToSessionTarget) return null
    const displayName = getSessionNameFromKey(sendToSessionTarget)
    const keyUser = getSessionUserFromKey(sendToSessionTarget)
    const matching = keyUser
      ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === sendToSessionTarget)
      : sessions.filter(item => item.name === displayName)
    const session = matching.length === 1 ? matching[0] : undefined
    const unresolvedBare = !keyUser && matching.length !== 1
    const unixUser = session?.unixUser ?? keyUser
    return { key: sendToSessionTarget, name: displayName, unixUser, session, unresolvedBare }
  }, [sendToSessionTarget, sessions])

  activeTargetKeyRef.current = target?.key ?? null
  defaultSubmitRef.current = defaultSubmitForSession(target?.session)

  useLayoutEffect(() => {
    activeSendRef.current = null
    submitTouchedRef.current = false
    setText(sendToSessionPrefill || '')
    setFiles([])
    setSubmit(defaultSubmitRef.current)
    setSending(false)
    setPanes([])
    setSelectedPane('')
    setPanesLoading(Boolean(target && !target.unresolvedBare))
    setPanesFailed(Boolean(target?.unresolvedBare))
    if (fileInputRef.current) fileInputRef.current.value = ''
  }, [sendToSessionPrefill, sendToSessionRequestId, target?.key])

  useLayoutEffect(() => {
    if (!submitTouchedRef.current) setSubmit(defaultSubmitRef.current)
  }, [target?.session])

  useEffect(() => {
    if (!target) return
    if (target.unresolvedBare) {
      setPanes([])
      setSelectedPane('')
      setPanesLoading(false)
      setPanesFailed(true)
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
  }, [listSessionPanes, sendToSessionRequestId, target?.key, target?.name, target?.unixUser, target?.unresolvedBare])

  const addFiles = useCallback((incoming: File[]) => {
    if (incoming.length === 0) return
    setFiles(prev => [...prev, ...incoming])
  }, [])

  const handlePaste = useCallback((event: React.ClipboardEvent) => {
    const pasted = filesFromList(event.clipboardData.files)
    if (pasted.length > 0) {
      event.preventDefault()
      addFiles(pasted)
    }
  }, [addFiles])

  const handleDrop = useCallback((event: React.DragEvent) => {
    event.preventDefault()
    addFiles(filesFromList(event.dataTransfer.files))
  }, [addFiles])

  const removeFile = useCallback((index: number) => {
    setFiles(prev => prev.filter((_, idx) => idx !== index))
  }, [])

  const selectedPaneTarget = useMemo(
    () => panes.find(pane => pane.pane === selectedPane),
    [panes, selectedPane],
  )

  const handleSend = useCallback(async () => {
    if (!target || target.unresolvedBare || sending || !selectedPaneTarget) return
    if (!text.trim() && files.length === 0) return
    const sendToken = Symbol(target.key)
    activeSendRef.current = sendToken
    setSending(true)
    try {
      const ok = await sendToSession(target.name, {
        text,
        files,
        submit,
        pane: selectedPaneTarget.pane,
        sessionId: selectedPaneTarget.sessionId,
        panePid: selectedPaneTarget.panePid,
        serverPid: selectedPaneTarget.serverPid,
      }, target.unixUser)
      if (ok && activeTargetKeyRef.current === target.key && activeSendRef.current === sendToken) {
        closeSendToSession()
      } else if (!ok && activeTargetKeyRef.current === target.key && activeSendRef.current === sendToken) {
        setPanes([])
        setSelectedPane('')
        setPanesFailed(false)
        setPanesLoading(true)
        const refreshed = await listSessionPanes(target.name, target.unixUser)
        if (activeTargetKeyRef.current === target.key && activeSendRef.current === sendToken) {
          setPanesLoading(false)
          if (!refreshed) {
            setPanesFailed(true)
          } else {
            setPanes(refreshed)
            setSelectedPane(refreshed.length === 1 ? refreshed[0].pane : '')
          }
        }
      }
    } finally {
      if (activeSendRef.current === sendToken) {
        activeSendRef.current = null
        setSending(false)
      }
    }
  }, [closeSendToSession, files, listSessionPanes, selectedPaneTarget, sendToSession, sending, submit, target, text])

  const handleKeyDown = useCallback((event: React.KeyboardEvent) => {
    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
      event.preventDefault()
      void handleSend()
    } else if (event.key === 'Escape') {
      closeSendToSession()
    }
  }, [closeSendToSession, handleSend])

  if (!target) return null

  const canSend = Boolean(text.trim() || files.length > 0) && Boolean(selectedPaneTarget) && !target.unresolvedBare && !panesLoading && !sending

  return (
    <div className="send-session-overlay" onClick={closeSendToSession}>
      <section
        className="send-session-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="send-session-title"
        onClick={event => event.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        <header className="send-session-header">
          <div>
            <h2 id="send-session-title">Send to Session: {target.name}</h2>
            {target.unixUser && <span className="send-session-target-user">Unix user: {target.unixUser}</span>}
          </div>
          <button className="modal-close" onClick={closeSendToSession} aria-label="Close Send to Session">×</button>
        </header>

        <p className="send-session-disclaimer">
          Sent text and files are retained for seven days by default and cleaned automatically in the background.
        </p>

        {panesLoading && <p className="send-session-pane-status" role="status">Resolving target panes…</p>}
        {panesFailed && (
          <p className="send-session-pane-status send-session-pane-error" role="alert">
            {target.unresolvedBare
              ? 'Session target is ambiguous or missing; open a user-qualified session target.'
              : 'Unable to resolve a safe target pane.'}
          </p>
        )}
        {!panesLoading && !panesFailed && panes.length === 1 && (
          <p className="send-session-pane-status">Target pane: <strong>{paneLabel(panes[0])}</strong></p>
        )}
        {!panesLoading && !panesFailed && panes.length > 1 && (
          <label className="send-session-label" htmlFor="send-session-pane">
            Target pane
            <select
              id="send-session-pane"
              className="send-session-pane-select"
              value={selectedPane}
              onChange={event => setSelectedPane(event.target.value)}
            >
              <option value="">Select an exact pane…</option>
              {panes.map(pane => <option key={pane.pane} value={pane.pane}>{paneLabel(pane)}</option>)}
            </select>
          </label>
        )}

        <label className="send-session-label" htmlFor="send-session-text">
          Message to send
        </label>
        <textarea
          id="send-session-text"
          className="send-session-textarea"
          value={text}
          onChange={event => setText(event.target.value)}
          onPaste={handlePaste}
          placeholder="Type or paste text here. Ctrl+Enter sends."
          autoFocus
        />

        <div
          className="send-session-dropzone"
          aria-label="Drop files or paste images"
          tabIndex={0}
          onPaste={handlePaste}
          onDragOver={event => event.preventDefault()}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
        >
          <strong>Drop files or paste images</strong>
          <span>Multiple files are OK. Paths will be inserted into the session.</span>
          <input
            ref={fileInputRef}
            className="send-session-file-input"
            type="file"
            multiple
            onChange={event => {
              addFiles(filesFromList(event.target.files))
              event.currentTarget.value = ''
            }}
          />
        </div>

        {files.length > 0 && (
          <ul className="send-session-file-list" aria-label="Files queued for Send to Session">
            {files.map((file, index) => (
              <li key={`${file.name}-${file.size}-${index}`}>
                <span>{file.name}</span>
                <em>{formatBytes(file.size)}</em>
                <button type="button" onClick={() => removeFile(index)} aria-label={`Remove ${file.name}`}>×</button>
              </li>
            ))}
          </ul>
        )}

        <footer className="send-session-actions">
          <label className="send-session-submit-toggle">
            <input
              type="checkbox"
              checked={submit}
              onChange={event => {
                submitTouchedRef.current = true
                setSubmit(event.target.checked)
              }}
            />
            Press Enter after sending
          </label>
          <div className="send-session-buttons">
            <button type="button" className="send-session-secondary" onClick={closeSendToSession}>Cancel</button>
            <button type="button" className="send-session-primary" onClick={handleSend} disabled={!canSend}>
              {sending ? 'Sending…' : 'Send'}
            </button>
          </div>
        </footer>
      </section>
    </div>
  )
}

export default SendToSessionModal
