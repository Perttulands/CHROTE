import { FileItem } from '../types'
import { useViewportMenuPosition } from '../../../hooks/useViewportMenuPosition'
import DismissiblePanel from '../../DismissiblePanel'

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
  const menuPosition = useViewportMenuPosition<HTMLDivElement>(
    { x, y },
    { estimatedSize: { width: 200, height: 180 } },
  )

  return (
    <DismissiblePanel onDismiss={onClose} panelZIndex={2200} panelPosition="fixed">
      <div ref={menuPosition.ref} className="fb-context-menu" style={menuPosition.style}>
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
    </DismissiblePanel>
  )
}
