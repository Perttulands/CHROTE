// Beads Module API - Self-contained API client
// This module is designed to be removable without affecting the rest of the application

import type {
  BeadsApiResponse,
  BeadsApiError,
  BeadsIssuesResponse,
  BeadsTriage,
  BeadsInsights,
  BeadsPlan,
} from './types';

// ============================================================================
// API CONFIGURATION
// ============================================================================

const API_BASE = '/api/beads';
const DEFAULT_TIMEOUT = 30000; // 30 seconds

// ============================================================================
// ERROR HANDLING
// ============================================================================

export class BeadsApiException extends Error {
  public readonly code: string;
  public readonly details?: string;

  constructor(error: BeadsApiError) {
    super(error.message);
    this.name = 'BeadsApiException';
    this.code = error.code;
    this.details = error.details;
  }
}

function createError(code: string, message: string, details?: string): BeadsApiError {
  return { code, message, details };
}

// ============================================================================
// FETCH WRAPPER WITH ERROR HANDLING
// ============================================================================

async function fetchWithTimeout<T>(
  url: string,
  options: RequestInit = {},
  timeout: number = DEFAULT_TIMEOUT
): Promise<BeadsApiResponse<T>> {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeout);

  try {
    const response = await fetch(url, {
      ...options,
      signal: controller.signal,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      const errorText = await response.text();
      let errorData: BeadsApiError;

      try {
        const parsed = JSON.parse(errorText);
        errorData = createError(
          parsed.error?.code || `HTTP_${response.status}`,
          parsed.error?.message || `HTTP ${response.status}: ${response.statusText}`,
          parsed.error?.details
        );
      } catch {
        errorData = createError(
          `HTTP_${response.status}`,
          `HTTP ${response.status}: ${response.statusText}`,
          errorText
        );
      }

      return {
        success: false,
        error: errorData,
        timestamp: new Date().toISOString(),
      };
    }

    const data = await response.json();
    return {
      success: true,
      data,
      timestamp: new Date().toISOString(),
    };
  } catch (error) {
    clearTimeout(timeoutId);

    if (error instanceof Error) {
      if (error.name === 'AbortError') {
        return {
          success: false,
          error: createError('TIMEOUT', `Request timed out after ${timeout}ms`),
          timestamp: new Date().toISOString(),
        };
      }

      return {
        success: false,
        error: createError('NETWORK_ERROR', error.message),
        timestamp: new Date().toISOString(),
      };
    }

    return {
      success: false,
      error: createError('UNKNOWN_ERROR', 'An unknown error occurred'),
      timestamp: new Date().toISOString(),
    };
  }
}

// ============================================================================
// API FUNCTIONS
// ============================================================================

/**
 * Fetch all issues from a beads project
 */
export async function fetchIssues(projectPath: string): Promise<BeadsApiResponse<BeadsIssuesResponse>> {
  const encodedPath = encodeURIComponent(projectPath);
  return fetchWithTimeout<BeadsIssuesResponse>(`${API_BASE}/issues?path=${encodedPath}`);
}

/**
 * Fetch AI triage recommendations
 */
export async function fetchTriage(projectPath: string): Promise<BeadsApiResponse<BeadsTriage>> {
  const encodedPath = encodeURIComponent(projectPath);
  // Triage can take longer due to AI analysis
  return fetchWithTimeout<BeadsTriage>(`${API_BASE}/triage?path=${encodedPath}`, {}, 60000);
}

/**
 * Fetch graph insights and metrics
 */
export async function fetchInsights(projectPath: string): Promise<BeadsApiResponse<BeadsInsights>> {
  const encodedPath = encodeURIComponent(projectPath);
  return fetchWithTimeout<BeadsInsights>(`${API_BASE}/insights?path=${encodedPath}`);
}

/**
 * Fetch execution plan
 */
export async function fetchPlan(projectPath: string): Promise<BeadsApiResponse<BeadsPlan>> {
  const encodedPath = encodeURIComponent(projectPath);
  return fetchWithTimeout<BeadsPlan>(`${API_BASE}/plan?path=${encodedPath}`, {}, 60000);
}

/**
 * Check API health
 */
export async function checkHealth(): Promise<BeadsApiResponse<{ status: string }>> {
  return fetchWithTimeout<{ status: string }>(`${API_BASE}/health`);
}

/**
 * Discover available beads projects
 * Searches common mount points (/code, /workspace) by default
 *
 * @param root - Optional root path to search from
 * @param depth - Maximum directory depth to search (default: 3)
 */
export async function discoverProjects(
  root?: string,
  depth: number = 3
): Promise<BeadsApiResponse<{ projects: string[]; searchRoots?: string[] }>> {
  const params = new URLSearchParams();
  if (root) params.set('root', root);
  params.set('depth', depth.toString());

  const query = params.toString();
  return fetchWithTimeout<{ projects: string[]; searchRoots?: string[] }>(
    `${API_BASE}/projects${query ? `?${query}` : ''}`
  );
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Unwrap API response or throw error
 */
export function unwrapResponse<T>(response: BeadsApiResponse<T>): T {
  if (!response.success || !response.data) {
    throw new BeadsApiException(
      response.error || createError('UNKNOWN_ERROR', 'Failed to unwrap response')
    );
  }
  return response.data;
}

/**
 * Check if an error is a BeadsApiException
 */
export function isBeadsApiError(error: unknown): error is BeadsApiException {
  return error instanceof BeadsApiException;
}

/**
 * Format error for display
 */
export function formatError(error: BeadsApiError): string {
  if (error.details) {
    return `${error.message} (${error.details})`;
  }
  return error.message;
}
