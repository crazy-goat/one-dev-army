import type { WSMessage } from '../api/types'

type MessageHandler = (msg: WSMessage) => void
type StatusHandler = (connected: boolean) => void

/**
 * Reconnecting WebSocket client for the ODA dashboard.
 *
 * Features:
 * - Automatic reconnection with exponential back-off (1 s -> 30 s cap)
 * - Periodic ping to keep the connection alive
 * - Simple subscribe/unsubscribe via `onMessage`
 * - Connection status tracking via `onStatusChange`
 */
export class OdaWebSocket {
  private ws: WebSocket | null = null
  private readonly url: string
  private handlers: MessageHandler[] = []
  private statusHandlers: StatusHandler[] = []
  private reconnectDelay = 1000
  private readonly maxReconnectDelay = 30_000
  private shouldReconnect = true
  private pingInterval: ReturnType<typeof setInterval> | null = null
  private _connected = false

  constructor(url: string) {
    this.url = url
    this.connect()
  }

  get connected(): boolean {
    return this._connected
  }

  private setConnected(value: boolean) {
    if (this._connected !== value) {
      this._connected = value
      this.statusHandlers.forEach((h) => h(value))
    }
  }

  private connect() {
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      this.reconnectDelay = 1000
      this.setConnected(true)
      this.startPing()
    }

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg = JSON.parse(event.data as string) as WSMessage
        if (msg.type === 'pong') {return}
        this.handlers.forEach((h) => h(msg))
      } catch {
        /* ignore malformed frames */
      }
    }

    this.ws.onclose = () => {
      this.stopPing()
      this.setConnected(false)
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

  /** Subscribe to connection status changes. Returns an unsubscribe function. */
  onStatusChange(handler: StatusHandler): () => void {
    this.statusHandlers.push(handler)
    return () => {
      this.statusHandlers = this.statusHandlers.filter((h) => h !== handler)
    }
  }

  /** Permanently close the connection (no reconnect). */
  close() {
    this.shouldReconnect = false
    this.stopPing()
    this.ws?.close()
  }
}
