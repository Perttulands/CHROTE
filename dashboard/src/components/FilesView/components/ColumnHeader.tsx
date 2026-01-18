interface ColumnHeaderProps {
  label: string
  sortKey: 'name' | 'size' | 'modified'
  currentSort: string
  currentDir: string
  onSort: (key: 'name' | 'size' | 'modified') => void
  className?: string
}

export function ColumnHeader({
  label,
  sortKey,
  currentSort,
  currentDir,
  onSort,
  className,
}: ColumnHeaderProps) {
  const isActive = currentSort === sortKey

  return (
    <button
      className={`fb-column-header ${className || ''} ${isActive ? 'active' : ''}`}
      onClick={() => onSort(sortKey)}
    >
      {label}
      {isActive && (
        <span className="fb-sort-indicator">
          {currentDir === 'asc' ? '▲' : '▼'}
        </span>
      )}
    </button>
  )
}
