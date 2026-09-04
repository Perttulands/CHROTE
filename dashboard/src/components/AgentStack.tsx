/**
 * The three layers an agent loads, drawn once for both surfaces that show them.
 *
 * Instructions in the order the harness reads them, the skills that are
 * reachable, and the memories written for the folder — each row naming where it
 * came from, because the source is the fact the operator is checking. A row
 * opens in place: the file is fetched when it is asked for, never before, and a
 * file the server cannot read says so rather than going missing.
 *
 * The panel and the Agents tab render this same component, so what the operator
 * learns from a session's menu is what the tab teaches him about the folder.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import Markdown from './Markdown'
import Editor from './Editor'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { copyAndAnnounce } from '../utils/clipboard'
import type { MenuGroup } from './Menu'
import MenuTarget from './MenuTarget'
import { writeTextFile } from './FilesView/fileService'
import {
  fetchAgentFile,
  formatSize,
  formatUpdated,
  isSettingsInstruction,
  memoryKindLabel,
  skillSourceLabel,
  type AgentContext,
} from '../agents/agentContextApi'
import './AgentStack.css'

interface AgentStackProps {
  context: AgentContext
  /** Filters skills and memories by name; instructions are the whole stack. */
  query?: string
}

type OpenRow = { section: 'instructions' | 'skills' | 'memories'; path: string }

function sameRow(left: OpenRow | null, right: OpenRow): boolean {
  return left !== null && left.section === right.section && left.path === right.path
}

export default function AgentStack({ context, query = '' }: AgentStackProps) {
  const { openSendToSession } = useSession()
  const { announce } = useStatus()
  const [open, setOpen] = useState<OpenRow | null>(null)
  const [content, setContent] = useState<string | null>(null)
  const [readError, setReadError] = useState<string | null>(null)
  const [draft, setDraft] = useState<string | null>(null)

  // A new folder or harness is a new stack: nothing that was open belongs to it.
  useEffect(() => {
    setOpen(null)
    setContent(null)
    setReadError(null)
    setDraft(null)
  }, [context.folder, context.harness, context.user])

  const needle = query.trim().toLowerCase()
  const skills = useMemo(
    () => (needle
      ? context.skills.filter(skill =>
        skill.name.toLowerCase().includes(needle) || skill.description.toLowerCase().includes(needle))
      : context.skills),
    [context.skills, needle],
  )
  const memories = useMemo(
    () => (needle ? context.memories.filter(memory => memory.title.toLowerCase().includes(needle)) : context.memories),
    [context.memories, needle],
  )

  // Reading the file and putting it in the editor are the same request with a
  // different ending, so the fetch is written once: the text arrives, and an
  // edit starts from it rather than from whatever was read before.
  const showRow = useCallback((row: OpenRow, intent: 'read' | 'edit' = 'read') => {
    setOpen(row)
    setContent(null)
    setReadError(null)
    setDraft(null)
    fetchAgentFile(row.path, context.folder, context.harness, context.user)
      .then(text => {
        setContent(text)
        if (intent === 'edit') setDraft(text)
      })
      .catch((cause: unknown) => setReadError(cause instanceof Error ? cause.message : 'Could not read the file'))
  }, [context.folder, context.harness, context.user])

  // A click on the row is a toggle; the menu's Open is a request for a state.
  const openRow = useCallback((row: OpenRow) => {
    if (sameRow(open, row)) {
      setOpen(null)
      setDraft(null)
      return
    }
    showRow(row)
  }, [open, showRow])

  /** The actions every file in the stack offers, wherever it is listed. */
  const fileMenu = (row: OpenRow, readable: boolean) => (): MenuGroup[] => {
    const unreadable = readable ? undefined : 'The server cannot read this file'
    return [
      {
        id: 'read',
        rows: [
          { id: 'open', label: 'Open', disabled: !readable, reason: unreadable, onSelect: () => showRow(row) },
          { id: 'edit', label: 'Edit', disabled: !readable, reason: unreadable, onSelect: () => showRow(row, 'edit') },
        ],
      },
      {
        id: 'hand',
        rows: [
          { id: 'copy-path', label: 'Copy path', onSelect: () => { void copyAndAnnounce(row.path, row.path, announce) } },
          { id: 'send', label: 'Send', onSelect: () => openSendToSession({ reference: `path ${row.path}` }) },
        ],
      },
    ]
  }

  const saveDraft = useCallback(async () => {
    if (open === null || draft === null) return
    try {
      await writeTextFile(open.path, draft)
      setContent(draft)
      setDraft(null)
      announce(`Saved ${open.path}`, 'success')
    } catch (cause: unknown) {
      announce(cause instanceof Error ? cause.message : `Could not save ${open.path}`, 'error')
    }
  }, [announce, draft, open])

  const expansion = (row: OpenRow, path: string, asText: boolean) => {
    if (!sameRow(open, row)) return null
    if (draft !== null) {
      return (
        <div className="agent-expansion" key={`${path}-edit`}>
          <Editor
            value={draft}
            onChange={setDraft}
            onSave={() => { void saveDraft() }}
            onCancel={() => setDraft(null)}
            label={`Edit ${path}`}
            autoFocus
          />
          <div className="agent-expansion-actions">
            <button type="button" className="agent-word" onClick={() => { void saveDraft() }}>Save</button>
            <button type="button" className="agent-word" onClick={() => setDraft(null)}>Discard</button>
          </div>
        </div>
      )
    }
    if (readError !== null) return <div className="agent-expansion agent-note" key={`${path}-error`}>{readError}</div>
    if (content === null) return <div className="agent-expansion agent-note" key={`${path}-loading`}>Reading…</div>
    return (
      <div className="agent-expansion" key={`${path}-body`}>
        {asText ? <pre className="agent-plain">{content}</pre> : <Markdown content={content} basePath={path} />}
      </div>
    )
  }

  return (
    <div className="agent-stack">
      <section className="agent-section">
        <h3>Instructions</h3>
        <div className="agent-rule" />
        {context.instructions.length === 0 && (
          <div className="agent-note">No instruction file reaches this folder.</div>
        )}
        {context.instructions.map(instruction => {
          const row: OpenRow = { section: 'instructions', path: instruction.path }
          const isOpen = sameRow(open, row)
          return (
            <div key={instruction.path}>
              <MenuTarget label={`Actions for ${instruction.path}`} groups={fileMenu(row, instruction.readable)}>
                <div
                  className={`agent-row agent-instruction ${isOpen ? 'open' : ''} ${instruction.readable ? '' : 'unreadable'}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => { if (instruction.readable) openRow(row) }}
                  onKeyDown={event => {
                    if (event.key !== 'Enter' && event.key !== ' ') return
                    event.preventDefault()
                    if (instruction.readable) openRow(row)
                  }}
                >
                  <span className="agent-scope">{instruction.scope}</span>
                  <span className="agent-path">{instruction.path}</span>
                  {instruction.link && <span className="agent-note-inline">links to {instruction.link}</span>}
                  {instruction.readable
                    ? <span className="agent-size">{formatSize(instruction.size)}</span>
                    : <span className="agent-note-inline agent-right">not readable by the server</span>}
                  {isOpen && instruction.readable && content !== null && draft === null && (
                    <button
                      type="button"
                      className="agent-word"
                      onClick={event => { event.stopPropagation(); setDraft(content) }}
                    >
                      Edit
                    </button>
                  )}
                </div>
              </MenuTarget>
              {expansion(row, instruction.path, isSettingsInstruction(instruction))}
            </div>
          )
        })}
      </section>

      <section className="agent-section">
        <h3>Skills</h3>
        <div className="agent-rule" />
        {skills.length === 0 && <div className="agent-note">No skill reaches this folder.</div>}
        {skills.map(skill => {
          const path = `${skill.path}/SKILL.md`
          const row: OpenRow = { section: 'skills', path }
          const isOpen = sameRow(open, row)
          return (
            <div key={skill.path}>
              <MenuTarget label={`Actions for ${path}`} groups={fileMenu(row, true)}>
                <div
                  className={`agent-row agent-skill ${isOpen ? 'open' : ''}`}
                  role="button"
                  tabIndex={0}
                  onClick={() => openRow(row)}
                  onKeyDown={event => {
                    if (event.key !== 'Enter' && event.key !== ' ') return
                    event.preventDefault()
                    openRow(row)
                  }}
                >
                  <span className="agent-name">{skill.name}</span>
                  <span className="agent-description">{skill.description}</span>
                  <span className="agent-source">{skillSourceLabel(skill)}</span>
                  {isOpen && content !== null && draft === null && (
                    <button
                      type="button"
                      className="agent-word"
                      onClick={event => { event.stopPropagation(); setDraft(content) }}
                    >
                      Edit
                    </button>
                  )}
                </div>
              </MenuTarget>
              {expansion(row, path, false)}
            </div>
          )
        })}
      </section>

      <section className="agent-section">
        <h3>Memories</h3>
        <div className="agent-rule" />
        {memories.length === 0 && <div className="agent-note">Nothing has been remembered for this folder.</div>}
        {memories.map(memory => {
          const row: OpenRow = { section: 'memories', path: memory.path }
          const isOpen = sameRow(open, row)
          const readable = memory.readable && memory.path !== ''
          const line = (
            <div
              className={`agent-row agent-memory ${isOpen ? 'open' : ''} ${memory.readable ? '' : 'unreadable'}`}
              role={readable ? 'button' : undefined}
              tabIndex={readable ? 0 : undefined}
              onClick={() => { if (readable) openRow(row) }}
              onKeyDown={event => {
                if (!readable || (event.key !== 'Enter' && event.key !== ' ')) return
                event.preventDefault()
                openRow(row)
              }}
            >
              <span className="agent-title">{memory.title}</span>
              {memory.readable
                ? <span className="agent-updated">{formatUpdated(memory.updated)}</span>
                : <span className="agent-updated">not readable by the server</span>}
              <span className="agent-kind">{memoryKindLabel(memory)}</span>
            </div>
          )
          return (
            <div key={`${memory.kind}-${memory.path}-${memory.title}`}>
              {/* A memory bd keeps has no file of its own, so it has nothing a menu could act on. */}
              {memory.path === ''
                ? line
                : (
                  <MenuTarget label={`Actions for ${memory.path}`} groups={fileMenu(row, memory.readable)}>
                    {line}
                  </MenuTarget>
                )}
              {memory.path !== '' && expansion(row, memory.path, false)}
            </div>
          )
        })}
      </section>
    </div>
  )
}
