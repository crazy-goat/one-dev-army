import { useEffect, useRef, useState, useCallback } from 'react'
import { Link } from 'react-router'

import type { CurrentTicket, LogStreamPayload } from '../../api/types'
import { useAppContext } from '../../hooks/useAppContext'

interface ProcessingPanelProps {
  currentTicket?: CurrentTicket
  totalTickets: number
}

/** Maximum number of log lines to keep in the mini viewer. */
const MAX_LOG_LINES = 20

function priorityBadge(priority: string) {
  const colors: Record<string, string> = {
    high: 'border-red-500/30 bg-red-500/10 text-red-400',
    medium: 'border-yellow-500/30 bg-yellow-500/10 text-yellow-400',
    low: 'border-green-500/30 bg-green-500/10 text-green-400',
  }
  const icons: Record<string, string> = {
    high: '\uD83D\uDD34',
    medium: '\uD83D\uDFE1',
    low: '\uD83D\uDFE2',
  }
  return (
    <span
      className={`text-xs px-2 py-0.5 rounded border capitalize ${colors[priority] ?? 'border-gray-700 bg-gray-800 text-gray-400'}`}
    >
      {icons[priority] ?? ''} {priority}
    </span>
  )
}

function typeBadge(type: string) {
  return (
    <span className="text-xs px-2 py-0.5 rounded border border-gray-700 bg-gray-800 text-gray-400">
      {type === 'bug' ? '\uD83D\uDC1B Bug' : '\u2728 Feature'}
    </span>
  )
}

function sizeBadge(size: string) {
  const sizeEmojis: Record<string, string> = {
    S: '\uD83D\uDC1C',
    M: '\uD83D\uDC15',
    L: '\uD83D\uDC18',
    XL: '\uD83E\uDD95',
  }
  return (
    <span className="text-xs px-2 py-0.5 rounded border border-gray-700 bg-gray-800 text-gray-400">
      {sizeEmojis[size] ?? '\uD83D\uDCCF'} {size}
    </span>
  )
}

export function ProcessingPanel({ currentTicket, totalTickets }: ProcessingPanelProps) {
  const { onLogStream } = useAppContext()
  const [logLines, setLogLines] = useState<string[]>([])
  const logContainerRef = useRef<HTMLDivElement>(null)

  // MISSING 12: Subscribe to log_stream WebSocket messages
  const handleLogStream = useCallback(
    (payload: LogStreamPayload) => {
      if (payload.issue_number !== currentTicket?.number) {
        return
      }
      const line = payload.message || ''
      setLogLines(prev => {
        const updated = [...prev, line]
        // Keep only the last MAX_LOG_LINES
        return updated.length > MAX_LOG_LINES
          ? updated.slice(updated.length - MAX_LOG_LINES)
          : updated
      })
    },
    [currentTicket]
  )

  useEffect(() => {
    const unsub = onLogStream(handleLogStream)
    return unsub
  }, [onLogStream, handleLogStream])

  // Clear log lines when ticket changes
  useEffect(() => {
    setLogLines([])
  }, [currentTicket?.number])

  // Auto-scroll log viewer
  useEffect(() => {
    const el = logContainerRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [logLines])

  if (!currentTicket) {
    return (
      <div className="bg-gray-500/8 border border-gray-500/20 rounded-lg p-4 h-full flex flex-col">
        <div className="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-2">
          Processing
        </div>
        {totalTickets === 0 ? (
          <div className="flex flex-col gap-1">
            <span className="text-lg font-semibold text-gray-200">No tickets in sprint</span>
            <span className="text-sm text-gray-500">Create your first ticket to get started</span>
            <Link
              to="/wizard"
              className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-blue-400 bg-gray-800 border border-gray-700 rounded-md mt-2 hover:bg-gray-700 hover:border-blue-500 transition-colors w-fit"
            >
              + New Ticket
            </Link>
          </div>
        ) : (
          <span className="text-sm text-gray-500">No active ticket &mdash; Worker ready</span>
        )}
      </div>
    )
  }

  return (
    <div className="bg-blue-500/8 border border-blue-500/20 rounded-lg p-4 h-full flex flex-col">
      <div className="flex-shrink-0">
        <div className="text-xs font-semibold uppercase tracking-wider text-gray-500 mb-2 flex items-center gap-2">
          Processing
          <span className="inline-block w-2 h-2 rounded-full bg-green-500 animate-pulse" />
        </div>

        {/* Badges */}
        <div className="flex gap-1.5 flex-wrap mb-2">
          {currentTicket.priority !== undefined && priorityBadge(currentTicket.priority)}
          {currentTicket.type !== undefined && typeBadge(currentTicket.type)}
          {currentTicket.size !== undefined && sizeBadge(currentTicket.size)}
        </div>

        {/* Ticket info */}
        <Link to={`/task/${String(currentTicket.number)}`} className="block group">
          <span className="text-xs text-gray-500 font-medium">#{currentTicket.number}</span>
          <span className="block text-xl font-semibold text-gray-100 leading-snug group-hover:text-white transition-colors line-clamp-2">
            {currentTicket.title}
          </span>
        </Link>

        {/* Status */}
        <div className="mt-2 text-xs text-blue-400 capitalize">{currentTicket.status}</div>
      </div>

      {/* MISSING 12: Mini log viewer */}
      {logLines.length > 0 && (
        <div
          ref={logContainerRef}
          className="mt-3 flex-1 min-h-0 overflow-y-auto bg-gray-950 border border-gray-800 rounded p-2 font-mono text-xs text-gray-400 leading-relaxed"
        >
          {logLines.map((line, i) => (
            <div key={`log-${String(i)}`} className="whitespace-pre-wrap break-words">
              {line}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
