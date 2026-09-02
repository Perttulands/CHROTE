/* A window with nothing in it is the launcher, standing on the slot's own
   illustration. The picture says which slot this is at a glance; the launcher
   says the only thing there is to do here. */

import Launcher from './Launcher'
import { useTheme } from '../theme/ThemeContext'
import type { WorkspaceId } from '../types'
import './EmptyWindow.css'

interface EmptyWindowProps {
  workspaceId: WorkspaceId
  windowId: string
  /** The window's slot in the workspace, which picks its illustration. */
  colorIndex: number
}

export default function EmptyWindow({ workspaceId, windowId, colorIndex }: EmptyWindowProps) {
  const theme = useTheme()
  // A theme with no art is not a theme with a missing picture: the empty
  // window is simply the launcher on the terminal background.
  const art = theme.art.length > 0 ? theme.art[Math.abs(colorIndex) % theme.art.length] : null

  return (
    <div className="empty-window-state">
      {art && (
        <div
          className="empty-window-art"
          data-art={art}
          aria-hidden="true"
          style={{ backgroundImage: `url(/api/theme/art/${encodeURIComponent(art)})` }}
        />
      )}
      <Launcher workspaceId={workspaceId} attachTo={{ workspaceId, windowId }} />
      <span className="empty-window-drop-hint">or drag a session here</span>
    </div>
  )
}
