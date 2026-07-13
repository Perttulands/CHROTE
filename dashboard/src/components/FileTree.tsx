import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent, MouseEvent } from 'react'
import { fetchDirectory } from './FilesView/fileService'
import type { FileItem } from './FilesView/types'

interface FileTreeProps {
  currentPath: string
  rootPath?: string
  selectedPath: string | null
  expandedPaths: string[]
  scrollTop: number
  refreshToken?: number
  className?: string
  onOpenDirectory: (path: string) => void
  onOpenFile: (item: FileItem) => void
  onExpandedPathsChange: (paths: string[]) => void
  onScrollTopChange: (scrollTop: number) => void
  onItemContextMenu?: (event: MouseEvent<HTMLElement>, item: FileItem) => void
}

function sortTreeItems(items: FileItem[]): FileItem[] {
  return [...items].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

function fileBadge(item: FileItem): string {
  if (item.isDir) return 'DIR'
  const extension = item.name.toLowerCase().split('.').pop()
  return extension && extension !== item.name.toLowerCase() ? extension.slice(0, 4).toUpperCase() : 'TXT'
}

function ancestorPaths(path: string): string[] {
  const parts = path.split('/').filter(Boolean)
  const paths = ['/']
  for (let index = 0; index < parts.length - 1; index += 1) {
    paths.push(`/${parts.slice(0, index + 1).join('/')}`)
  }
  return paths
}

function FileTree({
  currentPath,
  rootPath = '/',
  selectedPath,
  expandedPaths,
  scrollTop,
  refreshToken = 0,
  className = '',
  onOpenDirectory,
  onOpenFile,
  onExpandedPathsChange,
  onScrollTopChange,
  onItemContextMenu,
}: FileTreeProps) {
  const treeRef = useRef<HTMLDivElement>(null)
  const loadingRef = useRef(new Set<string>())
  const [treeItems, setTreeItems] = useState<Record<string, FileItem[]>>({})
  const [treeLoading, setTreeLoading] = useState<Set<string>>(new Set())
  const [localExpanded, setLocalExpanded] = useState(() => new Set(expandedPaths.length > 0 ? expandedPaths : ['/']))

  useEffect(() => {
    const next = new Set(expandedPaths.length > 0 ? expandedPaths : [rootPath])
    next.add(rootPath)
    setLocalExpanded(next)
  }, [expandedPaths.join('\u0000'), rootPath])

  const loadPath = useCallback(async (path: string, force = false) => {
    if (loadingRef.current.has(path)) return
    if (!force && treeItems[path]) return
    loadingRef.current.add(path)
    setTreeLoading(prev => new Set(prev).add(path))
    try {
      const items = await fetchDirectory(path)
      setTreeItems(prev => ({ ...prev, [path]: sortTreeItems(items) }))
    } catch {
      setTreeItems(prev => (path in prev ? prev : { ...prev, [path]: [] }))
    } finally {
      loadingRef.current.delete(path)
      setTreeLoading(prev => {
        const next = new Set(prev)
        next.delete(path)
        return next
      })
    }
  }, [treeItems])

  useEffect(() => {
    const paths = new Set([rootPath, ...localExpanded])
    paths.forEach(path => void loadPath(path, refreshToken > 0))
  // refreshToken is the explicit invalidation boundary; expanded paths are read from this render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshToken, rootPath])

  useEffect(() => {
    const required = new Set([rootPath, ...localExpanded, ...ancestorPaths(selectedPath || currentPath)])
    required.forEach(path => void loadPath(path))
  }, [currentPath, selectedPath, localExpanded, loadPath, rootPath])

  useLayoutEffect(() => {
    if (treeRef.current && treeRef.current.scrollTop !== scrollTop) {
      treeRef.current.scrollTop = scrollTop
    }
  }, [scrollTop])

  const updateExpanded = useCallback((path: string, expanded: boolean) => {
    setLocalExpanded(prev => {
      const next = new Set(prev)
      if (expanded) next.add(path)
      else next.delete(path)
      next.add(rootPath)
      onExpandedPathsChange(Array.from(next))
      return next
    })
    if (expanded) void loadPath(path)
  }, [loadPath, onExpandedPathsChange, rootPath])

  const openItem = useCallback((item: FileItem) => {
    if (item.isDir) {
      onOpenDirectory(item.path)
      if (!localExpanded.has(item.path)) updateExpanded(item.path, true)
    } else {
      onOpenFile(item)
    }
  }, [localExpanded, onOpenDirectory, onOpenFile, updateExpanded])

  const handleItemKey = useCallback((event: KeyboardEvent<HTMLElement>, item: FileItem) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openItem(item)
    } else if (item.isDir && event.key === 'ArrowRight') {
      event.preventDefault()
      updateExpanded(item.path, true)
    } else if (item.isDir && event.key === 'ArrowLeft') {
      event.preventDefault()
      updateExpanded(item.path, false)
    }
  }, [openItem, updateExpanded])

  const renderNode = (item: FileItem, level: number) => {
    const expanded = item.isDir && localExpanded.has(item.path)
    const children = treeItems[item.path] || []
    const active = selectedPath === item.path || (!selectedPath && currentPath === item.path)
    const label = `${item.isDir ? 'Folder' : 'File'} ${item.name}`

    return (
      <div className="fb-tree-node" key={item.path}>
        <div
          className={`fb-tree-row ${active ? 'active' : ''}`}
          style={{ paddingLeft: `${8 + level * 14}px` }}
          role="treeitem"
          aria-label={label}
          aria-selected={active}
          aria-expanded={item.isDir ? expanded : undefined}
          tabIndex={0}
          onClick={() => openItem(item)}
          onKeyDown={event => handleItemKey(event, item)}
          onContextMenu={event => onItemContextMenu?.(event, item)}
        >
          {item.isDir ? (
            <button
              className="fb-tree-toggle"
              type="button"
              aria-label={`${expanded ? 'Collapse' : 'Expand'} ${item.path}`}
              onClick={event => {
                event.stopPropagation()
                updateExpanded(item.path, !expanded)
              }}
            >
              {expanded ? 'v' : '>'}
            </button>
          ) : (
            <span className="fb-tree-toggle fb-tree-toggle-spacer" aria-hidden="true" />
          )}
          <span className={`fb-tree-glyph ${item.isDir ? 'folder' : 'file'}`}>{fileBadge(item)}</span>
          <span className="fb-tree-name" title={item.path}>{item.name}</span>
        </div>
        {expanded && (
          <div className="fb-tree-children" role="group">
            {treeLoading.has(item.path) && !treeItems[item.path] ? (
              <div className="fb-tree-loading" style={{ paddingLeft: `${28 + level * 14}px` }}>Loading...</div>
            ) : children.map(child => renderNode(child, level + 1))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div
      ref={treeRef}
      className={`fb-tree shared-file-tree ${className}`.trim()}
      role="tree"
      aria-label="File tree"
      onScroll={event => onScrollTopChange(event.currentTarget.scrollTop)}
    >
      {treeLoading.has(rootPath) && !treeItems[rootPath] ? (
        <div className="fb-tree-loading">Loading...</div>
      ) : (treeItems[rootPath] || []).map(item => renderNode(item, 0))}
    </div>
  )
}

export default FileTree
