/**
 * The Beads tab's two filters: which projects and which statuses the table
 * keeps. Each row's count is what choosing it would give under the other
 * filters, and the rows keep their places whatever the counts do: projects in
 * order of size, statuses in the order bd works through them.
 *
 * The box toggles one row; the name keeps that row alone, and a second click
 * on the same name brings everything back.
 */

import { memo, useMemo, useState } from 'react'
import Rail, { RailScroll, RailSection } from '../Rail'
import { beadGlyph } from '../../beads/beadStatus'
import { SLICE_STATUSES, STATUS_LABELS, type SliceIndex, type SliceResult } from '../../beads/beadsSlice'

interface FacetRailProps {
  index: SliceIndex
  result: SliceResult
  /** Project indices kept; null keeps every project. */
  projects: ReadonlySet<number> | null
  statuses: ReadonlySet<number> | null
  onProjects: (next: ReadonlySet<number> | null) => void
  onStatuses: (next: ReadonlySet<number> | null) => void
  width: number
  onWidthCommit: (width: number) => void
}

/** One row toggled in or out; every row kept reads as no filter at all. */
function toggled(kept: ReadonlySet<number> | null, item: number, all: number): ReadonlySet<number> | null {
  const next = new Set(kept ?? Array.from({ length: all }, (_, i) => i))
  if (next.has(item)) next.delete(item)
  else next.add(item)
  return next.size === all ? null : next
}

/** One row alone, or everything again when it already stood alone. */
function only(kept: ReadonlySet<number> | null, item: number): ReadonlySet<number> | null {
  return kept?.size === 1 && kept.has(item) ? null : new Set([item])
}

const STATUS_GLYPHS: Record<string, string> = {
  open: beadGlyph('open'),
  in_progress: beadGlyph('in_progress'),
  blocked: beadGlyph('open', true),
  deferred: beadGlyph('deferred'),
  closed: beadGlyph('closed'),
}

function FacetRail({ index, result, projects, statuses, onProjects, onStatuses, width, onWidthCommit }: FacetRailProps) {
  const [find, setFind] = useState('')
  const ordered = useMemo(() => index.projects
    .map((project, p) => ({ project, p }))
    .sort((a, b) => b.project.total - a.project.total || a.project.label.localeCompare(b.project.label)), [index.projects])
  const needle = find.trim().toLowerCase()
  const shown = needle
    ? ordered.filter(({ project }) => project.label.toLowerCase().includes(needle) || project.path.toLowerCase().includes(needle))
    : ordered

  return (
    <Rail className="beads-rail" label="Beads filters" width={width} onWidthCommit={onWidthCommit}>
      <RailSection
        fill
        className="beads-facet"
        title={(
          <span className="beads-facet-heading">
            Project
            <button type="button" className="beads-facet-all" disabled={projects === null} onClick={() => onProjects(null)}>All</button>
          </span>
        )}
      >
        <input
          className="beads-facet-find"
          type="search"
          value={find}
          placeholder="Filter projects…"
          aria-label="Filter projects"
          onChange={event => setFind(event.target.value)}
        />
        <RailScroll role="group" aria-label="Projects">
          {shown.map(({ project, p }) => {
            const kept = projects === null || projects.has(p)
            const count = result.projectCounts[p]
            return (
              <div key={project.path} className={`beads-facet-row${count === 0 ? ' empty' : ''}`} data-ui="beads.project">
                <input
                  type="checkbox"
                  checked={kept}
                  aria-label={`Keep ${project.label}`}
                  onChange={() => onProjects(toggled(projects, p, index.projects.length))}
                />
                <button
                  type="button"
                  className="beads-facet-name"
                  title={`${project.path}\nShow only ${project.label}`}
                  onClick={() => onProjects(only(projects, p))}
                >
                  {project.label}
                </button>
                <span className="beads-facet-count">{count}</span>
              </div>
            )
          })}
          {shown.length === 0 && <p className="beads-facet-note">No project matches.</p>}
        </RailScroll>
      </RailSection>
      <RailSection
        className="beads-facet beads-facet-status"
        title={(
          <span className="beads-facet-heading">
            Status
            <button type="button" className="beads-facet-all" disabled={statuses === null} onClick={() => onStatuses(null)}>All</button>
          </span>
        )}
      >
        <div role="group" aria-label="Statuses">
          {SLICE_STATUSES.map((status, s) => {
            const kept = statuses === null || statuses.has(s)
            const count = result.statusCounts[s]
            return (
              <div key={status} className={`beads-facet-row${count === 0 ? ' empty' : ''}`} data-ui="beads.status">
                <input
                  type="checkbox"
                  checked={kept}
                  aria-label={`Keep ${STATUS_LABELS[status]}`}
                  onChange={() => onStatuses(toggled(statuses, s, SLICE_STATUSES.length))}
                />
                <button
                  type="button"
                  className="beads-facet-name"
                  title={`Show only ${STATUS_LABELS[status]}`}
                  onClick={() => onStatuses(only(statuses, s))}
                >
                  <span className="beads-glyph" aria-hidden="true">{STATUS_GLYPHS[status]}</span>
                  {STATUS_LABELS[status]}
                </button>
                <span className="beads-facet-count">{count}</span>
              </div>
            )
          })}
        </div>
      </RailSection>
    </Rail>
  )
}

export default memo(FacetRail)
