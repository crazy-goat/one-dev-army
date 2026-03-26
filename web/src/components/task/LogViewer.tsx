import { useEffect, useRef, useState, useCallback } from 'react'
import { useSSE } from '../../hooks/useSSE'

interface LogViewerProps {
  issueNumber: number
}

interface StreamEvent {
  delta?: string
  done?: boolean
  history?: Array<{ role: string; content: string }>
}

export function LogViewer({ issueNumber }: LogViewerProps) {
  const [lines, setLines] = useState<string[]>([])
  const [history, setHistory] = useState<
    Array<{ role: string; content: string }>
  >([])
  const [connected, setConnected] = useState(true)
  const containerRef = useRef<HTMLDivElement>(null)

  const handleEvent = useCallback((data: unknown) => {
    const event = data as StreamEvent

    if (event.done) {
      setConnected(false)
      return
    }

    if (event.history) {
      setHistory(event.history)
      return
    }

    if (event.delta) {
      setLines((prev) => {
        const updated = [...prev]
        const lastIdx = updated.length - 1
        if (lastIdx >= 0 && !updated[lastIdx]!.endsWith('\n')) {
          updated[lastIdx] = updated[lastIdx]! + event.delta!
        } else {
          updated.push(event.delta!)
        }
        return updated
      })
    }
  }, [])

  useSSE(`/api/v2/issues/${String(issueNumber)}/stream`, handleEvent)

  // Auto-scroll to bottom
  useEffect(() => {
    const el = containerRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    }
  }, [lines, history])

  return (
    <div className="bg-gray-950 border border-gray-800 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="flex justify-between items-center px-3 py-2 bg-gray-900 border-b border-gray-800 text-xs">
        <span className="text-gray-400">Live Output</span>
        <div className="flex items-center gap-1.5">
          <span
            className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500 animate-pulse' : 'bg-gray-600'}`}
          />
          <span className="text-gray-500">
            {connected ? 'Connected' : 'Disconnected'}
          </span>
        </div>
      </div>

      {/* Content */}
      <div
        ref={containerRef}
        className="max-h-[500px] overflow-y-auto p-3 font-mono text-sm leading-relaxed"
      >
        {/* Historical messages */}
        {history.map((msg, i) => (
          <div
            key={`hist-${String(i)}`}
            className={`mb-2 p-2 rounded ${
              msg.role === 'user'
                ? 'bg-blue-500/10 border-l-2 border-blue-500'
                : 'bg-green-500/10 border-l-2 border-green-500'
            }`}
          >
            <div
              className={`text-xs font-bold mb-1 ${
                msg.role === 'user' ? 'text-blue-400' : 'text-green-400'
              }`}
            >
              {msg.role === 'user' ? 'User' : 'Assistant'}
            </div>
            <div className="text-gray-300 whitespace-pre-wrap break-words">
              {msg.content}
            </div>
          </div>
        ))}

        {/* Separator */}
        {history.length > 0 && lines.length > 0 && (
          <div className="text-center text-gray-600 text-xs my-2 border-t border-gray-800 pt-2">
            --- Live ---
          </div>
        )}

        {/* Live streaming content */}
        {lines.length > 0 ? (
          <div className="text-gray-300 whitespace-pre-wrap break-words">
            {lines.join('')}
            <span className="inline-block w-2 h-[1em] bg-blue-500 animate-[blink_1s_step-end_infinite] ml-0.5" />
          </div>
        ) : (
          history.length === 0 && (
            <p className="text-gray-600 italic text-center py-4">
              Waiting for output...
            </p>
          )
        )}
      </div>
    </div>
  )
}
