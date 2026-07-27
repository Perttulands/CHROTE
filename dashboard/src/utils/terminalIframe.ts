// Same-origin helpers applied to pooled terminal iframes (chrt-6dcn).

interface TerminalWindow extends Window {
  term?: { paste?: (text: string) => void }
  __chrotePasteBridge?: boolean
}

// Ctrl+V/Cmd+V paste bridge. ttyd's xterm relies on native browser paste into
// its hidden textarea, which silently sends ^V instead when that textarea is
// not the focus target. Intercept the chord at capture phase, read the
// iframe-scoped clipboard, and hand the text to xterm's own paste() so
// bracketed paste mode is honored and exactly one paste happens.
//
// The interception only fires when BOTH the async clipboard and the xterm
// handle are actually available; otherwise the event is left untouched so the
// native path (however flaky) keeps working — notably on plain-HTTP origins
// such as Tailscale, where navigator.clipboard does not exist.
export function attachPasteBridge(iframeWindow: TerminalWindow): void {
  if (iframeWindow.__chrotePasteBridge) return
  iframeWindow.__chrotePasteBridge = true
  iframeWindow.addEventListener('keydown', (event: KeyboardEvent) => {
    if (!(event.ctrlKey || event.metaKey) || event.shiftKey || event.altKey) return
    if (event.key !== 'v' && event.key !== 'V') return
    const clipboard = iframeWindow.navigator?.clipboard
    const term = iframeWindow.term
    if (typeof clipboard?.readText !== 'function' || typeof term?.paste !== 'function') return
    event.preventDefault()
    event.stopPropagation()
    void clipboard.readText()
      .then(text => { if (text) term.paste?.(text) })
      .catch(() => { /* permission denied or empty clipboard: nothing to paste */ })
  }, true)
}

const SCROLLBAR_STYLE_ID = 'chrote-hide-scrollbar'

// xterm's .xterm-viewport keeps a permanent scrollbar gutter, but under
// ttyd-attached tmux the xterm scrollback is empty, so the bar is dead UI.
// Injecting/removing a style element flips visibility live, without reloading
// the iframe or touching the WebSocket.
export function applyScrollbarVisibility(doc: Document, hidden: boolean): void {
  const existing = doc.getElementById(SCROLLBAR_STYLE_ID)
  if (!hidden) {
    existing?.remove()
    return
  }
  if (existing) return
  const style = doc.createElement('style')
  style.id = SCROLLBAR_STYLE_ID
  style.textContent = [
    '.xterm-viewport { scrollbar-width: none !important; }',
    '.xterm-viewport::-webkit-scrollbar { width: 0; height: 0; display: none; }',
  ].join('\n')
  ;(doc.head ?? doc.documentElement)?.appendChild(style)
}
