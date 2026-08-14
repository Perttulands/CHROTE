import type { FileItem } from './FilesView/types'
import { useViewportMenuPosition } from '../hooks/useViewportMenuPosition'
import DismissiblePanel from './DismissiblePanel'

interface FileContextMenuProps {
  x: number
  y: number
  item: FileItem | null
  itemPinned: boolean
  currentPathPinned: boolean
  onClose: () => void
  onOpen: (item: FileItem) => void
  onDownload: (item: FileItem) => void
  onRename: (item: FileItem) => void
  onTogglePin: (item: FileItem) => void
  onCopyPath: (path: string) => void
  onCopySelectedPaths?: () => void
  onCopyRelativePath: (path: string) => void
  onOpenParent: (path: string) => void
  onDelete: (item: FileItem) => void
  onNewFile: () => void
  onNewFolder: () => void
  onUpload: () => void
  onRefresh: () => void
  onCopyCurrentPath: () => void
  onToggleCurrentPathPin: () => void
}

export function FileContextMenu({
  x,
  y,
  item,
  itemPinned,
  currentPathPinned,
  onClose,
  onOpen,
  onDownload,
  onRename,
  onTogglePin,
  onCopyPath,
  onCopySelectedPaths,
  onCopyRelativePath,
  onOpenParent,
  onDelete,
  onNewFile,
  onNewFolder,
  onUpload,
  onRefresh,
  onCopyCurrentPath,
  onToggleCurrentPathPin,
}: FileContextMenuProps) {
  const menuPosition = useViewportMenuPosition<HTMLDivElement>(
    { x, y },
    { estimatedSize: { width: 220, height: 320 } },
  )

  return (
    <DismissiblePanel onDismiss={onClose} panelZIndex={2200} panelPosition="fixed">
      <div ref={menuPosition.ref} className="fb-context-menu" style={menuPosition.style}>
        {item?.isDir && (
          <button className="fb-context-item" type="button" onClick={() => onOpen(item)}>Open Folder</button>
        )}
        {item && !item.isDir && (
          <>
            <button className="fb-context-item" type="button" onClick={() => onOpen(item)}>Open</button>
            <button className="fb-context-item" type="button" onClick={() => onDownload(item)}>Download</button>
          </>
        )}
        {!item && (
          <>
            <button className="fb-context-item" type="button" onClick={onNewFile}>New File</button>
            <button className="fb-context-item" type="button" onClick={onNewFolder}>New Folder</button>
            <button className="fb-context-item" type="button" onClick={onUpload}>Upload</button>
            <button className="fb-context-item" type="button" onClick={onRefresh}>Refresh</button>
            <div className="fb-context-divider" />
            <button className="fb-context-item" type="button" onClick={onCopyCurrentPath}>Copy Current Folder Path</button>
            {onCopySelectedPaths && (
              <button className="fb-context-item" type="button" onClick={onCopySelectedPaths}>Copy Selected Path(s)</button>
            )}
            <button className="fb-context-item" type="button" onClick={onToggleCurrentPathPin}>
              {currentPathPinned ? 'Unpin Current Folder' : 'Pin Current Folder'}
            </button>
          </>
        )}
        {item && (
          <>
            <div className="fb-context-divider" />
            <button className="fb-context-item" type="button" onClick={() => onRename(item)}>Rename</button>
            <button className="fb-context-item" type="button" onClick={() => onTogglePin(item)}>{itemPinned ? 'Unpin' : 'Pin'}</button>
            <button className="fb-context-item" type="button" onClick={() => onCopyPath(item.path)}>Copy Path</button>
            {onCopySelectedPaths && (
              <button className="fb-context-item" type="button" onClick={onCopySelectedPaths}>Copy Selected Path(s)</button>
            )}
            <button className="fb-context-item" type="button" onClick={() => onCopyRelativePath(item.path)}>Copy Relative Path</button>
            <button className="fb-context-item" type="button" onClick={() => onOpenParent(item.path)}>Open Parent Folder</button>
            <div className="fb-context-divider" />
            <button className="fb-context-item fb-context-danger" type="button" onClick={() => onDelete(item)}>Delete</button>
          </>
        )}
      </div>
    </DismissiblePanel>
  )
}
