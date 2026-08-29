import { ServicesApiError, type ServiceStatus } from '../../services/servicesClient'

export function errorMessage(error: unknown): string {
  if (error instanceof ServicesApiError) return error.message
  if (error instanceof Error) return error.message
  return 'Service request failed'
}

export function statusLabel(service?: ServiceStatus): string {
  return service?.status ?? 'unknown'
}

export function formatDate(raw?: string) {
  if (!raw) return ''
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
}
