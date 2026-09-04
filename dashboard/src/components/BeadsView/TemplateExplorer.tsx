/**
 * Formula and molecule inspection. The server returns bd's full objects, so
 * this renderer names their structure without throwing away fields it does
 * not yet know about.
 */

import type { BeadsStructure } from '../../beads/beadsApi'

function labelFor(key: string): string {
  if (key === 'vars') return 'Variables'
  const words = key.replace(/([a-z])([A-Z])/g, '$1 $2').replace(/_/g, ' ')
  return words.charAt(0).toUpperCase() + words.slice(1)
}

function Structure({ value, field }: { value: unknown; field?: string }) {
  if (value === null || value === undefined || value === '') return <span className="beads-template-empty-value">None</span>
  if (typeof value === 'boolean') return <span>{value ? 'Yes' : 'No'}</span>
  if (typeof value === 'string' || typeof value === 'number') {
    const shown = field === 'status' ? String(value).replace(/_/g, ' ') : String(value)
    return <span>{shown}</span>
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return <span className="beads-template-empty-value">None</span>
    return (
      <ol className="beads-template-list">
        {value.map((item, index) => <li key={index}><Structure value={item} /></li>)}
      </ol>
    )
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return <span className="beads-template-empty-value">None</span>
    return (
      <dl className="beads-template-fields">
        {entries.map(([key, item]) => (
          <div key={key} className="beads-template-field">
            <dt>{labelFor(key)}</dt>
            <dd><Structure value={item} field={key} /></dd>
          </div>
        ))}
      </dl>
    )
  }
  return <span>{String(value)}</span>
}

function firstText(detail: BeadsStructure, keys: string[]): string | undefined {
  for (const key of keys) {
    const value = detail[key]
    if (typeof value === 'string' && value.trim() !== '') return value
  }
  return undefined
}

interface TemplateExplorerProps {
  kind: 'formula' | 'molecule'
  fallbackName: string
  projectName: string
  projectPath: string
  loading: boolean
  error: string | null
  detail: BeadsStructure | null
}

export default function TemplateExplorer({
  kind,
  fallbackName,
  projectName,
  projectPath,
  loading,
  error,
  detail,
}: TemplateExplorerProps) {
  const title = detail
    ? firstText(detail, kind === 'formula' ? ['name', 'formula', 'title'] : ['title', 'id']) ?? fallbackName
    : fallbackName
  const description = detail ? firstText(detail, ['description']) : undefined
  const root = detail && typeof detail.root === 'object' && detail.root !== null && !Array.isArray(detail.root)
    ? detail.root as BeadsStructure
    : null
  const source = detail
    ? firstText(detail, ['source', 'source_formula', 'origin']) ?? (root ? firstText(root, ['source', 'source_formula', 'origin']) : undefined)
    : undefined
  const omitted = new Set(['name', 'formula', 'title', 'description', 'source', 'source_formula', 'origin'])
  const fields = detail ? Object.entries(detail).filter(([key]) => !omitted.has(key)) : []

  return (
    <article className="beads-template-detail" data-ui="beads.template">
      <header className="beads-template-header">
        <span>{kind === 'formula' ? 'Formula' : 'Molecule'} · {projectName}</span>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </header>
      {loading && <p className="beads-empty">Reading {kind}…</p>}
      {!loading && error && <p className="beads-error">{error}</p>}
      {!loading && !error && detail && (
        <>
          <section className="beads-template-provenance">
            <h2>Provenance</h2>
            <dl>
              <div><dt>Store</dt><dd>{projectName} · {projectPath}</dd></div>
              <div><dt>Source</dt><dd>{source ?? 'Not reported by bd'}</dd></div>
            </dl>
          </section>
          {fields.map(([key, value]) => (
            <section key={key} className="beads-template-section">
              <h2>{labelFor(key)}</h2>
              <Structure value={value} />
            </section>
          ))}
        </>
      )}
    </article>
  )
}
