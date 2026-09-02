import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { CLAUDE_CODE_MARK_COLOR, HarnessMark, harnessIdForCommand, harnessShortName } from './harnessMarks'

describe('harnessIdForCommand', () => {
  it('recognises the two agents and every shell, and nothing else', () => {
    expect(harnessIdForCommand('claude')).toBe('claude-code')
    expect(harnessIdForCommand('codex')).toBe('codex')
    for (const shell of ['sh', 'bash', 'dash', 'fish', 'ksh', 'zsh']) {
      expect(harnessIdForCommand(shell)).toBe('shell')
    }
    expect(harnessIdForCommand('sleep')).toBeNull()
    expect(harnessIdForCommand('node')).toBeNull()
    expect(harnessIdForCommand('')).toBeNull()
    expect(harnessIdForCommand(undefined)).toBeNull()
    expect(harnessIdForCommand(' claude ')).toBe('claude-code')
  })

  it('derives the word a session name starts with', () => {
    expect(harnessShortName('claude-code')).toBe('claude')
    expect(harnessShortName('codex')).toBe('codex')
    expect(harnessShortName('shell')).toBe('shell')
  })
})

describe('HarnessMark', () => {
  it('draws Claude Code in its native colour and Codex in currentColor', () => {
    const claude = render(<HarnessMark id="claude-code" />).container.querySelector('svg')
    expect(claude?.getAttribute('fill')).toBe(CLAUDE_CODE_MARK_COLOR)
    expect(claude?.getAttribute('data-harness')).toBe('claude-code')
    const codex = render(<HarnessMark id="codex" size={20} />).container.querySelector('svg')
    expect(codex?.getAttribute('fill')).toBe('currentColor')
    expect(codex?.getAttribute('width')).toBe('20')
  })

  it('draws nothing for a shell', () => {
    expect(render(<HarnessMark id="shell" />).container.innerHTML).toBe('')
  })
})
