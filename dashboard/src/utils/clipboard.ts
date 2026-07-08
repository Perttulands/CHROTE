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
