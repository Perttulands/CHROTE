/**
 * Where the operator stands, as a trail of folders to step back along.
 *
 * Each folder on the way is a word that goes there, so the trail is the way up
 * as well as the answer to where am I, and there is no separate Up. Inside a
 * project the trail starts at the project, named, rather than at the root it
 * lives under; a home word, when given, comes first. The early folders give
 * way to the last when the trail is too long for its column, so the folder the
 * operator is in always reads whole.
 */

import { Fragment } from 'react'

export interface PathTrailProps {
  path: string
  onNavigate: (path: string) => void
  /** The folder the trail starts at, and what it is called: a project. */
  base?: { path: string; label: string } | null
  /** A first word before the folders, such as Projects. */
  home?: { label: string; onSelect: () => void } | null
  className?: string
}

export default function PathTrail({ path, onNavigate, base, home, className }: PathTrailProps) {
  const start = base && (path === base.path || path.startsWith(`${base.path}/`)) ? base : null
  const rest = (start ? path.slice(start.path.length) : path).split('/').filter(Boolean)
  const steps: { label: string; path: string }[] = []
  if (start) steps.push({ label: start.label, path: start.path })
  else if (!home) steps.push({ label: '/', path: '/' })
  rest.reduce((at, segment) => {
    const next = `${at.replace(/\/$/, '')}/${segment}`
    steps.push({ label: segment, path: next })
    return next
  }, start ? start.path : '')

  return (
    <nav className={`path-trail${className ? ` ${className}` : ''}`} aria-label="Path">
      {home && (
        <button type="button" className="kit-button path-trail-step" onClick={home.onSelect}>{home.label}</button>
      )}
      {steps.map((step, index) => {
        const last = index === steps.length - 1
        return (
          <Fragment key={step.path}>
            {(index > 0 || home) && <span className="path-trail-sep" aria-hidden="true">/</span>}
            <button
              type="button"
              className={`kit-button path-trail-step${last ? ' is-here' : ''}`}
              aria-current={last ? 'location' : undefined}
              title={step.path}
              onClick={() => onNavigate(step.path)}
            >
              {step.label}
            </button>
          </Fragment>
        )
      })}
    </nav>
  )
}
