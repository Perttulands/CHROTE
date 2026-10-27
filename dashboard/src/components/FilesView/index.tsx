/**
 * The Files tab: the Files panel a terminal keeps beside it, as a tab of its
 * own. The same find, tree, pins and menu stand in the tab's column, and the
 * file they open fills the rest instead of hanging off the panel's edge. A
 * request from elsewhere (a Files link, Open in Files) is a folder to stand in
 * or a file to read, as it is for the panel.
 */

import { useCallback, useEffect, useState } from 'react'
import TerminalFilesPanel from '../TerminalFilesPanel'
import {
  readWorkspaceFilesDockState,
  writeWorkspaceFilesDockState,
} from '../workspaceFilesState'
import type { FilesViewProps } from './types'

function FilesView({ navigateRequest, sendTarget = null }: FilesViewProps) {
  const [dock, setDock] = useState(() => readWorkspaceFilesDockState('files'))
  const [request, setRequest] = useState<{ path: string; requestId: number } | null>(null)

  useEffect(() => { writeWorkspaceFilesDockState('files', dock) }, [dock])
  useEffect(() => {
    if (navigateRequest) setRequest({ path: navigateRequest.path, requestId: navigateRequest.nonce })
  }, [navigateRequest])

  const openHere = useCallback((path: string) => setRequest({ path, requestId: Date.now() }), [])

  return (
    <TerminalFilesPanel
      workspaceId="files"
      place="tab"
      sendTarget={sendTarget}
      collapsed={false}
      pinned
      width={dock.width}
      panelId="files-tab-panel"
      onClose={() => undefined}
      onWidthChange={width => setDock(previous => ({ ...previous, width }))}
      onOpenInFiles={openHere}
      navigateRequest={request}
      onNavigateRequestHandled={() => setRequest(null)}
    />
  )
}

export default FilesView
