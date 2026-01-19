// Folder picker modal for selecting beads project paths
// Reuses file browser API and styling patterns

import { useState, useEffect, useCallback } from 'react'
import { fetchDirectory } from './FilesView/fileService'
import type { FileItem } from './FilesView/types'
import { toDisplayPath } from './FilesView/types'

interface FolderPickerModalProps {
  onSelect: (path: string) => void
  onClose: () => void
  initialPath?: string
}

interface FolderItem extends FileItem {
  hasBeads?: boolean
}

export default function FolderPickerModal({ onSelect, onClose, initialPath = '/' }: FolderPickerModalProps) {
  const [currentPath, setCurrentPath] = useState(initialPath)
  const [items, setItems] = useState<FolderItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [currentHasBeads, setCurrentHasBeads] = useState(false)

  // Check if a folder has .beads subdirectory
  const checkHasBeads = useCallback(async (folderPath: string): Promise<boolean> => {
    try {
      const beadsPath = folderPath === '/' ? '/.beads' : `${folderPath}/.beads`
      const response = await fetch(`/api/files/resources${beadsPath}`)
      if (!response.ok) return false
      const data = await response.json()
      return data.isDir === true
    } catch {
      return false
    }
  }, [])

  // Load directory and check for .beads in each subfolder
  const loadDirectory = useCallback(async (path: string) => {
    setLoading(true)
    setError(null)

    try {
      const allItems = await fetchDirectory(path)

      // Filter to directories only
      const dirs = allItems.filter(item => item.isDir)

      // Check if current folder has .beads
      const hasBeads = await checkHasBeads(path)
      setCurrentHasBeads(hasBeads)

      // Check each subfolder for .beads (in parallel)
      const withBeads = await Promise.all(
        dirs.map(async (dir): Promise<FolderItem> => ({
          ...dir,
          hasBeads: await checkHasBeads(dir.path)
        }))
      )

      setItems(withBeads)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load directory')
      setItems([])
    } finally {
      setLoading(false)
    }
  }, [checkHasBeads])

  // Load directory on mount and when path changes
  useEffect(() => {
    loadDirectory(currentPath)
  }, [currentPath, loadDirectory])

  // Navigate to a folder
  const navigateTo = (path: string) => {
    setCurrentPath(path)
  }

  // Navigate up one level
  const navigateUp = () => {
    if (currentPath === '/') return
    const parts = currentPath.split('/').filter(Boolean)
    parts.pop()
    setCurrentPath(parts.length === 0 ? '/' : '/' + parts.join('/'))
  }

  // Build breadcrumb segments
  const breadcrumbs = currentPath === '/'
    ? [{ name: 'Root', path: '/' }]
    : [
        { name: 'Root', path: '/' },
        ...currentPath.split('/').filter(Boolean).map((segment, index, arr) => ({
          name: segment,
          path: '/' + arr.slice(0, index + 1).join('/')
        }))
      ]

  // Handle select button
  const handleSelect = () => {
    onSelect(currentPath)
  }

  // Handle clicking outside modal
  const handleOverlayClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose()
    }
  }

  return (
    <div className="fb-dialog-overlay" onClick={handleOverlayClick}>
      <div className="fb-dialog folder-picker-dialog" onClick={e => e.stopPropagation()}>
        <div className="fb-dialog-header">
          <h3>Select Beads Project Folder</h3>
          <button className="fb-dialog-close" onClick={onClose}>&times;</button>
        </div>

        <div className="fb-dialog-body folder-picker-body">
          {/* Breadcrumb navigation */}
          <div className="folder-picker-nav">
            <button
              className="folder-picker-up-btn"
              onClick={navigateUp}
              disabled={currentPath === '/'}
              title="Go up"
            >
              &#x2191;
            </button>
            <div className="fb-breadcrumbs">
              {breadcrumbs.map((crumb, i) => (
                <span key={crumb.path}>
                  {i > 0 && <span className="fb-breadcrumb-sep">/</span>}
                  <button
                    className="fb-breadcrumb-item"
                    onClick={() => navigateTo(crumb.path)}
                  >
                    {crumb.name}
                  </button>
                </span>
              ))}
            </div>
          </div>

          {/* Current path display */}
          <div className="folder-picker-path">
            <span className="folder-picker-path-label">Path:</span>
            <code>{toDisplayPath(currentPath)}</code>
            {currentHasBeads && <span className="beads-badge">.beads</span>}
          </div>

          {/* Directory listing */}
          <div className="folder-picker-list">
            {loading ? (
              <div className="folder-picker-loading">Loading...</div>
            ) : error ? (
              <div className="folder-picker-error">{error}</div>
            ) : items.length === 0 ? (
              <div className="folder-picker-empty">No subfolders</div>
            ) : (
              items.map(item => (
                <div
                  key={item.path}
                  className={`folder-picker-item ${item.hasBeads ? 'has-beads' : ''}`}
                  onClick={() => navigateTo(item.path)}
                >
                  <span className="folder-picker-icon">&#128193;</span>
                  <span className="folder-picker-name">{item.name}</span>
                  {item.hasBeads && <span className="beads-badge">.beads</span>}
                </div>
              ))
            )}
          </div>
        </div>

        <div className="fb-dialog-footer">
          <button className="fb-dialog-btn fb-dialog-btn-cancel" onClick={onClose}>
            Cancel
          </button>
          <button
            className="fb-dialog-btn fb-dialog-btn-primary"
            onClick={handleSelect}
            disabled={!currentHasBeads}
            title={currentHasBeads ? 'Select this folder' : 'Folder must contain .beads directory'}
          >
            Select This Folder
          </button>
        </div>
      </div>
    </div>
  )
}
