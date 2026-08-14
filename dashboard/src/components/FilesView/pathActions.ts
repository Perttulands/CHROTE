import { normalizeFilePath } from '../FileViewer'

export function joinFilePath(parent: string, name: string): string {
  const cleanParent = normalizeFilePath(parent)
  return cleanParent === '/' ? `/${name}` : `${cleanParent}/${name}`
}

export function getParentPath(path: string): string {
  const normalized = normalizeFilePath(path)
  if (normalized === '/') return '/'
  const parts = normalized.split('/').filter(Boolean)
  parts.pop()
  return parts.length === 0 ? '/' : `/${parts.join('/')}`
}

export function pathRelativeTo(currentPath: string, path: string): string {
  const current = normalizeFilePath(currentPath)
  const target = normalizeFilePath(path)
  if (current === '/') return target.replace(/^\//, '') || '/'
  if (target === current) {
    const parts = current.split('/').filter(Boolean)
    return parts[parts.length - 1] || '/'
  }
  if (target.startsWith(`${current}/`)) return target.slice(current.length + 1)
  return target
}
