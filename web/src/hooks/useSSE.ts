import { useEffect, useRef } from 'react'

/**
 * Subscribe to a Server-Sent Events endpoint.
 *
 * @param url  The SSE endpoint URL, or `null` to disable.
 * @param onEvent  Callback invoked with each parsed JSON event payload.
 */
export function useSSE(url: string | null, onEvent: (data: unknown) => void) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    if (!url) return

    const source = new EventSource(url)

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
    }

    return () => source.close()
  }, [url])
}
