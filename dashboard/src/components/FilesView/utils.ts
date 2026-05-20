import { FileItem } from './types'

export function formatSize(bytes: number): string {
  if (bytes === 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays === 0) {
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
    if (diffHours === 0) {
      const diffMins = Math.floor(diffMs / (1000 * 60))
      return diffMins <= 1 ? 'Just now' : `${diffMins} min ago`
    }
    return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`
  }

  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays} days ago`

  return date.toLocaleDateString()
}

export function getFileIcon(item: FileItem): string {
  if (item.isDir) return '📁'

  const ext = item.name.split('.').pop()?.toLowerCase() || ''
  const iconMap: Record<string, string> = {
    // Code
    js: '📜', ts: '📜', jsx: '📜', tsx: '📜',
    py: '🐍', rb: '💎', go: '🔵', rs: '🦀',
    java: '☕', c: '⚙️', cpp: '⚙️', h: '⚙️',
    cs: '🔷', php: '🐘', swift: '🍎',
    // Web
    html: '🌐', css: '🎨', scss: '🎨', less: '🎨',
    // Data
    json: '📋', yaml: '📋', yml: '📋', xml: '📋',
    csv: '📊', sql: '🗄️',
    // Documents
    md: '📝', txt: '📄', pdf: '📕', doc: '📘', docx: '📘',
    xls: '📗', xlsx: '📗', ppt: '📙', pptx: '📙',
    // Media
    png: '🖼️', jpg: '🖼️', jpeg: '🖼️', gif: '🖼️', svg: '🖼️', webp: '🖼️',
    mp3: '🎵', wav: '🎵', flac: '🎵', ogg: '🎵',
    mp4: '🎬', mkv: '🎬', avi: '🎬', mov: '🎬', webm: '🎬',
    // Archives
    zip: '📦', tar: '📦', gz: '📦', rar: '📦', '7z': '📦',
    // Config
    env: '🔐', gitignore: '🚫', dockerfile: '🐳',
    // Shell
    sh: '💻', bash: '💻', zsh: '💻', fish: '💻',
  }

  return iconMap[ext] || '📄'
}
