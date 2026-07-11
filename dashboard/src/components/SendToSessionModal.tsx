import { useCallback, useMemo, useRef, useState } from 'react'
import { useSession } from '../context/SessionContext'
import { getSessionKey, getSessionNameFromKey, getSessionUserFromKey } from '../types'

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function filesFromList(list: FileList | File[] | null | undefined): File[] {
  if (!list) return []
  return Array.from(list).filter(file => file.size >= 0)
}

function SendToSessionModal() {
  const { sendToSessionTarget, sessions, closeSendToSession, sendToSession } = useSession()
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [submit, setSubmit] = useState(true)
  const [sending, setSending] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const target = useMemo(() => {
    if (!sendToSessionTarget) return null
    const displayName = getSessionNameFromKey(sendToSessionTarget)
    const keyUser = getSessionUserFromKey(sendToSessionTarget)
    const matching = keyUser
      ? sessions.filter(item => getSessionKey(item.name, item.unixUser) === sendToSessionTarget)
      : sessions.filter(item => item.name === displayName)
    const session = matching.length === 1 ? matching[0] : undefined
    const unixUser = session?.unixUser ?? keyUser
    return { key: sendToSessionTarget, name: displayName, unixUser, session }
  }, [sendToSessionTarget, sessions])

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

  const handleSend = useCallback(async () => {
    if (!target || sending) return
    if (!text.trim() && files.length === 0) return
    setSending(true)
    try {
      const ok = await sendToSession(target.name, { text, files, submit }, target.unixUser)
      if (ok) closeSendToSession()
    } finally {
      setSending(false)
    }
  }, [closeSendToSession, files, sendToSession, sending, submit, target, text])

  const handleKeyDown = useCallback((event: React.KeyboardEvent) => {
    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
      event.preventDefault()
      void handleSend()
    } else if (event.key === 'Escape') {
      closeSendToSession()
    }
  }, [closeSendToSession, handleSend])

  if (!target) return null

  const canSend = Boolean(text.trim() || files.length > 0) && !sending

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
          Sent text and files are stored on disk until removed.
        </p>

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
            onChange={event => addFiles(filesFromList(event.target.files))}
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
              onChange={event => setSubmit(event.target.checked)}
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
