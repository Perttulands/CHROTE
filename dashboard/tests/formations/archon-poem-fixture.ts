import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

type JsonObject = Record<string, unknown>

export type ArchonPoemRoundTripFixture = {
  workspace: string
  board: JsonObject
  layout: JsonObject
  agents: JsonObject[]
  startedStatus: JsonObject
  startedEvents: JsonObject[]
  approvedStatus: JsonObject
  approvedEvents: JsonObject[]
  finalStatus: JsonObject
  finalEvents: JsonObject[]
  runStatus: JsonObject
  runEvents: JsonObject[]
  mission: JsonObject
  draft: JsonObject
  polish: JsonObject
  gate: JsonObject
  cleanup: () => void
}

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..')
const srcDir = path.join(repoRoot, 'src')

function writeInitialBoard(workspace: string) {
  const boardsDir = path.join(workspace, '.formations', 'boards')
  fs.mkdirSync(boardsDir, { recursive: true })
  fs.writeFileSync(path.join(boardsDir, 'poems.formation.toml'), [
    'schema = 1',
    'id = "brd_poems"',
    'slug = "poems"',
    'title = "Poems"',
    'rev = 1',
    '',
  ].join('\n'))
}

function parseLayout(workspace: string, board: JsonObject): JsonObject {
  const raw = fs.readFileSync(path.join(workspace, '.formations', 'layout', 'poems.layout.toml'), 'utf8')
  const nodes: Array<{ id: string; x: number; y: number }> = []
  let current: Partial<{ id: string; x: number; y: number }> | null = null
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (trimmed === '[[node]]') {
      if (current?.id) nodes.push(current as { id: string; x: number; y: number })
      current = {}
      continue
    }
    if (!current) continue
    const match = trimmed.match(/^([A-Za-z]+)\s*=\s*(?:"([^"]*)"|(-?\d+))$/)
    if (!match) continue
    if (match[1] === 'id') current.id = match[2]
    if (match[1] === 'x') current.x = Number(match[3])
    if (match[1] === 'y') current.y = Number(match[3])
  }
  if (current?.id) nodes.push(current as { id: string; x: number; y: number })
  return {
    schema: 1,
    boardId: board.id,
    boardRev: board.rev,
    etag: '"archon-poem-layout"',
    nodes,
    edges: [],
  }
}

function jsonCommand(archonBin: string, env: NodeJS.ProcessEnv, args: string[]): JsonObject {
  const output = execFileSync(archonBin, args, {
    cwd: srcDir,
    env,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  return JSON.parse(output) as JsonObject
}

function arrayCommand(archonBin: string, env: NodeJS.ProcessEnv, args: string[]): JsonObject[] {
  return jsonCommand(archonBin, env, args) as unknown as JsonObject[]
}

function byTitle<T extends JsonObject>(items: unknown, title: string): T {
  const found = Array.isArray(items) ? items.find(item => item && typeof item === 'object' && (item as JsonObject).title === title) : null
  if (!found || typeof found !== 'object') throw new Error(`Archon fixture did not create ${title}`)
  return found as T
}

function first<T>(items: unknown, label: string): T {
  if (!Array.isArray(items) || !items[0]) throw new Error(`Archon fixture missing ${label}`)
  return items[0] as T
}

export function createArchonPoemRoundTripFixture(): ArchonPoemRoundTripFixture {
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'chrote-archon-poem-'))
  const agentsDir = path.join(workspace, 'agents')
  const archonBin = path.join(workspace, 'archon')
  const env = {
    ...process.env,
    CHROTE_AGENTS_DIR: agentsDir,
    CHROTE_FORMATIONS_LAB_HARNESSES: 'lab-fake',
    CHROTE_FORMATIONS_LAB_CWD: workspace,
    CHROTE_FORMATIONS_LAB_ROOTS: workspace,
  }

  execFileSync('go', ['build', '-o', archonBin, './cmd/archon'], {
    cwd: srcDir,
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  writeInitialBoard(workspace)

  const archon = (...args: string[]) => jsonCommand(archonBin, env, ['--workspace', workspace, ...args])
  archon('agent', 'new', 'lab-poet', '--kind', 'poet', '--harness', 'lab-fake', '--json')
  archon('agent', 'new', 'lab-poem-reviewer', '--kind', 'reviewer', '--harness', 'lab-fake', '--json')
  archon('board', 'list', '--json')
  archon('mission', 'create', 'poems', '--title', 'Simple poem', '--goal', 'Create a simple poem', '--bead', 'home-vdki.34.1', '--json')
  archon('formation', 'create', 'poems', 'solo', '--title', 'Draft poem', '--x', '320', '--y', '120', '--json')
  archon('formation', 'create', 'poems', 'solo', '--title', 'Polish poem', '--x', '860', '--y', '120', '--json')
  archon('gate', 'create', 'poems', '--title', 'Human review', '--kinds', 'human', '--criterion', 'Draft is ready to polish', '--json')

  let board = archon('board', 'inspect', 'poems', '--json')
  const mission = byTitle<JsonObject>(board.missions, 'Simple poem')
  const draft = byTitle<JsonObject>(board.formations, 'Draft poem')
  const polish = byTitle<JsonObject>(board.formations, 'Polish poem')
  const gate = byTitle<JsonObject>(board.gates, 'Human review')
  const draftInput = first<JsonObject>(draft.inputs, 'draft input')
  const draftOutput = first<JsonObject>(draft.outputs, 'draft output')
  const draftSlot = first<JsonObject>(draft.slots, 'draft slot')
  const polishInput = first<JsonObject>(polish.inputs, 'polish input')
  const polishSlot = first<JsonObject>(polish.slots, 'polish slot')

  archon('formation', 'assign', 'poems', String(draft.id), '--slot', String(draftSlot.id), '--agent', 'lab-poet', '--harness', 'lab-fake', '--json')
  archon('formation', 'assign', 'poems', String(polish.id), '--slot', String(polishSlot.id), '--agent', 'lab-poem-reviewer', '--harness', 'lab-fake', '--json')
  archon('mission', 'wire', 'poems', String(mission.id), `${draft.id}:${draftInput.id}`, '--json')
  archon('formation', 'wire', 'poems', `${draft.id}:${draftOutput.id}`, `${gate.id}:in`, '--json')
  archon('formation', 'wire', 'poems', `${gate.id}:pass`, `${polish.id}:${polishInput.id}`, '--json')
  board = archon('board', 'inspect', 'poems', '--json')

  const started = archon('mission', 'run', 'poems', '--mission', String(mission.id), '--json')
  const runId = String(started.runId)
  const startedStatus = (started.status && typeof started.status === 'object' ? started.status : started) as JsonObject
  const startedEvents = arrayCommand(archonBin, env, ['--workspace', workspace, 'run', 'logs', runId, '--json'])
  const approvedStatus = archon('gate', 'approve', runId, String(gate.id), '--reason', 'draft approved', '--json')
  const approvedEvents = arrayCommand(archonBin, env, ['--workspace', workspace, 'run', 'logs', runId, '--json'])
  const finalStatus = archon('run', 'resume', runId, '--reason', 'gate approved', '--json')
  const finalEvents = arrayCommand(archonBin, env, ['--workspace', workspace, 'run', 'logs', runId, '--json'])

  return {
    workspace,
    board,
    layout: parseLayout(workspace, board),
    agents: [
      { id: 'lab-poet', displayName: 'Lab Poet', harnessDefault: 'lab-fake', assignable: true, liveness: 'live' },
      { id: 'lab-poem-reviewer', displayName: 'Lab Poem Reviewer', harnessDefault: 'lab-fake', assignable: true, liveness: 'live' },
    ],
    startedStatus,
    startedEvents,
    approvedStatus,
    approvedEvents,
    finalStatus,
    finalEvents,
    runStatus: approvedStatus,
    runEvents: approvedEvents,
    mission,
    draft,
    polish,
    gate,
    cleanup: () => fs.rmSync(workspace, { recursive: true, force: true }),
  }
}
