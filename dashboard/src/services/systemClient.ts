export interface ApiErrorPayload {
  code: string
  message: string
}

interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: ApiErrorPayload
}

export class SystemApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'SystemApiError'
    this.code = code
    this.status = status
  }
}

export interface SystemStatus {
  timestamp: string
  host: SystemHostStatus
  cpu: SystemCPUStatus
  memory: SystemMemoryStatus
  disks: SystemDiskStatus[]
  network: SystemNetworkStatus[]
  gpus: SystemGPUStatus[]
  warnings?: SystemWarning[]
}

export interface SystemHostStatus {
  hostname: string
  uptimeSeconds: number
  load1: number
  load5: number
  load15: number
}

export interface SystemCPUStatus {
  cores: number
  totalTicks: number
  idleTicks: number
}

export interface SystemMemoryStatus {
  totalBytes: number
  availableBytes: number
  usedBytes: number
  usedPercent: number
  swapTotalBytes: number
  swapUsedBytes: number
  swapUsedPercent: number
}

export interface SystemDiskStatus {
  mount: string
  totalBytes: number
  availableBytes: number
  usedBytes: number
  usedPercent: number
}

export interface SystemNetworkStatus {
  name: string
  rxBytes: number
  txBytes: number
}

export interface SystemGPUStatus {
  available: boolean
  name?: string
  utilizationPercent?: number
  memoryTotalBytes?: number
  memoryUsedBytes?: number
  temperatureCelsius?: number
  powerWatts?: number
  message?: string
}

export interface SystemWarning {
  code: string
  message: string
}

async function request<T>(path: string): Promise<T> {
  const response = await fetch(path)

  let envelope: ApiEnvelope<T>
  try {
    envelope = await response.json()
  } catch {
    throw new SystemApiError('SYSTEM_INVALID_RESPONSE', 'System status returned invalid JSON', response.status)
  }

  if (!response.ok || !envelope.success) {
    const code = envelope.error?.code || 'SYSTEM_REQUEST_FAILED'
    const message = envelope.error?.message || `System status request failed with status ${response.status}`
    throw new SystemApiError(code, message, response.status)
  }

  return envelope.data as T
}

export function getSystemStatus() {
  return request<SystemStatus>('/api/system/status')
}
