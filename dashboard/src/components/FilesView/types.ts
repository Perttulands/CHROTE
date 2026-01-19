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
// STATE TYPES
// ============================================

export interface FileBrowserState {
  items: FileItem[]
  loading: boolean
  error: string | null
  currentPath: string
  selectedItems: Set<string>
  sortBy: 'name' | 'size' | 'modified'
  sortDir: 'asc' | 'desc'
  viewMode: 'list' | 'grid'
  searchQuery: string
}

export type OperationType = 'rename' | 'delete' | 'create' | 'upload' | null

export interface OperationState {
  type: OperationType
  loading: boolean
  error: string | null
  target?: string
}

export type ViewTab = 'browser' | 'info'

// ============================================
// CONTEXT MENU TYPES
// ============================================

export interface ContextMenuState {
  x: number
  y: number
  item: FileItem | null
}

// ============================================
// PATH UTILITIES
// ============================================

/**
 * Convert container path to display path
 * In WSL mode, paths are shown as-is (no Windows mapping needed)
 */
export function toDisplayPath(containerPath: string): string {
  return containerPath
}

/**
 * Convert display path to container path
 * In WSL mode, paths are the same (no Windows mapping needed)
 */
export function toContainerPath(displayPath: string): string {
  return displayPath
}

/**
 * Get display name for root items
 * Shows the actual path name (e.g., 'code', 'vault')
 */
export function getRootDisplayName(name: string): string {
  return `/${name}`
}
