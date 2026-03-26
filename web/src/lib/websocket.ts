import type { WSMessage } from '../api/types'

type MessageHandler = (msg: WSMessage) => void

/**
 * Reconnecting WebSocket client for the ODA dashboard.
 *
 * Features:
 * - Automatic reconnection with exponential back-off (1 s → 30 s cap)
 * - Periodic ping to keep the connection alive
 * - Simple subscribe/unsubscribe via `onMessage`
 */
export class OdaWebSocket {
  private ws: WebSocket | null = null
  private readonly url: string
  private handlers: MessageHandler[] = []
  private reconnectDelay = 1000
  private readonly maxReconnectDelay = 30_000
  private shouldReconnect = true
  private pingInterval: ReturnType<typeof setInterval> | null = null

  constructor(url: string) {
    this.url = url
    this.connect()
  }

  private connect() {
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      this.reconnectDelay = 1000
      this.startPing()
    }

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg = JSON.parse(event.data as string) as WSMessage
        if (msg.type === 'pong') return
        this.handlers.forEach((h) => h(msg))
      } catch {
        /* ignore malformed frames */
      }
    }

    this.ws.onclose = () => {
      this.stopPing()
      if (this.shouldReconnect) {
        setTimeout(() => this.connect(), this.reconnectDelay)
        this.reconnectDelay = Math.min(
          this.reconnectDelay * 2,
          this.maxReconnectDelay,
        )
      }
    }

    this.ws.onerror = () => {
      this.ws?.close()
    }
  }

  private startPing() {
    this.pingInterval = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'ping' }))
      }
    }, 30_000)
  }

  private stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval)
      this.pingInterval = null
    }
  }

  /** Subscribe to incoming messages. Returns an unsubscribe function. */
  onMessage(handler: MessageHandler): () => void {
    this.handlers.push(handler)
    return () => {
      this.handlers = this.handlers.filter((h) => h !== handler)
    }
  }

  /** Permanently close the connection (no reconnect). */
  close() {
    this.shouldReconnect = false
    this.stopPing()
    this.ws?.close()
  }
}
