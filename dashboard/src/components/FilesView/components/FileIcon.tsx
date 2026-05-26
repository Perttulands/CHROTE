import { FileItem } from '../types'
import { getFileIcon } from '../utils'

interface FileIconProps {
  item: FileItem
}

export function FileIcon({ item }: FileIconProps) {
  return (
    <span className={`fb-icon ${item.isDir ? 'fb-icon-folder' : 'fb-icon-file'}`}>
      {getFileIcon(item)}
    </span>
  )
}
