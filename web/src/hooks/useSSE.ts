import { useEffect, useRef, useCallback } from 'react'

/**
 * Subscribe to a Server-Sent Events endpoint with automatic reconnection.
 *
 * @param url  The SSE endpoint URL, or `null` to disable.
 * @param onEvent  Callback invoked with each parsed JSON event payload.
 */
export function useSSE(url: string | null, onEvent: (data: unknown) => void) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  const reconnectDelayRef = useRef(1000)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const shouldReconnectRef = useRef(true)

  const connect = useCallback((targetUrl: string) => {
    const source = new EventSource(targetUrl)

    source.onopen = () => {
      // Reset backoff on successful connection
      reconnectDelayRef.current = 1000
    }

    source.onmessage = (e: MessageEvent) => {
      try {
        const data: unknown = JSON.parse(e.data as string)
        onEventRef.current(data)
      } catch {
        /* ignore malformed events */
      }
    }

    source.onerror = () => {
      source.close()

      // Reconnect with exponential backoff
      if (shouldReconnectRef.current) {
        const delay = reconnectDelayRef.current
        reconnectTimerRef.current = setTimeout(() => {
          connect(targetUrl)
        }, delay)
        // Exponential backoff capped at 30s
        reconnectDelayRef.current = Math.min(delay * 2, 30_000)
      }
    }

    return source
  }, [])

  useEffect(() => {
    if (url === null) {return}

    shouldReconnectRef.current = true
    reconnectDelayRef.current = 1000
    const source = connect(url)

    return () => {
      shouldReconnectRef.current = false
      if (reconnectTimerRef.current !== null) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
      source.close()
    }
  }, [url, connect])
}
