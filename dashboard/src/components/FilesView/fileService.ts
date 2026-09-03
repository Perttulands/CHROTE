import { FileItem, FileOperationError, DirectoryResponse, RawFileItem } from './types'

const API_BASE = '/api/files'
const TEXT_FILE_ACCEPT = 'text/plain, text/*, application/json, */*'
export const MAX_TEXT_PREVIEW_BYTES = 1024 * 1024

// ============================================
// FILENAME SANITIZATION (security)
// ============================================

/**
 * Sanitize filename to prevent path traversal and other attacks
 * SECURITY: Rejects dangerous patterns, allows only safe characters
 */
export function sanitizeFilename(filename: string): string {
  // Reject path separators and traversal patterns
  if (filename.includes('/') || filename.includes('\\') || filename.includes('..')) {
    throw new FileOperationError(
      'Invalid filename: path separators and ".." are not allowed',
      'INVALID'
    )
  }

  // Reject null bytes and control characters
  if (/[\x00-\x1f\x7f]/.test(filename)) {
    throw new FileOperationError(
      'Invalid filename: control characters are not allowed',
      'INVALID'
    )
  }

  // Reject empty or whitespace-only names
  const trimmed = filename.trim()
  if (!trimmed) {
    throw new FileOperationError('Invalid filename: name cannot be empty', 'INVALID')
  }

  // Reject names that are only dots
  if (/^\.+$/.test(trimmed)) {
    throw new FileOperationError('Invalid filename: "." and ".." are not allowed', 'INVALID')
  }

  return trimmed
}

// ============================================
// ERROR HANDLING
// ============================================

/**
 * Throws a FileOperationError based on HTTP status code
 * No silent failures - every error surfaces
 */
function throwForStatus(response: Response, context: string): void {
  if (response.ok) return

  switch (response.status) {
    case 403:
      throw new FileOperationError('Permission denied', 'PERMISSION', 403)
    case 404:
      throw new FileOperationError('File not found', 'NOT_FOUND', 404)
    case 409:
      throw new FileOperationError('Already exists', 'CONFLICT', 409)
    case 413:
      throw new FileOperationError('File too large', 'STORAGE', 413)
    case 507:
      throw new FileOperationError('Insufficient storage', 'STORAGE', 507)
    default:
      throw new FileOperationError(`${context}: ${response.status}`, 'SERVER', response.status)
  }
}

/**
 * Get user-friendly error message for operations
 */
export function getErrorMessage(error: unknown, context: string): string {
  if (error instanceof FileOperationError) {
    switch (error.code) {
      case 'PERMISSION':
        return 'Permission denied'
      case 'NOT_FOUND':
        return context === 'rename' ? 'File not found' : 'Path not found'
      case 'CONFLICT':
        return context === 'create' ? 'Folder already exists' : 'Destination already exists'
      case 'STORAGE':
        return error.status === 413 ? 'File too large' : 'Insufficient storage'
      case 'INVALID':
        return error.message
      case 'NETWORK':
        return 'Network error - check your connection'
      case 'SERVER':
        return `Server error (${error.status})`
      default:
        return error.message
    }
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'An unexpected error occurred'
}

// ============================================
// API FUNCTIONS - No fallbacks, throw on any error
// ============================================

/** One path the server's walk matched. */
export interface FileMatch {
  path: string
  name: string
}

/** What a find returned, and whether the roots held more than it will show. */
export interface FindResult {
  matches: FileMatch[]
  truncated: boolean
}

/** A file's unified diff against its git HEAD, as the server read it. */
export interface FileDiffResult {
  path: string
  /** The repository that contains the file, or '' when it is in none. */
  repository: string
  /** The unified diff, or '' when nothing changed and when there is no repository. */
  diff: string
  truncated: boolean
}

/**
 * Find files by name across the configured roots.
 *
 * The server owns the walk, the ignore list and the result bound; the client
 * only carries the query and the caller's abort signal, so a keystroke that has
 * been overtaken stops costing anything.
 */
export async function findFiles(query: string, signal?: AbortSignal): Promise<FindResult> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/find?q=${encodeURIComponent(query)}`, {
      headers: { Accept: 'application/json' },
      signal: signal ?? AbortSignal.timeout(10000),
    })
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') throw error
    throw new FileOperationError(error instanceof Error ? error.message : 'Network error', 'NETWORK')
  }

  throwForStatus(response, 'Failed to find files')
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object') throw new FileOperationError('Malformed find response', 'SERVER')
  const raw = (data as { matches?: unknown }).matches
  const matches = Array.isArray(raw)
    ? raw.flatMap((entry): FileMatch[] => {
        if (!entry || typeof entry !== 'object') return []
        const { path, name } = entry as { path?: unknown; name?: unknown }
        if (typeof path !== 'string' || !path) return []
        return [{ path, name: typeof name === 'string' && name ? name : path.split('/').pop() || path }]
      })
    : []
  return { matches, truncated: (data as { truncated?: unknown }).truncated === true }
}

/**
 * Read a file's unified diff against its git HEAD.
 *
 * A file outside any repository, and a file nobody has changed, both come back
 * with an empty diff: the viewer says so rather than treating either as a
 * failure.
 */
export async function fetchFileDiff(path: string, signal?: AbortSignal): Promise<FileDiffResult> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/diff?path=${encodeURIComponent(path)}`, {
      headers: { Accept: 'application/json' },
      signal: signal ?? AbortSignal.timeout(20000),
    })
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') throw error
    throw new FileOperationError(error instanceof Error ? error.message : 'Network error', 'NETWORK')
  }

  throwForStatus(response, 'Failed to read diff')
  const data: unknown = await response.json()
  if (!data || typeof data !== 'object') throw new FileOperationError('Malformed diff response', 'SERVER')
  const { repository, diff, truncated } = data as { repository?: unknown; diff?: unknown; truncated?: unknown }
  return {
    path,
    repository: typeof repository === 'string' ? repository : '',
    diff: typeof diff === 'string' ? diff : '',
    truncated: truncated === true,
  }
}

/**
 * Fetch directory contents
 * Throws on: non-200 response, non-directory, malformed response, network error
 */
export async function fetchDirectory(path: string): Promise<FileItem[]> {
  const cleanPath = path.startsWith('/') ? path : '/' + path

  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${cleanPath}`, {
      headers: {
        'Accept': 'application/json',
      },
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to fetch directory')

  let data: DirectoryResponse
  try {
    data = await response.json()
  } catch {
    throw new FileOperationError('Invalid server response', 'INVALID')
  }

  // Validate response structure - NO silent fallback to []
  if (typeof data !== 'object' || data === null) {
    throw new FileOperationError('Invalid directory response', 'INVALID')
  }

  if (data.isDir === false) {
    throw new FileOperationError('Path is not a directory', 'INVALID')
  }

  if (!('items' in data) || !Array.isArray(data.items)) {
    throw new FileOperationError('Invalid directory response', 'INVALID')
  }

  return data.items.map((item: RawFileItem) => ({
    name: item.name,
    size: item.size,
    modified: item.modified,
    isDir: item.isDir,
    type: item.type,
    path: cleanPath === '/' ? `/${item.name}` : `${cleanPath}/${item.name}`,
  }))
}

/**
 * Create a new folder
 * Throws on: 403 (permission), 409 (exists), 500+ (server error)
 */
export async function createFolder(path: string, name: string): Promise<void> {
  const safeName = sanitizeFilename(name)
  const fullPath = path === '/' ? `/${safeName}` : `${path}/${safeName}`

  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${fullPath}/`, {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain',
      },
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  if (response.status === 409) {
    throw new FileOperationError('Folder already exists', 'CONFLICT', 409)
  }

  throwForStatus(response, 'Failed to create folder')
}

/**
 * Read a file as text for the editor/preview pane.
 * Throws on all non-OK responses so callers can surface the real failure.
 */
export async function readTextFile(path: string, maxBytes = MAX_TEXT_PREVIEW_BYTES): Promise<string> {
  const response = await fetchRawFile(path)
  const bytes = await readBoundedBytes(response, maxBytes)
  if (bytes === null) throw new FileOperationError('File too large', 'STORAGE', 413)
  return new TextDecoder().decode(bytes)
}

async function fetchRawFile(path: string): Promise<Response> {
  let response: Response
  try {
    response = await fetch(getDownloadUrl(path), {
      headers: {
        'Accept': TEXT_FILE_ACCEPT,
      },
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to read file')
  return response
}

async function readBoundedBytes(response: Response, maxBytes: number): Promise<Uint8Array | null> {
  const contentLength = Number(response.headers.get('Content-Length'))
  if (Number.isFinite(contentLength) && contentLength > maxBytes) {
    await response.body?.cancel()
    return null
  }
  if (!response.body) return new Uint8Array()

  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let total = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (!value) continue
      if (total + value.byteLength > maxBytes) {
        await reader.cancel()
        return null
      }
      chunks.push(value)
      total += value.byteLength
    }
  } finally {
    reader.releaseLock()
  }

  const bytes = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    bytes.set(chunk, offset)
    offset += chunk.byteLength
  }
  return bytes
}

/**
 * Probe an otherwise unknown file without turning binary data into mojibake or
 * buffering beyond the preview limit, even when listing metadata is stale.
 */
export async function probeTextFile(path: string, maxBytes = MAX_TEXT_PREVIEW_BYTES): Promise<string | null> {
  const response = await fetchRawFile(path)
  const bytes = await readBoundedBytes(response, maxBytes)
  if (bytes === null) return null
  if (bytes.includes(0)) return null

  let text: string
  try {
    text = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return null
  }

  for (const character of text) {
    const codePoint = character.codePointAt(0) || 0
    if (codePoint < 32 && codePoint !== 9 && codePoint !== 10 && codePoint !== 13) return null
  }
  return text
}

/**
 * Write text content to a file. The backend intentionally uses POST for both
 * file creation and overwrite.
 */
export async function writeTextFile(path: string, content: string): Promise<void> {
  const cleanPath = path.startsWith('/') ? path : '/' + path

  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${cleanPath}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'text/plain; charset=utf-8',
      },
      body: content,
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to write file')
}

/**
 * Create an empty file in a directory.
 */
export async function createFile(path: string, name: string): Promise<string> {
  const safeName = sanitizeFilename(name)
  const fullPath = path === '/' ? `/${safeName}` : `${path}/${safeName}`
  await writeTextFile(fullPath, '')
  return fullPath
}

/**
 * Rename/move a file or folder
 * Throws on: 404 (not found), 409 (destination exists), 403 (permission)
 */
export async function renameItem(oldPath: string, newPath: string): Promise<void> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${oldPath}`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        action: 'rename',
        destination: newPath,
      }),
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  if (response.status === 409) {
    throw new FileOperationError('Destination already exists', 'CONFLICT', 409)
  }

  throwForStatus(response, 'Failed to rename')
}

/**
 * Delete a file or folder
 * Throws on: 403 (permission), 404 (not found)
 */
export async function deleteItem(path: string): Promise<void> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}/resources${path}`, {
      method: 'DELETE',
      signal: AbortSignal.timeout(10000),
    })
  } catch (error) {
    throw new FileOperationError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK'
    )
  }

  throwForStatus(response, 'Failed to delete')
}

/**
 * Upload files to a directory
 * Throws on: 413 (too large), 507 (storage), 403 (permission), invalid filename
 * SECURITY: Sanitizes filenames to prevent path traversal
 */
export async function uploadFiles(path: string, files: FileList | File[]): Promise<void> {
  const fileArray = Array.isArray(files) ? files : Array.from(files)

  for (const file of fileArray) {
    // SECURITY: Sanitize filename before building path
    const safeName = sanitizeFilename(file.name)
    const fullPath = path === '/' ? `/${safeName}` : `${path}/${safeName}`

    let response: Response
    try {
      response = await fetch(`${API_BASE}/resources${fullPath}`, {
        method: 'POST',
        headers: {
          'Content-Type': file.type || 'application/octet-stream',
        },
        body: file,
        signal: AbortSignal.timeout(30000),
      })
    } catch (error) {
      throw new FileOperationError(
        error instanceof Error ? error.message : 'Network error',
        'NETWORK'
      )
    }

    if (response.status === 413) {
      throw new FileOperationError('File too large', 'STORAGE', 413)
    }

    if (response.status === 507) {
      throw new FileOperationError('Insufficient storage', 'STORAGE', 507)
    }

    throwForStatus(response, `Failed to upload ${safeName}`)
  }
}

/**
 * Check if a path exists (used by InboxPanel)
 * Returns true/false, does not throw on 404
 */
export async function pathExists(path: string): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE}/resources${path}`, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
      },
      signal: AbortSignal.timeout(10000),
    })
    return response.ok
  } catch {
    return false
  }
}

/**
 * Get download URL for a file
 */
export function getDownloadUrl(path: string): string {
  return `${API_BASE}/raw${path}?inline=false`
}

/**
 * Represents a file with its relative path within a folder
 */
export interface FileWithPath {
  file: File
  relativePath: string
}

/**
 * Upload files preserving directory structure
 * Used for folder uploads where files have relative paths
 * SECURITY: Sanitizes each path component to prevent path traversal
 */
export async function uploadFilesWithPaths(basePath: string, filesWithPaths: FileWithPath[]): Promise<void> {
  for (const { file, relativePath } of filesWithPaths) {
    // SECURITY: Sanitize each path component
    const pathParts = relativePath.split('/').filter(p => p.length > 0)
    const sanitizedParts = pathParts.map(part => sanitizeFilename(part))
    const safePath = sanitizedParts.join('/')

    const fullPath = basePath === '/' ? `/${safePath}` : `${basePath}/${safePath}`

    let response: Response
    try {
      response = await fetch(`${API_BASE}/resources${fullPath}`, {
        method: 'POST',
        headers: {
          'Content-Type': file.type || 'application/octet-stream',
        },
        body: file,
        signal: AbortSignal.timeout(30000),
      })
    } catch (error) {
      throw new FileOperationError(
        error instanceof Error ? error.message : 'Network error',
        'NETWORK'
      )
    }

    if (response.status === 413) {
      throw new FileOperationError('File too large', 'STORAGE', 413)
    }

    if (response.status === 507) {
      throw new FileOperationError('Insufficient storage', 'STORAGE', 507)
    }

    throwForStatus(response, `Failed to upload ${safePath}`)
  }
}
