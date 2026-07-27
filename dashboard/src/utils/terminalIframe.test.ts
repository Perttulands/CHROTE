import { beforeEach, describe, expect, it, vi } from 'vitest'
import { applyScrollbarVisibility, attachPasteBridge } from './terminalIframe'

interface TestTerminalWindow extends Window {
  term?: { paste?: (text: string) => void }
  __chrotePasteBridge?: boolean
}

function freshIframeWindow(): TestTerminalWindow {
  const iframe = document.createElement('iframe')
  document.body.appendChild(iframe)
  const win = iframe.contentWindow as TestTerminalWindow | null
  if (!win) throw new Error('jsdom iframe has no contentWindow')
  return win
}

function pasteChord(win: Window, init: KeyboardEventInit = {}) {
  const event = new win.window.KeyboardEvent('keydown', {
    key: 'v',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  })
  win.dispatchEvent(event)
  return event
}

function stubClipboard(win: TestTerminalWindow, text: string) {
  const readText = vi.fn(() => Promise.resolve(text))
  Object.defineProperty(win.navigator, 'clipboard', {
    configurable: true,
    value: { readText },
  })
  return readText
}

describe('attachPasteBridge', () => {
  let win: TestTerminalWindow

  beforeEach(() => {
    win = freshIframeWindow()
  })

  it('intercepts Ctrl+V, reads the clipboard, and pastes through xterm exactly once', async () => {
    const readText = stubClipboard(win, 'hello world')
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)

    const event = pasteChord(win)

    expect(event.defaultPrevented).toBe(true)
    await vi.waitFor(() => expect(paste).toHaveBeenCalledWith('hello world'))
    expect(readText).toHaveBeenCalledTimes(1)
    expect(paste).toHaveBeenCalledTimes(1)
  })

  it('supports Cmd+V for macOS browsers', async () => {
    stubClipboard(win, 'mac paste')
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)

    const event = pasteChord(win, { ctrlKey: false, metaKey: true })

    expect(event.defaultPrevented).toBe(true)
    await vi.waitFor(() => expect(paste).toHaveBeenCalledWith('mac paste'))
  })

  it('attaches only once per content window even when load fires repeatedly', async () => {
    stubClipboard(win, 'once')
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)
    attachPasteBridge(win)
    attachPasteBridge(win)

    pasteChord(win)

    await vi.waitFor(() => expect(paste).toHaveBeenCalledTimes(1))
  })

  it.each([
    ['Ctrl+Shift+V', { shiftKey: true }],
    ['Ctrl+Alt+V', { altKey: true }],
    ['plain v', { ctrlKey: false }],
    ['Ctrl+C', { key: 'c' }],
  ])('leaves %s untouched', (_label, init) => {
    stubClipboard(win, 'nope')
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)

    const event = pasteChord(win, init)

    expect(event.defaultPrevented).toBe(false)
    expect(paste).not.toHaveBeenCalled()
  })

  it('leaves the native path alone when the async clipboard is unavailable (plain-HTTP origins)', () => {
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)

    const event = pasteChord(win)

    expect(event.defaultPrevented).toBe(false)
    expect(paste).not.toHaveBeenCalled()
  })

  it('leaves the native path alone when xterm is not ready yet', () => {
    stubClipboard(win, 'early')
    attachPasteBridge(win)

    const event = pasteChord(win)

    expect(event.defaultPrevented).toBe(false)
  })

  it('swallows clipboard permission rejections without pasting', async () => {
    Object.defineProperty(win.navigator, 'clipboard', {
      configurable: true,
      value: { readText: vi.fn(() => Promise.reject(new Error('denied'))) },
    })
    const paste = vi.fn()
    win.term = { paste }
    attachPasteBridge(win)

    const event = pasteChord(win)

    expect(event.defaultPrevented).toBe(true)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(paste).not.toHaveBeenCalled()
  })
})

describe('applyScrollbarVisibility', () => {
  it('injects the hide style once, idempotently, and removes it on show', () => {
    const win = freshIframeWindow()
    const doc = win.document

    applyScrollbarVisibility(doc, true)
    applyScrollbarVisibility(doc, true)

    const styles = doc.querySelectorAll('#chrote-hide-scrollbar')
    expect(styles).toHaveLength(1)
    expect(styles[0].textContent).toContain('.xterm-viewport')
    expect(styles[0].textContent).toContain('scrollbar-width: none')

    applyScrollbarVisibility(doc, false)
    applyScrollbarVisibility(doc, false)
    expect(doc.querySelectorAll('#chrote-hide-scrollbar')).toHaveLength(0)
  })
})
