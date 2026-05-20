import { toDisplayPath, getRootDisplayName } from '../types'

interface BreadcrumbsProps {
  path: string
  onNavigate: (path: string) => void
}

export function Breadcrumbs({ path, onNavigate }: BreadcrumbsProps) {
  const parts = path.split('/').filter(Boolean)
  const isRoot = parts.length === 0

  // Display Windows-style paths
  const displayPath = toDisplayPath(path)

  return (
    <nav className="fb-breadcrumbs">
      <button
        className="fb-breadcrumb-item fb-breadcrumb-root"
        onClick={() => onNavigate('/')}
        title="Root"
      >
        {isRoot ? displayPath || '/' : '/'}
      </button>
      {parts.map((part, index) => {
        const partPath = '/' + parts.slice(0, index + 1).join('/')
        const isLast = index === parts.length - 1
        // At root level, show Windows paths
        const displayName = index === 0 ? getRootDisplayName(part) : part

        return (
          <span key={partPath} className="fb-breadcrumb-segment">
            <span className="fb-breadcrumb-sep">/</span>
            <button
              className={`fb-breadcrumb-item ${isLast ? 'active' : ''}`}
              onClick={() => onNavigate(partPath)}
              disabled={isLast}
            >
              {displayName}
            </button>
          </span>
        )
      })}
    </nav>
  )
}
