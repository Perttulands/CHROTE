import { useEffect } from 'react'
import { useSession } from '../context/SessionContext'
import { useStatus } from '../context/StatusContext'
import { refreshBeadProjects } from './beadIds'

/** One nonblocking catalog owner; terminals only read the resulting registry. */
export default function BeadCatalog() {
  const { settings } = useSession()
  const { announce } = useStatus()
  const paths = JSON.stringify([...new Set((settings.beadsProjectPaths ?? []).map(path => path.trim()).filter(Boolean))].sort())
  useEffect(() => {
    let current = true
    void refreshBeadProjects(JSON.parse(paths) as string[]).catch((cause: unknown) => {
      if (current) announce(cause instanceof Error ? cause.message : 'Could not discover Bead links', 'error')
    })
    return () => { current = false }
  }, [announce, paths])
  return null
}
