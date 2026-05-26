import { FileItem, getRootDisplayName } from '../types'
import { getDownloadUrl } from '../fileService'
import { getFileIcon } from '../utils'

interface FileGridItemProps {
  item: FileItem
  isSelected: boolean
  onSelect: (e: React.MouseEvent) => void
  onNavigate: () => void
  onContextMenu: (e: React.MouseEvent) => void
  isAtRoot: boolean
  disabled?: boolean
}

export function FileGridItem({
  item,
  isSelected,
  onSelect,
  onNavigate,
  onContextMenu,
  isAtRoot,
  disabled,
}: FileGridItemProps) {
  const handleDoubleClick = () => {
    if (disabled) return
    if (item.isDir) {
      onNavigate()
    } else {
      window.open(getDownloadUrl(item.path), '_blank')
    }
  }

  const displayName = isAtRoot ? getRootDisplayName(item.name) : item.name

  return (
    <div
      className={`fb-grid-item ${isSelected ? 'selected' : ''} ${disabled ? 'disabled' : ''}`}
      onClick={disabled ? undefined : onSelect}
      onDoubleClick={handleDoubleClick}
      onContextMenu={disabled ? undefined : onContextMenu}
      tabIndex={disabled ? -1 : 0}
      role="gridcell"
    >
      <div className="fb-grid-icon">
        {getFileIcon(item)}
      </div>
      <div className="fb-grid-name" title={displayName}>
        {displayName}
      </div>
    </div>
  )
}
