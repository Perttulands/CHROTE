import { describe, expect, it, vi } from 'vitest'
import { copyAndAnnounce, copyTextToClipboard } from './clipboard'

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

describe('copyAndAnnounce', () => {
  it('says Copied only once the write has settled', async () => {
    let settle: () => void = () => {}
    const writeText = vi.fn(() => new Promise<void>(resolve => { settle = resolve }))
    setClipboard({ writeText })
    const announce = vi.fn()

    const pending = copyAndAnnounce('chrote-5grx.40', 'chrote-5grx.40', announce)
    expect(announce).not.toHaveBeenCalled()

    settle()
    await expect(pending).resolves.toBe(true)
    expect(announce).toHaveBeenCalledWith('Copied chrote-5grx.40', 'success')
  })

  it('reports a failure with its reason when nothing takes the text', async () => {
    setClipboard(undefined)
    const hadExecCommand = 'execCommand' in document
    const originalExecCommand = document.execCommand
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn(() => false) })
    const announce = vi.fn()

    try {
      await expect(copyAndAnnounce('/srv/chrote', 'path', announce)).resolves.toBe(false)
      expect(announce).toHaveBeenCalledWith(
        'Could not copy path: the clipboard API is unavailable here and the fallback was refused',
        'error',
      )
    } finally {
      if (hadExecCommand) {
        Object.defineProperty(document, 'execCommand', { configurable: true, value: originalExecCommand })
      } else {
        Reflect.deleteProperty(document, 'execCommand')
      }
    }
  })
})
