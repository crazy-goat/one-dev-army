import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { OdaWebSocket } from '../lib/websocket'
import type { WSMessage } from '../api/types'

/**
 * Connects to the ODA WebSocket and invalidates TanStack Query caches
 * when relevant server-side events arrive.
 *
 * Should be called once at the App root level.
 */
export function useWebSocketUpdates() {
  const queryClient = useQueryClient()
  const wsRef = useRef<OdaWebSocket | null>(null)

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws`
    const ws = new OdaWebSocket(wsUrl)
    wsRef.current = ws

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
      }
    })

    return () => ws.close()
  }, [queryClient])
}
