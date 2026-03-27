import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState, useCallback } from 'react'

import type { WSMessage, LogStreamPayload } from '../api/types'
import { OdaWebSocket } from '../lib/websocket'

/** Callback for log_stream messages from WebSocket. */
type LogStreamHandler = (payload: LogStreamPayload) => void

/**
 * Connects to the ODA WebSocket and invalidates TanStack Query caches
 * when relevant server-side events arrive.
 *
 * Returns `{ wsConnected, onLogStream }` for use by child components.
 *
 * Should be called once at the App root level.
 */
export function useWebSocketUpdates() {
  const queryClient = useQueryClient()
  const wsRef = useRef<OdaWebSocket | null>(null)
  const [wsConnected, setWsConnected] = useState(false)
  const logStreamHandlersRef = useRef<LogStreamHandler[]>([])

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws`
    const ws = new OdaWebSocket(wsUrl)
    wsRef.current = ws

    ws.onStatusChange((connected) => {
      setWsConnected(connected)
    })

    ws.onMessage((msg: WSMessage) => {
      switch (msg.type) {
        case 'issue_update':
        case 'sync_complete':
          void queryClient.invalidateQueries({ queryKey: ['board'] })
          break
        case 'worker_update':
          void queryClient.invalidateQueries({ queryKey: ['workers'] })
          break
        case 'can_close_sprint':
          void queryClient.invalidateQueries({ queryKey: ['sprint'] })
          void queryClient.invalidateQueries({ queryKey: ['board'] })
          break
        case 'log_stream':
          // Forward log_stream messages to registered handlers
          if (msg.payload !== null) {
            const payload = msg.payload as LogStreamPayload
            logStreamHandlersRef.current.forEach((h) => h(payload))
          }
          break
      }
    })

    return () => ws.close()
  }, [queryClient])

  /** Register a handler for log_stream messages. Returns unsubscribe function. */
  const onLogStream = useCallback((handler: LogStreamHandler): (() => void) => {
    logStreamHandlersRef.current.push(handler)
    return () => {
      logStreamHandlersRef.current = logStreamHandlersRef.current.filter(
        (h) => h !== handler,
      )
    }
  }, [])

  return { wsConnected, onLogStream }
}
