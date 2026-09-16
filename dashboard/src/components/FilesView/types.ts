// ============================================
// FILE OPERATION ERROR
// ============================================

export type FileErrorCode =
  | 'NETWORK'
  | 'NOT_FOUND'
  | 'PERMISSION'
  | 'CONFLICT'
  | 'INVALID'
  | 'SERVER'
  | 'STORAGE'

export class FileOperationError extends Error {
  constructor(
    message: string,
    public code: FileErrorCode,
    public status?: number
  ) {
    super(message)
    this.name = 'FileOperationError'
  }
}

// ============================================
// FILE ITEM TYPES
// ============================================

export interface FileItem {
  name: string
  size: number
  modified: string
  isDir: boolean
  type: string
  path: string
}

export interface RawFileItem {
  name: string
  size: number
  modified: string
  isDir: boolean
  type: string
}

export interface DirectoryResponse {
  isDir: boolean
  items?: RawFileItem[]
  name?: string
}

// ============================================
// CONTEXT MENU TYPES
// ============================================

export interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

export type SortKey = 'name' | 'size' | 'modified'
export type SortDir = 'asc' | 'desc'
export type ViewMode = 'list' | 'grid'
export type CreateKind = 'file' | 'folder'

export interface FilesViewProps {
  navigateRequest?: { path: string; nonce: number } | null
  onSendPath?: (path: string) => void
  sendTargetLabel?: string | null
}

export interface TabContextMenuState {
  x: number
  y: number
  path: string
}

export interface CreateIntent {
  kind: CreateKind
  parentPath: string
  name: string
}
