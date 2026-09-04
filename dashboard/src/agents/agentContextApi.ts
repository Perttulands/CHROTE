/**
 * What an agent sees, as the server resolves it.
 *
 * One request answers the whole question for a folder and a harness: the
 * instruction stack in loading order, the skills that are reachable, and the
 * memories written for that folder. Nothing is cached here — the panel and the
 * tab each ask once when they open, which is what makes the answer true.
 */

import { apiErrorMessage } from '../apiErrors'

export type AgentHarness = 'claude-code' | 'codex'

/** The harnesses, in the order the tab lists them, with the words it shows. */
export const AGENT_HARNESSES: { id: AgentHarness; label: string }[] = [
  { id: 'claude-code', label: 'Claude Code' },
  { id: 'codex', label: 'Codex' },
]

export interface AgentInstruction {
  path: string
  /** managed, user, ancestor or project: which rung of the stack this is; conditional for a rule whose paths frontmatter limits it. */
  scope: string
  /** CLAUDE.md, AGENTS.md or settings. */
  kind: string
  /** False when the file is there but the server's account cannot open it. */
  readable: boolean
  size: number
  /** The base name a symlink points at, when this file is one. */
  link?: string
}

export interface AgentSkill {
  name: string
  description: string
  path: string
  /** project, user or shared. */
  source: string
}

export interface AgentMemory {
  /** claude-auto, codex or bd. */
  kind: string
  /** Empty for a memory that is not a file, such as a bd memory. */
  path: string
  title: string
  updated: string
  readable: boolean
}

export interface AgentContext {
  folder: string
  harness: AgentHarness
  user: string
  instructions: AgentInstruction[]
  skills: AgentSkill[]
  memories: AgentMemory[]
}

const REQUEST_TIMEOUT_MS = 20000

async function getJson<T>(url: string, failure: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal: signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS) })
  if (!response.ok) throw new Error(apiErrorMessage(await response.text(), failure))
  return await response.json() as T
}

function contextQuery(folder: string, harness: AgentHarness, user: string): string {
  const params = new URLSearchParams({ folder, harness })
  if (user) params.set('user', user)
  return params.toString()
}

export function fetchAgentContext(
  folder: string,
  harness: AgentHarness,
  user: string,
  signal?: AbortSignal,
): Promise<AgentContext> {
  return getJson<AgentContext>(
    `/api/agent/context?${contextQuery(folder, harness, user)}`,
    'Could not resolve what this agent sees',
    signal,
  )
}

/**
 * One file the stack listed. The server checks the path against the same
 * resolution the panel drew, so this reads what is on screen and nothing else.
 */
export async function fetchAgentFile(
  path: string,
  folder: string,
  harness: AgentHarness,
  user: string,
  signal?: AbortSignal,
): Promise<string> {
  const params = new URLSearchParams({ path, folder, harness })
  if (user) params.set('user', user)
  const body = await getJson<{ path: string; content: string }>(
    `/api/agent/file?${params.toString()}`,
    'Could not read the file',
    signal,
  )
  return body.content
}

/** Bytes as the stack prints them: grouped in threes, never abbreviated. */
export function formatSize(size: number): string {
  return `${String(size).replace(/\B(?=(\d{3})+(?!\d))/g, ' ')} B`
}

/** A timestamp as a date and a time, or nothing when the memory has none. */
export function formatUpdated(updated: string): string {
  if (!updated) return 'not dated'
  const when = new Date(updated)
  if (Number.isNaN(when.getTime())) return 'not dated'
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${when.getFullYear()}-${pad(when.getMonth() + 1)}-${pad(when.getDate())} ` +
    `${pad(when.getHours())}:${pad(when.getMinutes())}`
}

/** Who wrote a memory, in the words the rows read. */
export function memoryKindLabel(memory: AgentMemory): string {
  if (memory.kind === 'bd') return 'bd'
  if (memory.kind === 'codex') return 'codex'
  return memory.path.endsWith('/MEMORY.md') ? 'claude index' : 'claude'
}

/** Where a skill was reached from: the source, then the directory holding it. */
export function skillSourceLabel(skill: AgentSkill): string {
  const parent = skill.path.slice(0, skill.path.lastIndexOf('/'))
  return parent ? `${skill.source} ${parent}` : skill.source
}

/**
 * A workspace path as the rail can hold it. The last segment is what tells two
 * workspaces apart, so a path too long for the column loses its head.
 */
export function shortWorkspacePath(path: string, max = 22): string {
  if (path.length <= max) return path
  const tail = path.slice(path.lastIndexOf('/') + 1)
  return `/…${tail}`
}

/** A settings file is read as text; everything else in the stack is Markdown. */
export function isSettingsInstruction(instruction: AgentInstruction): boolean {
  return instruction.kind === 'settings'
}
