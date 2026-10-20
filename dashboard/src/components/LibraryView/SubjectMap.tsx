import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { useTheme } from '../../theme/ThemeContext'
import type { LibraryGraph } from '../../library/libraryApi'
import LibraryMap, { type MapProps } from './Map'
import { existedAt, momentOf, NOW } from './mapMoment'
import { kindOf, subjectConnections, subjectGraph, subjectHue, subjectName, subjectOf, subjectsOf } from './brainSubjects'
import ContinuousMap from './ContinuousMap'
import BrainViewControls from './BrainViewControls'
import type { BrainViewSnapshot, BrainViewMode } from './brainViewState'
import type { MapTransform } from '../../hooks/useMapTransform'
import './SubjectMap.css'

interface SubjectMapProps extends MapProps {
  fullGraph: LibraryGraph
  scrub: number
  hiddenShelves: string[]
  onRestoreFilters: (scrub: number, hiddenShelves: string[]) => void
  timeControl: ReactNode
}

export default function SubjectMap(props: SubjectMapProps) {
  const { graph, openPath, hoverPath, matches, soloShelf, moment = NOW, onOpen } = props
  const theme = useTheme()
  const [mode, setMode] = useState<BrainViewMode>('subjects')
  const [cameras, setCameras] = useState<BrainViewSnapshot['cameras']>({ map: null, subjects: null })
  const [cameraRevision, setCameraRevision] = useState(0)
  const mapCamera = useCallback((camera: MapTransform) => setCameras(previous => ({ ...previous, map: camera })), [])
  const subjectCamera = useCallback((camera: MapTransform) => setCameras(previous => ({ ...previous, subjects: camera })), [])
  const [subject, setSubject] = useState<string | null>(null)
  const [facet, setFacet] = useState('')
  const [collection, setCollection] = useState('')
  const [hiddenKinds, setHiddenKinds] = useState<ReadonlySet<string>>(new Set(['source']))
  const [picked, setPicked] = useState<string | null>(null)
  const facets = useMemo(() => [...new Set(graph.pages.flatMap(page => page.facets ?? []))].sort(), [graph])
  const collections = useMemo(() => [...new Set(graph.pages.map(page => page.shelf))].filter(Boolean).sort(), [graph])
  const kinds = useMemo(() => [...new Set(graph.pages.map(kindOf))].sort(), [graph])
  const dated = useMemo(() => graph.pages.filter(page => existedAt(momentOf(page.created), moment)), [graph, moment])
  const filtered = useMemo(() => dated.filter(page => (!facet || page.facets?.includes(facet))
    && (!collection || page.shelf === collection)), [dated, facet, collection])
  const subjects = useMemo(() => subjectsOf(filtered), [filtered])
  const visible = useMemo(() => {
    const pages = filtered.filter(page => !hiddenKinds.has(kindOf(page)))
    const paths = new Set(pages.map(page => page.path))
    return { pages, links: graph.links.filter(([a, b]) => paths.has(a) && paths.has(b)), tags: [] } as LibraryGraph
  }, [filtered, hiddenKinds, graph.links])
  const selected = picked
  const current = subjects.find(group => group.id === subject)
  const focused = useMemo(() => current ? subjectGraph(visible, current.id) : null, [visible, current])
  const connections = useMemo(() => current ? subjectConnections({ ...graph, pages: filtered }, current.id, selected) : [], [graph, filtered, current, selected])
  const colors = useMemo(() => new Map(focused?.pages.map(page => [page.shelf, subjectHue(subjectOf(page), theme.shelves)])), [focused, theme.shelves])
  const selectedPage = graph.pages.find(page => page.path === selected)
  const openedPage = graph.pages.find(page => page.path === openPath)
  const openedSubject = openedPage ? subjectOf(openedPage) : null
  const openedKind = openedPage ? kindOf(openedPage) : null
  const externalReferences = useMemo(() => {
    if (!selected || !current) return []
    const paths = new Set(graph.links.flatMap(([from, to]) => from === selected ? [to] : to === selected ? [from] : []))
    return graph.pages.filter(page => paths.has(page.path) && subjectOf(page) !== current.id)
      .sort((a, b) => a.title.localeCompare(b.title))
  }, [graph, selected, current])

  // A page opened from search or the reader's links selects its current subject.
  useEffect(() => {
    if (!openPath) return
    if (!openedSubject || !openedKind) return
    setSubject(openedSubject)
    setPicked(openPath)
    setCollection('')
    setFacet('')
    setHiddenKinds(previous => new Set([...previous].filter(kind => kind !== openedKind)))
  }, [openPath, openedSubject, openedKind])

  const selectSubject = (id: string) => {
    setSubject(id)
    setPicked(null)
    setCameras(previous => ({ ...previous, subjects: { x: 0, y: 0, scale: 1 } }))
    if (mode === 'subjects') setCameraRevision(previous => previous + 1)
  }
  const selectPage = (path: string) => {
    setPicked(path)
    const page = graph.pages.find(entry => entry.path === path)
    if (page) setSubject(subjectOf(page))
  }
  const overview = () => {
    setSubject(null); setPicked(null)
    if (mode === 'map') {
      setCameras(previous => ({ ...previous, map: null }))
      setCameraRevision(previous => previous + 1)
    }
  }
  const fit = () => {
    setCameras(previous => ({ ...previous, [mode]: mode === 'map' ? null : { x: 0, y: 0, scale: 1 } }))
    setCameraRevision(previous => previous + 1)
  }
  const snapshot: BrainViewSnapshot = { mode, subject, picked, facet, collection, hiddenKinds: [...hiddenKinds],
    cameras, scrub: props.scrub, hiddenShelves: props.hiddenShelves }
  const restore = (saved: BrainViewSnapshot) => {
    setMode(saved.mode); setSubject(saved.subject); setPicked(saved.picked)
    setFacet(saved.facet); setCollection(saved.collection); setHiddenKinds(new Set(saved.hiddenKinds))
    setCameras(saved.cameras); setCameraRevision(previous => previous + 1)
    props.onRestoreFilters(saved.scrub, saved.hiddenShelves)
  }

  const selection = <>
            <div className="brain-page-selection" aria-live="polite">
              {selectedPage ? <>
                <div className="brain-selection-copy"><strong>{selectedPage.title}</strong><span className="brain-count">{kindOf(selectedPage)} · {selectedPage.path}</span>
                {selectedPage.summary && <p>{selectedPage.summary}</p>}</div>
                <div className="brain-selection-actions">
                <button type="button" onClick={() => onOpen(selectedPage.path)}>Open page</button>
                <button type="button" onClick={() => setPicked(null)}>Clear selection</button></div>
              </> : <span className="brain-count">Select a page to see its references.</span>}
            </div>
            {externalReferences.length > 0 && <div className="brain-external" aria-label="References in other subjects">
              {externalReferences.map(page => <button type="button" key={page.path} onClick={() => {
                setSubject(subjectOf(page)); setPicked(page.path); setCollection(''); setFacet('')
                setHiddenKinds(previous => new Set([...previous].filter(kind => kind !== kindOf(page))))
              }}><span>{page.title}</span><span className="brain-count">{subjectName(subjectOf(page))} · {kindOf(page)}</span></button>)}
            </div>}
  </>

  return (
    <section className="brain-map" aria-label="Brain map">
      <div className="brain-map-controls">
        <button type="button" onClick={overview} disabled={!current}>Overview</button>
        {mode === 'map' ? <label className="brain-subject-picker">Subject <select aria-label="Map subject" value={subject ?? ''}
          onChange={event => event.target.value ? selectSubject(event.target.value) : overview()}>
          <option value="">All subjects</option>
          {subjects.map(group => <option key={group.id} value={group.id}>{group.name} · {group.pages.length}</option>)}
        </select></label> : current && <span className="brain-breadcrumb">{current.collection} / {current.name}</span>}
        <label>Collection <select aria-label="Map collection" value={collection} onChange={event => { setCollection(event.target.value); overview() }}>
          <option value="">All</option>
          {collections.map(name => <option key={name} value={name}>{name}</option>)}
        </select></label>
        <label>Facet <select aria-label="Map facet" value={facet} onChange={event => { setFacet(event.target.value); overview() }}>
          <option value="">All</option>
          {facets.map(name => <option key={name} value={name}>{name}</option>)}
        </select></label>
      </div>
      <div className="brain-map-kinds" aria-label="Page kinds">
        {kinds.map(kind => <label key={kind}>
          <input type="checkbox" checked={!hiddenKinds.has(kind)} onChange={() => setHiddenKinds(previous => {
            const next = new Set(previous)
            if (next.has(kind)) next.delete(kind)
            else next.add(kind)
            return next
          })} />
          <span className={`brain-kind-symbol brain-kind-${kind}`} />{kind}
          <span className="brain-count">{filtered.filter(page => kindOf(page) === kind).length}</span>
        </label>)}
        <span className="brain-count">{subjects.length} subjects · {visible.pages.length} / {filtered.length} pages shown</span>
      </div>
      <div className={`brain-map-body${mode === 'map' ? ' is-continuous' : ''}${mode === 'subjects' && current ? ' has-subject' : ''}`}>
        {mode === 'map' && <>
          <ContinuousMap graph={props.fullGraph} visible={visible} selected={selected} subject={subject}
            matches={matches} hoverPath={hoverPath} moment={moment} camera={cameras.map} cameraRevision={cameraRevision}
            onCameraChange={mapCamera} onSelect={selectPage} onSubject={selectSubject} />
          {selectedPage && selection}
        </>}
        {mode === 'subjects' && current && focused && (
          <div className="brain-subject-detail" style={{ '--subject-color': subjectHue(current.id, theme.shelves) } as CSSProperties}>
            <div className="brain-subject-heading">
              <span>{current.name}</span><span className="brain-count">{current.pages.length} pages</span>
            </div>
            <div className="brain-detail-canvas">
              {focused.pages.length > 0 ? <LibraryMap {...props} graph={focused} openPath={selected}
                hoverPath={hoverPath} soloShelf={null} hues={colors} selectionScale={1} selectedLinksOnly onOpen={selectPage}
                camera={cameras.subjects} cameraRevision={cameraRevision} onCameraChange={subjectCamera} />
                : <p className="library-empty">No pages match the selected kinds.</p>}
            </div>
            {selection}

            <div className="brain-connections">
              <span className="brain-count">{selectedPage ? 'Page connections' : 'Subject connections'}</span>
              {connections.length === 0 && <span className="brain-count">No links to other subjects</span>}
              {connections.map(([id, count]) => <button type="button" key={id} onClick={() => selectSubject(id)}>
                {subjectName(id)} <span className="brain-count">{count}</span>
              </button>)}
            </div>
          </div>
        )}
        {mode === 'subjects' && <div className="brain-atlas" aria-label="Subjects">
          {subjects.length === 0 && <p className="library-empty">No subjects match these filters.</p>}
          {subjects.map(group => {
            const pages = group.pages.filter(page => !hiddenKinds.has(kindOf(page)))
            const found = matches ? pages.filter(page => matches.has(page.path)).length : null
            const highlighted = group.pages.some(page => page.path === hoverPath)
            return <button type="button" key={group.id}
              className={`brain-subject${group.id === subject ? ' selected' : ''}${highlighted ? ' highlighted' : ''}`}
              aria-label={`${group.name}, ${group.pages.length} ${group.pages.length === 1 ? 'page' : 'pages'}`}
              aria-expanded={group.id === current?.id}
              style={{ '--subject-color': subjectHue(group.id, theme.shelves), opacity: (soloShelf && soloShelf !== group.collection) || (found === 0) ? 0.4 : 1 } as CSSProperties}
              onClick={() => selectSubject(group.id)}>
              <span className="brain-subject-collection">{group.collection}</span>
              <svg viewBox="0 0 180 94" aria-hidden="true">
                <ellipse cx="90" cy="47" rx="79" ry="40" />
                {pages.slice(0, 24).map((page, index) => {
                  const angle = index * 2.39996
                  const radius = Math.sqrt(index + 1) * 6
                  const x = 90 + Math.cos(angle) * radius * 1.8
                  const y = 47 + Math.sin(angle) * radius
                  const kind = kindOf(page)
                  return kind === 'research' ? <rect key={page.path} x={x - 2.5} y={y - 2.5} width="5" height="5" />
                    : kind === 'source' ? <path key={page.path} d={`M${x},${y - 3}l3,3l-3,3l-3,-3z`} />
                    : <circle key={page.path} cx={x} cy={y} r={kind === 'topic' ? 4 : 2} />
                })}
              </svg>
              <span className="brain-subject-name">{group.name}</span>
              <span className="brain-count">{group.pages.length} {group.pages.length === 1 ? 'page' : 'pages'}{found !== null ? ` · ${found} matches` : ''}</span>
              <span className="brain-subject-kinds">{[...group.kinds].map(([kind, count]) => `${count} ${kind}`).join(' · ')}</span>
            </button>
          })}
        </div>}
      </div>
      <BrainViewControls snapshot={snapshot} onRestore={restore} onMode={setMode} onFit={fit} timeControl={props.timeControl} />
    </section>
  )
}
