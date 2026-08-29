import FilesViewContent from './FilesViewContent'
import { useFilesView } from './useFilesView'
import type { FilesViewProps } from './types'

function FilesView(props: FilesViewProps) {
  return <FilesViewContent {...useFilesView(props)} />
}

export default FilesView
import './FilesView.css'
