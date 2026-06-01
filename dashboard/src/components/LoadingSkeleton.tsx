import type { CSSProperties } from 'react'

interface SkeletonProps {
  width?: string | number
  height?: string | number
  borderRadius?: string | number
  className?: string
  style?: CSSProperties
}

function Skeleton({
  width = '100%',
  height = 16,
  borderRadius = 4,
  className = '',
  style = {},
}: SkeletonProps) {
  return (
    <span
      className={`skeleton ${className}`}
      style={{
        width: typeof width === 'number' ? `${width}px` : width,
        height: typeof height === 'number' ? `${height}px` : height,
        borderRadius: typeof borderRadius === 'number' ? `${borderRadius}px` : borderRadius,
        ...style,
      }}
    />
  )
}

export function SkeletonCard({ count = 3 }: { count?: number }) {
  return (
    <div className="skeleton-card-list">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="skeleton-card">
          <Skeleton width="60%" height={18} borderRadius={4} />
          <Skeleton width="40%" height={14} borderRadius={4} />
          <div className="skeleton-card-row">
            <Skeleton width="30%" height={12} borderRadius={3} />
            <Skeleton width="20%" height={12} borderRadius={3} />
          </div>
        </div>
      ))}
    </div>
  )
}

export function SkeletonText({ lines = 3 }: { lines?: number }) {
  return (
    <div className="skeleton-text-block">
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton
          key={i}
          width={i === lines - 1 ? '70%' : '100%'}
          height={14}
          borderRadius={3}
        />
      ))}
    </div>
  )
}

export function SkeletonTable({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="skeleton-table">
      <div className="skeleton-table-row skeleton-table-header">
        {Array.from({ length: cols }).map((_, i) => (
          <Skeleton key={i} width="80%" height={16} borderRadius={3} />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="skeleton-table-row">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton key={c} width={`${60 + Math.random() * 30}%`} height={14} borderRadius={3} />
          ))}
        </div>
      ))}
    </div>
  )
}

export default Skeleton
