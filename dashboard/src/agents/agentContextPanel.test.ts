import { describe, expect, it } from 'vitest'
import { harnessOfCommand } from './agentContextPanel'

// The session's menu asks about the agent in the pane, so the harness is read
// from the command tmux reports. A pane that is not an agent still has a
// folder, and the panel shows that folder's stack rather than nothing.
describe('harnessOfCommand', () => {
  it.each([
    { command: 'claude', harness: 'claude-code', shell: false },
    { command: 'node .../claude-code/cli.js', harness: 'claude-code', shell: false },
    { command: 'codex', harness: 'codex', shell: false },
    { command: 'codex-tui', harness: 'codex', shell: false },
    { command: 'bash', harness: 'claude-code', shell: true },
    { command: undefined, harness: 'claude-code', shell: true },
  ])('reads $command as $harness', ({ command, harness, shell }) => {
    expect(harnessOfCommand(command)).toEqual({ harness, shell })
  })
})
