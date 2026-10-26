/**
 * A project's landing: what the Files tab shows on entering a project, before
 * a file is opened.
 *
 * It answers what the project is and what is happening in it: its name and
 * place, the sessions running there (each a Peek away), its open Beads, the
 * documents that say what it is (the agents' instructions, the vision, the
 * requirements, the architecture), and its README read in the theme. Send
 * hands the project to an agent, offering a new one in its folder, the way the
 * Agents tab does.
 */

import { useEffect, useMemo, useState } from 'react'
import Markdown from '../Markdown'
import PanelPath from '../PanelPath'
import { useSession } from '../../context/SessionContext'
import { getSessionKey } from '../../types'
import { workspaceName, type Workspace } from '../../workspaces/workspacesApi'
import { fetchDirectory, getErrorMessage, readTextFile } from './fileService'
import { projectFacts } from './FilesProjects'
import type { FileItem } from './types'

/** The documents that say what a project is, in the order they are listed. */
const KEY_DOCUMENTS = ['README.md', 'AGENTS.md', 'CLAUDE.md', 'VISION.md', 'PRD.md', 'ARCHITECTURE.md', 'CONTRIBUTING.md', 'CHANGELOG.md']

interface ProjectLandingProps {
  project: Workspace
  onOpenPath: (path: string) => void
}

export default function ProjectLanding({ project, onOpenPath }: ProjectLandingProps) {
  const { sessions, openFloatingModal, openSendToSession } = useSession()
  const [items, setItems] = useState<FileItem[] | null>(null)
  const [readme, setReadme] = useState<{ path: string; text: string } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const name = workspaceName(project.path)

  useEffect(() => {
    let current = true
    setItems(null)
    setReadme(null)
    setError(null)
    fetchDirectory(project.path)
      .then(listed => {
        if (!current) return
        setItems(listed)
        const file = listed.find(item => !item.isDir && item.name.toLowerCase() === 'readme.md')
        if (file) return readTextFile(file.path).then(text => { if (current) setReadme({ path: file.path, text }) })
      })
      .catch((cause: unknown) => { if (current) setError(getErrorMessage(cause, 'read')) })
    return () => { current = false }
  }, [project.path])

  const documents = useMemo(() => {
    const byName = new Map((items ?? []).filter(item => !item.isDir).map(item => [item.name, item]))
    return KEY_DOCUMENTS.flatMap(document => byName.get(document) ?? [])
  }, [items])

  const peek = (sessionName: string) => {
    const session = sessions.find(candidate => candidate.name === sessionName)
    openFloatingModal(getSessionKey(sessionName, session?.unixUser))
  }

  return (
    <article className="project-landing" aria-label={`Project ${name}`} data-ui="files.project">
      <header className="project-landing-head">
        <h2>{name}</h2>
        <PanelPath path={project.path} className="project-landing-path" />
        <span className="project-landing-facts">{projectFacts(project).join(' · ')}</span>
        <button
          type="button"
          className="files-panel-action"
          onClick={() => openSendToSession({ reference: `project ${project.path}`, launch: { label: `Launch in ${name}`, folder: project.path } })}
        >
          Send…
        </button>
      </header>

      {project.sessions.length > 0 && (
        <section className="project-landing-section" aria-label="Sessions here">
          <h3>Sessions</h3>
          {project.sessions.map(session => (
            <button type="button" key={session} className="project-landing-row" onClick={() => peek(session)}>
              {session}<span className="project-landing-hint">Peek</span>
            </button>
          ))}
        </section>
      )}

      {documents.length > 0 && (
        <section className="project-landing-section" aria-label="Key documents">
          <h3>Documents</h3>
          {documents.map(document => (
            <button type="button" key={document.path} className="project-landing-row" onClick={() => onOpenPath(document.path)}>
              {document.name}
            </button>
          ))}
        </section>
      )}

      {error && <p className="files-panel-note">{error}</p>}
      {readme !== null && (
        <section className="project-landing-readme" aria-label="README">
          <Markdown content={readme.text} basePath={readme.path} onOpenPath={onOpenPath} className="files-panel-markdown" />
        </section>
      )}
    </article>
  )
}
