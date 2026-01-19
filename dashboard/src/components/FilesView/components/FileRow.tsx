import { useRef, useEffect } from 'react'
import { FileItem, getRootDisplayName } from '../types'
import { getDownloadUrl } from '../fileService'
import { formatSize, formatDate } from '../utils'
import { FileIcon } from './FileIcon'

interface FileRowProps {
  item: FileItem
  isSelected: boolean
  onSelect: (e: React.MouseEvent) => void
  onNavigate: () => void
  onContextMenu: (e: React.MouseEvent) => void
  onRename: () => void
  isRenaming: boolean
  renameValue: string
  setRenameValue: (value: string) => void
  onRenameSubmit: () => void
  onRenameCancel: () => void
  isAtRoot: boolean
  disabled?: boolean
}

export function FileRow({
  item,
  isSelected,
  onSelect,
  onNavigate,
  onContextMenu,
  onRename,
  isRenaming,
  renameValue,
  setRenameValue,
  onRenameSubmit,
  onRenameCancel,
  isAtRoot,
  disabled,
}: FileRowProps) {
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isRenaming && inputRef.current) {
      inputRef.current.focus()
      const ext = item.name.includes('.') ? item.name.lastIndexOf('.') : item.name.length
      inputRef.current.setSelectionRange(0, ext)
    }
  }, [isRenaming, item.name])

  const handleDoubleClick = () => {
    if (disabled) return
    if (item.isDir) {
      onNavigate()
    } else {
      window.open(getDownloadUrl(item.path), '_blank')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return
    if (e.key === 'Enter' && !isRenaming) {
      handleDoubleClick()
    } else if (e.key === 'F2' && !isRenaming) {
      onRename()
    }
  }

  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      onRenameSubmit()
    } else if (e.key === 'Escape') {
      onRenameCancel()
    }
  }

  // At root level, show paths like /code, /vault
  const displayName = isAtRoot ? getRootDisplayName(item.name) : item.name

  return (
    <div
      className={`fb-row ${isSelected ? 'selected' : ''} ${disabled ? 'disabled' : ''}`}
      onClick={disabled ? undefined : onSelect}
      onDoubleClick={handleDoubleClick}
      onContextMenu={disabled ? undefined : onContextMenu}
      onKeyDown={handleKeyDown}
      tabIndex={disabled ? -1 : 0}
      role="row"
    >
      <div className="fb-cell fb-cell-name">
        <FileIcon item={item} />
        {isRenaming ? (
          <input
            ref={inputRef}
            type="text"
            className="fb-rename-input"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            onKeyDown={handleRenameKeyDown}
            onBlur={onRenameCancel}
          />
        ) : (
          <span className="fb-filename">{displayName}</span>
        )}
      </div>
      <div className="fb-cell fb-cell-size">
        {item.isDir ? '-' : formatSize(item.size)}
      </div>
      <div className="fb-cell fb-cell-modified">
        {formatDate(item.modified)}
      </div>
    </div>
  )
}
