/**
 * The brain's one view: the sky, and nothing ringing it.
 *
 * Subjects are the sky's own systems, so there is no second picture of them
 * and no mode to switch between. What the operator does is done on the sky:
 * a star opens its page, a system's name takes the camera to it, Fit takes in
 * the whole. The time line runs along the foot. On a phone, where a sky is too
 * small to aim at, the same subjects and pages are a list.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import type { LibraryGraph } from '../../library/libraryApi'
import type { MapTransform } from '../../hooks/useMapTransform'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import { existedAt, momentOf, NOW } from './mapMoment'
import { subjectOf, subjectsOf } from './brainSubjects'
import SkyMap, { type SkyWords } from './SkyMap'
import './BrainSky.css'

interface Props {
  /** The corpus less the collections the operator has taken off the sky. */
  graph: LibraryGraph
  /** The whole corpus, which places the sky so hiding a collection moves nothing. */
  fullGraph: LibraryGraph
  moment?: number
  matches: ReadonlySet<string> | null
  hoverPath?: string | null
  openPath: string | null
  onOpen: (path: string) => void
  /** The time line, drawn along the sky's foot. */
  timeControl: ReactNode
  /** What the sky is called, for a sky of something other than the brain. */
  label?: string
  placementKey?: string
  nameOf?: (subject: string) => string | undefined
  words?: SkyWords
}

export default function BrainSky({
  graph, fullGraph, moment = NOW, matches, hoverPath, openPath, onOpen, timeControl, label = 'Brain map', placementKey, nameOf, words,
}: Props) {
  const phone = useMediaQuery('(max-width: 768px)')
  const [subject, setSubject] = useState<string | null>(null)
  const [camera, setCamera] = useState<MapTransform | null>(null)
  const [revision, setRevision] = useState(0)

  const visible = useMemo(() => {
    const pages = graph.pages.filter(page => existedAt(momentOf(page.created), moment))
    const paths = new Set(pages.map(page => page.path))
    return { pages, links: graph.links.filter(([a, b]) => paths.has(a) && paths.has(b)), tags: [] } as LibraryGraph
  }, [graph, moment])

  // A page opened anywhere (a star, the rail, a search, a link in the reader)
  // is the sky's choice, and its system is the one in focus.
  const opened = openPath ? fullGraph.pages.find(page => page.path === openPath) : undefined
  const openedSubject = opened ? subjectOf(opened) : null
  useEffect(() => { if (openedSubject) setSubject(openedSubject) }, [openedSubject])

  const fit = () => { setSubject(null); setCamera(null); setRevision(previous => previous + 1) }
  const keepCamera = useCallback((next: MapTransform) => setCamera(next), [])

  return (
    <section className="brain-map" aria-label={label}>
      <div className="brain-map-body is-continuous">
        {phone
          ? <BrainList graph={visible} matches={matches} openPath={openPath} onOpen={onOpen} nameOf={nameOf} />
          : <SkyMap graph={fullGraph} visible={visible} selected={openPath} subject={subject}
            matches={matches} hoverPath={hoverPath} moment={moment} camera={camera} cameraRevision={revision}
            onCameraChange={keepCamera} onSelect={onOpen} onSubject={setSubject}
            placementKey={placementKey} nameOf={nameOf} words={words} />}
      </div>
      <footer className="brain-foot" aria-label="Time">
        {timeControl}
        {!phone && <button type="button" className="kit-button brain-fit" onClick={fit} title="Take in the whole sky (Home)">Fit</button>}
      </footer>
    </section>
  )
}

/** The subjects and their pages as a list, largest subject first. */
function BrainList({ graph, matches, openPath, onOpen, nameOf }: {
  graph: LibraryGraph; matches: ReadonlySet<string> | null; openPath: string | null; onOpen: (path: string) => void
  nameOf?: (subject: string) => string | undefined
}) {
  const [open, setOpen] = useState<string | null>(null)
  const subjects = useMemo(() => subjectsOf(graph.pages)
    .map(group => ({ ...group, name: nameOf?.(group.id) ?? group.name, pages: matches ? group.pages.filter(page => matches.has(page.path)) : group.pages }))
    .filter(group => group.pages.length > 0)
    .sort((a, b) => b.pages.length - a.pages.length || a.name.localeCompare(b.name)), [graph, matches])
  if (subjects.length === 0) return <p className="library-empty">Nothing here at this moment.</p>
  return (
    <div className="brain-list" aria-label="Subjects">
      {subjects.map(group => (
        <div key={group.id} className="brain-list-subject">
          <button type="button" aria-expanded={open === group.id} onClick={() => setOpen(current => current === group.id ? null : group.id)}>
            <span>{group.name}</span><span className="brain-count">{group.pages.length}</span>
          </button>
          {open === group.id && [...group.pages].sort((a, b) => a.title.localeCompare(b.title)).map(page => (
            <button type="button" key={page.path} className="brain-list-page" aria-current={page.path === openPath ? 'true' : undefined}
              onClick={() => onOpen(page.path)}>{page.title}</button>
          ))}
        </div>
      ))}
    </div>
  )
}
