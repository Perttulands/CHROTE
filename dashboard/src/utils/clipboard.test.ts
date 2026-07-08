import { describe, expect, it, vi } from 'vitest'
import { copyTextToClipboard } from './clipboard'

function setClipboard(value: { writeText: ReturnType<typeof vi.fn> } | undefined) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value,
  })
}

function captureExecCommandCopy() {
  const hadExecCommand = 'execCommand' in document
  const originalExecCommand = document.execCommand
  let copiedText = ''
  const execCommand = vi.fn((command: string) => {
    if (command !== 'copy') return false
    copiedText = (document.querySelector('[data-chrote-clipboard-fallback="true"]') as HTMLTextAreaElement | null)?.value ?? ''
    return true
  })
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: execCommand,
  })
  return {
    execCommand,
    copiedText: () => copiedText,
    restore: () => {
      if (hadExecCommand) {
        Object.defineProperty(document, 'execCommand', { configurable: true, value: originalExecCommand })
      } else {
        Reflect.deleteProperty(document, 'execCommand')
      }
    },
  }
}

describe('copyTextToClipboard', () => {
  it('uses the async Clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard({ writeText })

    await expect(copyTextToClipboard('/srv/chrote')).resolves.toBe(true)

    expect(writeText).toHaveBeenCalledWith('/srv/chrote')
  })

  it('falls back to execCommand when the Clipboard API is unavailable', async () => {
    setClipboard(undefined)
    const execCopy = captureExecCommandCopy()

    try {
      await expect(copyTextToClipboard('/srv/chrote/dashboard')).resolves.toBe(true)

      expect(execCopy.execCommand).toHaveBeenCalledWith('copy')
      expect(execCopy.copiedText()).toBe('/srv/chrote/dashboard')
    } finally {
      execCopy.restore()
    }
  })

  it('falls back to execCommand when async clipboard copy is denied', async () => {
    const writeText = vi.fn().mockRejectedValue(new DOMException('blocked', 'NotAllowedError'))
    setClipboard({ writeText })
    const execCopy = captureExecCommandCopy()

    try {
      await expect(copyTextToClipboard('ctx-cnl')).resolves.toBe(true)

      expect(writeText).toHaveBeenCalledWith('ctx-cnl')
      expect(execCopy.execCommand).toHaveBeenCalledWith('copy')
      expect(execCopy.copiedText()).toBe('ctx-cnl')
    } finally {
      execCopy.restore()
    }
  })
})
