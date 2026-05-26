export const FEATURE_FLAGS = {
  uiV2: 'chrote-ui-v2',
  beadsAllStatuses: 'chrote-beads-all-statuses',
  beadsDetailModal: 'chrote-beads-detail-modal',
  sessionLocationBadges: 'chrote-session-location-badges',
  filesPersistTabState: 'chrote-files-persist-tab-state',
  filesResizablePreview: 'chrote-files-resizable-preview',
  terminalRefitButton: 'chrote-terminal-refit-button',
  serverStatusTab: 'chrote-server-status-tab',
} as const

export type FeatureFlagName = keyof typeof FEATURE_FLAGS

const DEFAULT_ENABLED: Record<FeatureFlagName, boolean> = {
  uiV2: false,
  beadsAllStatuses: true,
  beadsDetailModal: true,
  sessionLocationBadges: true,
  filesPersistTabState: true,
  filesResizablePreview: true,
  terminalRefitButton: true,
  serverStatusTab: true,
}

function storageAvailable(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function enabledValue(value: string | null): boolean {
  return value === '1' || value === 'true' || value === 'on'
}

function disabledValue(value: string | null): boolean {
  return value === '0' || value === 'false' || value === 'off'
}

export function featureFlagKey(flag: FeatureFlagName): string {
  return FEATURE_FLAGS[flag]
}

export function isFeatureEnabled(flag: FeatureFlagName): boolean {
  const defaultEnabled = DEFAULT_ENABLED[flag]
  if (!storageAvailable()) return defaultEnabled
  const stored = window.localStorage.getItem(featureFlagKey(flag))
  if (enabledValue(stored)) return true
  if (disabledValue(stored)) return false
  return defaultEnabled
}

export function setFeatureEnabled(flag: FeatureFlagName, enabled: boolean): void {
  if (!storageAvailable()) return
  window.localStorage.setItem(featureFlagKey(flag), enabled ? '1' : '0')
}

export function listFeatureFlags(): Record<string, { key: string; enabled: boolean }> {
  return Object.fromEntries(
    Object.keys(FEATURE_FLAGS).map(flag => [
      flag,
      {
        key: featureFlagKey(flag as FeatureFlagName),
        enabled: isFeatureEnabled(flag as FeatureFlagName),
      },
    ])
  )
}

declare global {
  interface Window {
    chroteFeatureFlags?: {
      list: typeof listFeatureFlags
      enable: (flag: FeatureFlagName) => void
      disable: (flag: FeatureFlagName) => void
    }
  }
}

export function installFeatureFlagHelpers(): void {
  if (typeof window === 'undefined') return
  window.chroteFeatureFlags = {
    list: listFeatureFlags,
    enable: flag => setFeatureEnabled(flag, true),
    disable: flag => setFeatureEnabled(flag, false),
  }
}
