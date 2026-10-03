import { expect, it } from 'vitest'
import { nextCopiedSessionName } from './sessionCopy'

it('preserves numeric project names while numbering copies of a known family', () => {
  expect(nextCopiedSessionName('codex-project-2026', new Set(['codex-project-2026']))).toBe('codex-project-2026-2')
  expect(nextCopiedSessionName('codex-project-3', new Set(['codex-project', 'codex-project-2', 'codex-project-3']))).toBe('codex-project-4')
})
