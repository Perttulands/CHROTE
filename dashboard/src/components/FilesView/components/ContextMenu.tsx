import { useRef, useEffect } from 'react'
import { FileItem } from '../types'

interface ContextMenuProps {
  x: number
  y: number
  item: FileItem | null
  onClose: () => void
  onDownload: () => void
  onRename: () => void
  onDelete: () => void
  onCopyPath: () => void
  onNewFolder: () => void
}

export function ContextMenu({
  x,
  y,
  item,
  onClose,
  onDownload,
  onRename,
  onDelete,
  onCopyPath,
  onNewFolder,
}: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose()
      }
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [onClose])

  const style: React.CSSProperties = {
    position: 'fixed',
    top: y,
    left: x,
    zIndex: 1000,
  }

  return (
    <div ref={menuRef} className="fb-context-menu" style={style}>
      {item && !item.isDir && (
        <button className="fb-context-item" onClick={onDownload}>
          <span className="fb-context-icon">⬇</span>
          Download
        </button>
      )}
      {item && (
        <>
          <button className="fb-context-item" onClick={onRename}>
            <span className="fb-context-icon">✏</span>
            Rename
          </button>
          <button className="fb-context-item" onClick={onCopyPath}>
            <span className="fb-context-icon">📋</span>
            Copy Path
          </button>
          <div className="fb-context-divider" />
          <button className="fb-context-item fb-context-danger" onClick={onDelete}>
            <span className="fb-context-icon">🗑</span>
            Delete
          </button>
        </>
      )}
      {!item && (
        <button className="fb-context-item" onClick={onNewFolder}>
          <span className="fb-context-icon">📁</span>
          New Folder
        </button>
      )}
    </div>
  )
}
