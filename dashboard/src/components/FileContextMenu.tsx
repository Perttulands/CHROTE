import type { FileItem } from './FilesView/types'
import Menu, { type MenuAction, type MenuGroup } from './Menu'

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
  const selectedPaths: MenuAction[] = onCopySelectedPaths
    ? [{ id: 'copy-selected', label: 'Copy selected paths', onSelect: onCopySelectedPaths }]
    : []

  const groups: MenuGroup[] = item === null
    ? [
      {
        id: 'make',
        rows: [
          { id: 'new-file', label: 'New file', onSelect: onNewFile },
          { id: 'new-folder', label: 'New folder', onSelect: onNewFolder },
          { id: 'upload', label: 'Upload', onSelect: onUpload },
          { id: 'refresh', label: 'Refresh', onSelect: onRefresh },
        ],
      },
      {
        id: 'folder',
        rows: [
          { id: 'copy-current', label: 'Copy current folder path', onSelect: onCopyCurrentPath },
          ...selectedPaths,
          {
            id: 'pin-current',
            label: currentPathPinned ? 'Unpin current folder' : 'Pin current folder',
            onSelect: onToggleCurrentPathPin,
          },
        ],
      },
    ]
    : [
      {
        id: 'open',
        rows: item.isDir
          ? [{ id: 'open-folder', label: 'Open folder', onSelect: () => onOpen(item) }]
          : [
            { id: 'open', label: 'Open', onSelect: () => onOpen(item) },
            { id: 'download', label: 'Download', onSelect: () => onDownload(item) },
          ],
      },
      {
        id: 'item',
        rows: [
          { id: 'rename', label: 'Rename', onSelect: () => onRename(item) },
          { id: 'pin', label: itemPinned ? 'Unpin' : 'Pin', onSelect: () => onTogglePin(item) },
          { id: 'copy-path', label: 'Copy path', onSelect: () => onCopyPath(item.path) },
          ...selectedPaths,
          { id: 'copy-relative', label: 'Copy relative path', onSelect: () => onCopyRelativePath(item.path) },
          { id: 'open-parent', label: 'Open parent folder', onSelect: () => onOpenParent(item.path) },
        ],
      },
      {
        id: 'end',
        rows: [
          {
            id: 'delete',
            label: 'Delete',
            danger: true,
            onSelect: () => onDelete(item),
          },
        ],
      },
    ]

  return (
    <Menu
      at={{ x, y }}
      label={item ? `Actions for ${item.name}` : 'Folder actions'}
      zIndex={2200}
      estimatedSize={{ width: 220, height: 300 }}
      onClose={onClose}
      groups={groups}
    />
  )
}
