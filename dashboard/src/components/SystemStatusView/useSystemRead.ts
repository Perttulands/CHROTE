import { useCallback, useEffect, useRef, useState } from 'react'
import { SystemApiError } from '../../services/systemClient'

const REQUEST_TIMEOUT_MS = 10_000

interface PendingRead {
  controller: AbortController
  timeout: number
}

/** The two Server resources have separate lifetimes: slow history must not hold
 * up status. Keep the last good answer while one bounded read is in flight. */
export function useSystemRead<T>(
  read: (signal?: AbortSignal) => Promise<T>,
  visible: boolean,
  paused: boolean,
  intervalMs: number,
) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const pending = useRef<PendingRead | null>(null)

  const cancel = useCallback(() => {
    const request = pending.current
    pending.current = null
    if (request) {
      window.clearTimeout(request.timeout)
      request.controller.abort()
    }
  }, [])

  const refresh = useCallback(() => {
    if (!visible || pending.current) return
    const request: PendingRead = { controller: new AbortController(), timeout: 0 }
    pending.current = request
    request.timeout = window.setTimeout(() => {
      if (pending.current !== request) return
      cancel()
      setError('Request timed out after 10 seconds')
      setLoading(false)
    }, REQUEST_TIMEOUT_MS)

    void read(request.controller.signal).then(value => {
      if (pending.current !== request) return
      setData(value)
      setError('')
    }, cause => {
      if (pending.current !== request) return
      setError(cause instanceof SystemApiError
        ? `${cause.code}: ${cause.message}`
        : cause instanceof Error ? cause.message : 'System request failed')
    }).finally(() => {
      window.clearTimeout(request.timeout)
      if (pending.current !== request) return
      pending.current = null
      setLoading(false)
    })
  }, [read, visible, cancel])

  useEffect(() => {
    let interval: number | undefined
    if (visible && !paused) {
      refresh()
      interval = window.setInterval(refresh, intervalMs)
    }
    return () => {
      window.clearInterval(interval)
      cancel()
    }
  }, [visible, paused, intervalMs, refresh, cancel])

  return { data, error, loading, refresh }
}
