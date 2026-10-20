/**
 * Throwaway for chrote-8iqw.3: three Send layouts on /?prototype=send&variant=A.
 * Run: npm run prototype:send --prefix dashboard (default port 5188).
 * All sessions, output and delivery are simulated in memory; there is no API.
 * Close retains an unsent draft for comparison convenience, not as a proposed
 * production contract. New-agent startup and attachments are outside this demo.
 * Reduced height is a layout stress test, not proof of physical keyboard behavior.
 */
import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import './SendDrawerPrototype.css'

const variants = ['A', 'B', 'C'] as const
type Variant = typeof variants[number]
const names = { A: 'Compose first', B: 'Output + message', C: 'Choose, then write' }
const sessions = [
  { name: 'codex-chrote', path: '/projects/chrote', harness: 'Codex', output: 'The tab fixes are ready.\nWaiting for your next instruction.' },
  { name: 'claude-game', path: '/projects/game', harness: 'Claude', output: 'The scene is loaded.\nWhat would you like to work on?' },
  { name: 'shell-tools', path: '/projects/tools', harness: 'Shell', output: '$ pwd\n/projects/tools\n$' },
  { name: 'codex-dashboard-layout-investigation', path: '/projects/dashboard', harness: 'Codex', output: 'Layout investigation complete.\nReady for review.' },
]
type Session = typeof sessions[number]
type Receipt = { target: string; text: string; submit: boolean }

function VariantA({ target, message, actions }: { target: ReactNode; message: ReactNode; actions: ReactNode }) {
  return <div className="sp-composer sp-variant-a">{target}{message}{actions}</div>
}
function VariantB({ target, message, actions, output }: { target: ReactNode; message: ReactNode; actions: ReactNode; output: ReactNode }) {
  return <div className="sp-composer sp-variant-b">{target}{output}{message}{actions}</div>
}
function VariantC({ target, message, actions, choosing, picker }: { target: ReactNode; message: ReactNode; actions: ReactNode; choosing: boolean; picker: ReactNode }) {
  return <div className="sp-composer sp-variant-c">{choosing ? picker : <>{target}{message}{actions}</>}</div>
}

export default function SendDrawerPrototype() {
  const [variant, setVariant] = useState<Variant>(() => {
    const value = new URLSearchParams(location.search).get('variant')
    return variants.find(item => item === value) ?? 'A'
  })
  const [device, setDevice] = useState('phone')
  const [short, setShort] = useState(false)
  const [open, setOpen] = useState(true)
  const [selected, setSelected] = useState<Session>(sessions[0])
  const [draft, setDraft] = useState('')
  const [choosing, setChoosing] = useState(variant === 'C')
  const [search, setSearch] = useState('')
  const [receipts, setReceipts] = useState<Receipt[]>([])
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [failNext, setFailNext] = useState(false)
  const [error, setError] = useState('')
  const [viewport, setViewport] = useState({ height: window.visualViewport?.height ?? innerHeight, top: 0 })
  const textarea = useRef<HTMLTextAreaElement>(null)

  // Real keyboards may shrink only the visual viewport; the demo follows it.
  useEffect(() => {
    const view = window.visualViewport
    const update = () => setViewport({ height: view?.height ?? innerHeight, top: view?.offsetTop ?? 0 })
    update()
    view?.addEventListener('resize', update)
    view?.addEventListener('scroll', update)
    return () => { view?.removeEventListener('resize', update); view?.removeEventListener('scroll', update) }
  }, [])

  const changeVariant = (next: Variant) => {
    setVariant(next); setChoosing(next === 'C'); setSearch('')
    const url = new URL(location.href); url.searchParams.set('prototype', 'send'); url.searchParams.set('variant', next)
    history.replaceState(null, '', url)
  }
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.target as HTMLElement).closest('input, textarea, select, [contenteditable]')) return
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault()
        changeVariant(variants[(variants.indexOf(variant) + (event.key === 'ArrowRight' ? 1 : 2)) % 3])
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [variant])

  const deliver = (submit: boolean) => {
    if (!draft.trim()) return
    if (failNext) { setError('Could not reach this session. Your message is still here.'); setFailNext(false); return }
    const next = { target: selected.name, text: draft, submit }
    setReceipts(previous => [...previous, next]); setReceipt(next)
    setDraft(''); setError(''); setOpen(false); setChoosing(false)
    textarea.current?.blur()
  }
  const choose = (session: Session) => { setSelected(session); setChoosing(false); setSearch(''); setError('') }
  const picker = <section className="sp-picker" aria-label="Choose target session">
    <div className="sp-picker-heading"><strong>Choose a session</strong><button onClick={() => setChoosing(false)}>Cancel</button></div>
    <input aria-label="Search sessions" type="search" value={search} onChange={event => setSearch(event.target.value)} placeholder="Find a session or folder" />
    <div className="sp-session-list">
      {sessions.filter(session => `${session.name} ${session.path}`.toLowerCase().includes(search.toLowerCase())).map(session => <button key={session.name} aria-pressed={selected.name === session.name} onClick={() => choose(session)}>
        <span>{session.name}</span><small>{session.harness} · {session.path}</small>
      </button>)}
      {!sessions.some(session => `${session.name} ${session.path}`.toLowerCase().includes(search.toLowerCase())) && <p>No matching sessions.</p>}
    </div>
    <p className="sp-picker-note">Your message stays when you change the target.</p>
  </section>
  const target = <div className="sp-target">
    <div><small>TO · {selected.harness}</small><strong title={selected.name}>{selected.name}</strong><span>{selected.path}</span></div>
    <button onClick={() => setChoosing(true)}>Change</button>
  </div>
  const message = <div className="sp-message">
    <label htmlFor="sp-message">Message</label>
    <textarea ref={textarea} id="sp-message" aria-label="Message to send" value={draft} onChange={event => { setDraft(event.target.value); setError('') }} placeholder="Write a prompt or paste a command…" autoCapitalize="off" autoCorrect="off" spellCheck={false} onKeyDown={event => {
      // Preview only: phone input keeps Return for newlines. Desktop retains its chords.
      if (device === 'desktop' && event.key === 'Enter' && !event.shiftKey && !event.altKey && !event.metaKey) { event.preventDefault(); deliver(!event.ctrlKey) }
    }} />
    <span className="sp-input-hint">{device === 'desktop' ? 'Enter sends · Shift+Enter new line' : 'Return adds a new line'}</span>
  </div>
  const actions = <footer className="sp-actions">
    {error && <p role="alert">{error}</p>}
    <div><span>Paste leaves it unsubmitted</span><button disabled={!draft.trim()} onClick={() => deliver(false)}>Paste</button><button className="sp-primary" disabled={!draft.trim()} onClick={() => deliver(true)}>Send</button></div>
  </footer>
  const output = <div className="sp-output" aria-label="Sample terminal output"><small>LAST OUTPUT · SAMPLE</small><pre>{selected.output}</pre></div>

  return <main className="sp-lab" style={{ '--sp-viewport': `${viewport.height}px`, '--sp-top': `${viewport.top}px` } as CSSProperties}>
    <header className="sp-lab-header"><div><strong>Send to session</strong><span>Interactive prototype · simulated delivery</span></div>
      <div className="sp-lab-controls">
        <label>Preview <select aria-label="Preview size" value={device} onChange={event => setDevice(event.target.value)}><option value="phone">Phone · 390</option><option value="small">Small phone · 320</option><option value="desktop">Desktop</option></select></label>
        <button aria-pressed={short} onClick={() => setShort(!short)}>Less height</button>
        <button aria-pressed={failNext} onClick={() => setFailNext(!failNext)}>Fail next send</button>
      </div>
    </header>
    <div className="sp-stage">
      <section className={`sp-device sp-device-${device} ${short ? 'sp-short' : ''}`} aria-label="Workspace preview">
        <nav className="sp-app-tabs"><span>☰</span><strong>TERMINAL</strong><span>1 / 3</span></nav>
        <div className="sp-workspace">
          <div className="sp-tile"><div className="sp-tile-header">{selected.name}</div><pre>{selected.output}</pre>
            {receipt && <div className="sp-receipt" role="status"><strong>{receipt.submit ? 'Sent' : 'Pasted, not submitted'} · simulated</strong><span>{receipt.target}</span><pre>{receipt.text}</pre></div>}
            <button className="sp-reopen" onClick={() => { setOpen(true); setChoosing(variant === 'C') }}>{draft ? 'Continue message' : 'Send to session'}</button>
          </div>
          {open && <aside className={`sp-drawer sp-drawer-${variant}`} role="dialog" aria-label="Send to session">
            <header className="sp-drawer-header"><span>Send to session</span><button onClick={() => setOpen(false)}>Close</button></header>
            {variant === 'A' && <VariantA target={target} message={message} actions={actions} />}
            {variant === 'B' && <VariantB target={target} message={message} actions={actions} output={output} />}
            {variant === 'C' && <VariantC target={target} message={message} actions={actions} choosing={choosing} picker={picker} />}
            {choosing && variant !== 'C' && <div className="sp-picker-overlay">{picker}</div>}
          </aside>}
        </div>
      </section>
      <aside className="sp-guide">
        <small>{variant === 'A' ? 'RECOMMENDED START' : 'COMPARISON'}</small><h1>{names[variant]}</h1>
        <p>{variant === 'A' ? 'The target stays at the top. The message takes the space. Paste and Send stay within reach.' : variant === 'B' ? 'Keep a glimpse of terminal output above the message. More context, less room to write.' : 'Choose a target first, then move into an uncluttered writing surface. Clear separation, one extra step.'}</p>
        <ol><li>Write or paste a few lines.</li><li>Try Less height while composing.</li><li>Change the target; your draft stays.</li><li>Try Paste, Send, or a failed send.</li></ol>
        <div className="sp-state"><span>Target</span><strong>{selected.name}</strong><span>Draft</span><strong>{draft.length} characters · {draft.split('\n').length} lines</strong><span>Delivery</span><strong>{receipts.length} simulated · {failNext ? 'next attempt fails' : 'ready'}</strong></div>
        <p className="sp-caveat">Less height is a space test, not a simulated phone keyboard. On a phone, tap the message to try the real keyboard.</p>
      </aside>
    </div>
    <nav className="sp-switcher" aria-label="Prototype variants"><button aria-label="Previous variant" onClick={() => changeVariant(variants[(variants.indexOf(variant) + 2) % 3])}>←</button><span><strong>{variant}</strong> {names[variant]}</span><button aria-label="Next variant" onClick={() => changeVariant(variants[(variants.indexOf(variant) + 1) % 3])}>→</button></nav>
  </main>
}
