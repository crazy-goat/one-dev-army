import { useEffect, useRef } from 'react'
import { useStore } from '../store'
import { mutate } from 'swr'

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reconnectDelayRef = useRef(1000)
  const setWsConnected = useStore((s) => s.setWsConnected)
  const setWorkerStatus = useStore((s) => s.setWorkerStatus)

  useEffect(() => {
    let isManualClose = false

    function connect() {
      if (wsRef.current) return

      const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const wsUrl = `${wsProtocol}//${window.location.host}/ws`
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.onopen = () => {
        setWsConnected(true)
        reconnectDelayRef.current = 1000
      }

      ws.onclose = () => {
        setWsConnected(false)
        wsRef.current = null
        if (!isManualClose) {
          scheduleReconnect()
        }
      }

      ws.onerror = () => {
        // onclose will fire after onerror
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)

          switch (msg.type) {
            case 'worker_update': {
              const payload = JSON.parse(msg.payload)
              setWorkerStatus({
                active: payload.status === 'active',
                paused: false,
                step: payload.stage || '',
                issue_id: payload.task_id || 0,
                issue_title: payload.task_title || '',
              })
              // Refresh board data
              mutate('board')
              break
            }
            case 'issue_update':
            case 'sync_complete':
              mutate('board')
              mutate('sprint')
              break
          }
        } catch {
          // Ignore malformed messages
        }
      }
    }

    function scheduleReconnect() {
      if (reconnectTimerRef.current) return
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null
        connect()
      }, reconnectDelayRef.current)
      reconnectDelayRef.current = Math.min(reconnectDelayRef.current * 2, 30000)
    }

    connect()

    return () => {
      isManualClose = true
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
      }
      if (wsRef.current) {
        wsRef.current.close()
      }
    }
  }, [setWsConnected, setWorkerStatus])
}
