import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import type { WorkRow } from '../../beads/beadsTree'

interface FlowNavigation {
  rows: readonly WorkRow[]
  reveal: (row: WorkRow) => void
}

const FlowNavigationContext = createContext<FlowNavigation | null>(null)

export function FlowNavigationProvider({ rows, reveal, children }: FlowNavigation & { children: ReactNode }) {
  return (
    <FlowNavigationContext.Provider value={{ rows, reveal }}>
      {children}
    </FlowNavigationContext.Provider>
  )
}

/** The row menu's route into the one Flow owned by the enclosing Beads tab. */
export function useFlowNavigation(row: WorkRow) {
  const navigation = useContext(FlowNavigationContext)
  return {
    linked: navigation !== null && row.linked === true,
    open: () => navigation?.reveal(row),
  }
}
