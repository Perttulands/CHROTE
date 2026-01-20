// Mail types for Gas Town messaging system

export interface MailMessage {
  id: string
  from: string
  to: string
  subject: string
  body: string
  timestamp: string
  read: boolean
}

export interface MailRecipient {
  id: string
  name: string
  role: 'mayor' | 'deacon' | 'witness' | 'refinery' | 'crew' | 'polecat' | 'human'
  path: string // e.g., "mayor/", "Chrote/crew/Ronja", "--human"
  online: boolean
}

export interface TownStatus {
  name: string
  overseer: string
  roles: {
    mayor: RoleInfo | null
    deacon: RoleInfo | null
    witness: RoleInfo | null
    refinery: RoleInfo | null
    crew: RoleInfo[]
    polecats: RoleInfo[]
  }
}

export interface RoleInfo {
  name: string
  online: boolean
  unread: number
}

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: {
    code: string
    message: string
  }
  timestamp: string
}

export type MailSubTab = 'inbox' | 'compose'
