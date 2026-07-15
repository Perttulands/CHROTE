import { describe, expect, it } from 'vitest'
import type { SessionBankEntry, WorkloadRecoveryDescriptor } from './types'
import { getSessionBankRecoveryCapability, recoveryPlanDescriptors, summarizeSessionBankCapabilities } from './sessionBankRecovery'

const owner = { kind: 'session_bank', ref: 'alice/velis', mayRestart: true } as const

function descriptor(overrides: Partial<WorkloadRecoveryDescriptor> = {}): WorkloadRecoveryDescriptor {
  return {
    mode: 'agent',
    owner,
    topology: {
      sessionName: 'velis',
      sessionId: '$1',
      windowIndex: 0,
      windowName: 'agents',
      windowLayout: 'b25f,80x24,0,0',
      paneIndex: 0,
      paneId: '%1',
      paneCurrentPath: '/home/alice/velis',
    },
    workloadKind: 'codex',
    agent: {
      kind: 'codex',
      nativeSessionId: '019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
    },
    evidenceSource: 'argv',
    confidence: 'high',
    ...overrides,
  }
}

function banked(overrides: Partial<SessionBankEntry> = {}): SessionBankEntry {
  return {
    id: '$1',
    name: 'velis',
    unixUser: 'alice',
    windows: 1,
    attached: false,
    group: 'agents',
    live: false,
    firstSeen: '2026-07-15T10:00:00Z',
    lastSeen: '2026-07-15T10:05:00Z',
    ...overrides,
  }
}

function malformedDescriptor(overrides: Record<string, unknown>): WorkloadRecoveryDescriptor {
  return {
    ...descriptor(),
    ...overrides,
  } as unknown as WorkloadRecoveryDescriptor
}

function descriptorAt(windowIndex: number, paneIndex: number, overrides: Partial<WorkloadRecoveryDescriptor> = {}): WorkloadRecoveryDescriptor {
  return descriptor({
    topology: {
      ...descriptor().topology,
      windowIndex,
      windowName: `window-${windowIndex}`,
      windowLayout: `layout-${windowIndex}`,
      paneIndex,
      paneId: `%${windowIndex}-${paneIndex}`,
    },
    ...overrides,
  })
}

describe('session bank recovery capability', () => {
  it('classifies every backend descriptor capability without falling back to legacy shell labels', () => {
    const cases = [
      {
        name: 'workload plan with safe topology companion',
        entry: banked({
          recoveryPlan: [
            descriptor(),
            descriptor({
              mode: 'command',
              workloadKind: 'python-http-server',
              agent: undefined,
              command: {
                kind: 'python-http-server',
                pythonHTTPServer: { bind: '127.0.0.1', port: 8091, directory: '/home/alice/velis/public' },
              },
              topology: { ...descriptor().topology, paneIndex: 1, paneId: '%2' },
            }),
            descriptor({
              mode: 'topology',
              workloadKind: 'shell',
              agent: undefined,
              topology: { ...descriptor().topology, paneIndex: 2, paneId: '%3' },
              evidenceSource: 'topology',
              confidence: 'medium',
            }),
          ],
        }),
        kind: 'workload-recoverable',
        canRecoverWorkload: true,
        canRestoreTopologyOnly: true,
        badgeLabel: 'Workload recoverable',
      },
      {
        name: 'topology only',
        entry: banked({
          name: 'shape-only',
          recoveryPlan: [descriptor({
            mode: 'topology',
            owner: { kind: 'session_bank', ref: 'alice/shape-only', mayRestart: true },
            topology: { ...descriptor().topology, sessionName: 'shape-only' },
            workloadKind: 'shell',
            agent: undefined,
            evidenceSource: 'topology',
          })],
        }),
        kind: 'topology-only',
        canRecoverWorkload: false,
        canRestoreTopologyOnly: true,
        badgeLabel: 'Topology only',
      },
      {
        name: 'externally managed',
        entry: banked({
          name: 'systemd-worker',
          recoveryPlan: [
            descriptor({
              mode: 'managed',
              owner: { kind: 'external_manager', ref: 'systemd:user/velis.service', mayRestart: false },
              topology: { ...descriptor().topology, sessionName: 'systemd-worker' },
              workloadKind: 'managed',
              agent: undefined,
              evidenceSource: 'manager',
            }),
          ],
        }),
        kind: 'externally-managed',
        canRecoverWorkload: false,
        canRestoreTopologyOnly: false,
        badgeLabel: 'Managed read-only',
      },
      {
        name: 'mixed unresolved plan',
        entry: banked({
          name: 'mixed-agent',
          recoveryPlan: [
            descriptor({
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: true },
              topology: { ...descriptor().topology, sessionName: 'mixed-agent' },
            }),
            descriptor({
              mode: 'unresolved',
              owner: { kind: 'session_bank', ref: 'alice/mixed-agent', mayRestart: false },
              workloadKind: 'unknown',
              agent: undefined,
              evidenceSource: 'process',
              confidence: 'low',
              unresolvedReason: 'conflicting_evidence',
              topology: { ...descriptor().topology, paneIndex: 1, paneId: '%9' },
            }),
          ],
        }),
        kind: 'unresolved-unsafe',
        canRecoverWorkload: false,
        canRestoreTopologyOnly: false,
        badgeLabel: 'Unresolved / unsafe',
      },
      {
        name: 'legacy no plan',
        entry: banked({
          name: 'legacy-agent',
          recoveryKind: 'agent',
          agentKind: 'codex',
          agentSessionId: '019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
          resumeCommand: 'codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6',
        }),
        kind: 'legacy-no-plan',
        canRecoverWorkload: false,
        canRestoreTopologyOnly: false,
        badgeLabel: 'Legacy no plan',
      },
    ] as const

    cases.forEach(testCase => {
      const capability = getSessionBankRecoveryCapability(testCase.entry)
      expect(capability.kind).toBe(testCase.kind)
      expect(capability.canRecoverWorkload).toBe(testCase.canRecoverWorkload)
      expect(capability.canRestoreTopologyOnly).toBe(testCase.canRestoreTopologyOnly)
      expect(capability.badgeLabel).toBe(testCase.badgeLabel)
    })
  })

  it('reads backend recoveryPlan as an array', () => {
    expect(recoveryPlanDescriptors(banked({
      recoveryPlan: [descriptor()],
    }))).toHaveLength(1)
  })

  it.each([
    ['non-array object', { descriptors: [descriptor()] }, 'malformed_recovery_plan'],
    ['empty array', [], 'empty_recovery_plan'],
    ['array with non-object descriptor', [null], 'malformed_recovery_descriptor'],
  ])('fails closed when recoveryPlan is present but malformed: %s', (_, recoveryPlan, reason) => {
    const entry = banked({
      resumeCommand: 'tmux attach -t velis',
      recoveryPlan,
    } as unknown as Partial<SessionBankEntry>)

    const capability = getSessionBankRecoveryCapability(entry)

    expect(capability.kind).toBe('unresolved-unsafe')
    expect(capability.canRecoverWorkload).toBe(false)
    expect(capability.canRestoreTopologyOnly).toBe(false)
    expect(capability.unresolvedReasons).toContain(reason)
  })

  it('summarizes banked sessions without calling every entry recoverable', () => {
    const summary = summarizeSessionBankCapabilities([
      banked({
        name: 'safe',
        recoveryPlan: [descriptor({
          owner: { kind: 'session_bank', ref: 'alice/safe', mayRestart: true },
          topology: { ...descriptor().topology, sessionName: 'safe' },
        })],
      }),
      banked({
        name: 'shape',
        recoveryPlan: [descriptor({
          mode: 'topology',
          owner: { kind: 'session_bank', ref: 'alice/shape', mayRestart: true },
          topology: { ...descriptor().topology, sessionName: 'shape' },
          workloadKind: 'shell',
          agent: undefined,
        })],
      }),
      banked({
        name: 'external',
        recoveryPlan: [descriptor({
          mode: 'managed',
          owner: { kind: 'external_manager', ref: 'systemd:user/x.service', mayRestart: false },
          topology: { ...descriptor().topology, sessionName: 'external' },
          workloadKind: 'managed',
          agent: undefined,
        })],
      }),
      banked({ name: 'legacy', recoveryKind: 'shell' }),
    ])

    expect(summary).toMatchObject({
      total: 4,
      workloadRecoverable: 1,
      topologyOnly: 1,
      externallyManaged: 1,
      legacyNoPlan: 1,
    })
  })

  it.each([
    ['empty owner ref', malformedDescriptor({ owner: { kind: 'session_bank', ref: '', mayRestart: true } })],
    ['wrong owner ref', malformedDescriptor({ owner: { kind: 'session_bank', ref: 'bob/velis', mayRestart: true } })],
    ['non-restarting session bank owner', malformedDescriptor({ owner: { kind: 'session_bank', ref: 'alice/velis', mayRestart: false } })],
    ['topology targets another session', malformedDescriptor({ topology: { ...descriptor().topology, sessionName: 'other' } })],
    ['relative pane cwd', malformedDescriptor({ topology: { ...descriptor().topology, paneCurrentPath: 'relative/path' } })],
    ['negative window index', malformedDescriptor({ topology: { ...descriptor().topology, windowIndex: -1 } })],
    ['noninteger pane index', malformedDescriptor({ topology: { ...descriptor().topology, paneIndex: 1.25 } })],
    ['unsupported mode', malformedDescriptor({ mode: 'restart' })],
    ['unsupported evidence', malformedDescriptor({ evidenceSource: 'raw_argv' })],
    ['unsupported confidence', malformedDescriptor({ confidence: 'certain' })],
    ['unsupported agent kind', malformedDescriptor({ workloadKind: 'gpt', agent: { kind: 'gpt', nativeSessionId: 'abc' } })],
    ['missing agent id', malformedDescriptor({ agent: { kind: 'codex', nativeSessionId: '' } })],
    ['malformed hermes profile', malformedDescriptor({ workloadKind: 'hermes', agent: { kind: 'hermes', nativeSessionId: 'hermes-session', hermesProfile: '../scout' } })],
    ['unsupported command kind', malformedDescriptor({ mode: 'command', workloadKind: 'node', agent: undefined, command: { kind: 'node' } })],
    ['python server missing params', malformedDescriptor({ mode: 'command', workloadKind: 'python-http-server', agent: undefined, command: { kind: 'python-http-server' } })],
    ['python server unsafe bind', malformedDescriptor({ mode: 'command', workloadKind: 'python-http-server', agent: undefined, command: { kind: 'python-http-server', pythonHTTPServer: { bind: '0.0.0.0', port: 8091, directory: '/home/alice/site' } } })],
    ['python server invalid port', malformedDescriptor({ mode: 'command', workloadKind: 'python-http-server', agent: undefined, command: { kind: 'python-http-server', pythonHTTPServer: { bind: '127.0.0.1', port: 0, directory: '/home/alice/site' } } })],
    ['python server relative directory', malformedDescriptor({ mode: 'command', workloadKind: 'python-http-server', agent: undefined, command: { kind: 'python-http-server', pythonHTTPServer: { bind: '127.0.0.1', port: 8091, directory: 'site' } } })],
  ])('classifies malformed descriptor metadata as unresolved/unsafe: %s', (_, badDescriptor) => {
    const capability = getSessionBankRecoveryCapability(banked({ recoveryPlan: [badDescriptor] }))

    expect(capability.kind).toBe('unresolved-unsafe')
    expect(capability.canRecoverWorkload).toBe(false)
    expect(capability.canRestoreTopologyOnly).toBe(false)
  })

  it('rejects duplicate pane targets and inconsistent owners as unsafe', () => {
    const duplicateTarget = getSessionBankRecoveryCapability(banked({
      recoveryPlan: [
        descriptor(),
        descriptor({ topology: { ...descriptor().topology, paneId: '%2' } }),
      ],
    }))
    expect(duplicateTarget.kind).toBe('unresolved-unsafe')

    const conflictingOwner = getSessionBankRecoveryCapability(banked({
      recoveryPlan: [
        descriptor(),
        descriptor({
          owner: { kind: 'session_bank', ref: 'alice/other', mayRestart: true },
          topology: { ...descriptor().topology, paneIndex: 1, paneId: '%2' },
        }),
      ],
    }))
    expect(conflictingOwner.kind).toBe('unresolved-unsafe')
  })

  it.each([
    ['more than 128 descriptors', Array.from({ length: 129 }, (_, index) => descriptorAt(index % 32, Math.floor(index / 32)))],
    ['more than 32 windows', Array.from({ length: 33 }, (_, index) => descriptorAt(index, 0))],
    ['more than 32 panes in one window', Array.from({ length: 33 }, (_, index) => descriptorAt(0, index))],
    ['duplicate nonempty pane ids', [
      descriptorAt(0, 0, { topology: { ...descriptorAt(0, 0).topology, paneId: '%same' } }),
      descriptorAt(0, 1, { topology: { ...descriptorAt(0, 1).topology, paneId: '%same' } }),
    ]],
    ['non-contiguous window indexes from captured base', [
      descriptorAt(2, 0),
      descriptorAt(4, 0),
    ]],
    ['non-contiguous pane indexes from captured base', [
      descriptorAt(0, 2),
      descriptorAt(0, 4),
    ]],
    ['conflicting window names for the same window index', [
      descriptorAt(0, 0, { topology: { ...descriptorAt(0, 0).topology, windowName: 'agents' } }),
      descriptorAt(0, 1, { topology: { ...descriptorAt(0, 1).topology, windowName: 'other-agents' } }),
    ]],
    ['conflicting window layouts for the same window index', [
      descriptorAt(0, 0, { topology: { ...descriptorAt(0, 0).topology, windowLayout: 'layout-a' } }),
      descriptorAt(0, 1, { topology: { ...descriptorAt(0, 1).topology, windowLayout: 'layout-b' } }),
    ]],
  ])('classifies plan count/shape invariant failures as unresolved/unsafe: %s', (_, recoveryPlan) => {
    const capability = getSessionBankRecoveryCapability(banked({ recoveryPlan }))

    expect(capability.kind).toBe('unresolved-unsafe')
    expect(capability.canRecoverWorkload).toBe(false)
    expect(capability.canRestoreTopologyOnly).toBe(false)
  })
})
