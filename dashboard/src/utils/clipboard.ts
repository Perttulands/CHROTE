function isLocalhost(hostname: string | undefined): boolean {
  return hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1' || hostname === '[::1]'
}

function canUseAsyncClipboard(): boolean {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) return false
  if (typeof window === 'undefined') return true
  if (window.isSecureContext) return true
  return isLocalhost(window.location?.hostname)
}

function fallbackCopyText(text: string): boolean {
  if (typeof document === 'undefined' || !document.body || typeof document.execCommand !== 'function') return false

  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const selection = document.getSelection()
  const ranges: Range[] = []
  if (selection) {
    for (let index = 0; index < selection.rangeCount; index += 1) {
      ranges.push(selection.getRangeAt(index).cloneRange())
    }
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.setAttribute('aria-hidden', 'true')
  textarea.setAttribute('data-chrote-clipboard-fallback', 'true')
  textarea.style.position = 'fixed'
  textarea.style.top = '0'
  textarea.style.left = '0'
  textarea.style.width = '1px'
  textarea.style.height = '1px'
  textarea.style.padding = '0'
  textarea.style.border = '0'
  textarea.style.opacity = '0'
  textarea.style.pointerEvents = 'none'

  document.body.appendChild(textarea)
  textarea.focus({ preventScroll: true })
  textarea.select()
  textarea.setSelectionRange(0, textarea.value.length)

  let copied = false
  try {
    copied = document.execCommand('copy')
  } catch {
    // Keep the false default and restore the DOM/selection state below.
  } finally {
    textarea.remove()
    if (selection) {
      selection.removeAllRanges()
      ranges.forEach(range => selection.addRange(range))
    }
    activeElement?.focus({ preventScroll: true })
  }

  return copied
}

export async function copyTextToClipboard(text: string): Promise<boolean> {
  if (canUseAsyncClipboard()) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Fall through to the legacy browser-copy path. This keeps copy actions
      // useful on HTTP/Tailscale origins where the Clipboard API is absent or
      // denied but execCommand still works from a click/keyboard gesture.
    }
  }

  return fallbackCopyText(text)
}

export type CopyAnnouncer = (message: string, severity: 'success' | 'error') => void

// The boolean says only that the text did not land; this says what stood in
// the way, from what the page has to work with.
function clipboardFailureReason(): string {
  if (canUseAsyncClipboard()) return 'the browser refused'
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') return 'this browser has no clipboard API'
  return 'the clipboard API is unavailable here and the fallback was refused'
}

/**
 * Copy, wait for the write to settle, and say how it went: "Copied <what>",
 * or "Could not copy <what>: <reason>" as a failure. Every copy action in the
 * dashboard reports through here, so none of them claims success before it
 * has it.
 */
export async function copyAndAnnounce(text: string, what: string, announce: CopyAnnouncer): Promise<boolean> {
  const copied = await copyTextToClipboard(text)
  if (copied) announce(`Copied ${what}`, 'success')
  else announce(`Could not copy ${what}: ${clipboardFailureReason()}`, 'error')
  return copied
}
