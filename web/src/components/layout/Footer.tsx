import { useRateLimit, useToggleYolo, useBoard } from '../../api/queries'
import { useAppContext } from '../../App'

/** Returns a color class based on the percentage of remaining rate limit. */
function rateLimitColor(remaining: number, limit: number): string {
  if (limit === 0) return 'text-gray-500'
  const pct = remaining / limit
  if (pct > 0.5) return 'text-green-400'
  if (pct > 0.2) return 'text-yellow-400'
  return 'text-red-400'
}

export function Footer() {
  const { data: rateLimit } = useRateLimit()
  const { data: board } = useBoard()
  const toggleYolo = useToggleYolo()
  const { wsConnected } = useAppContext()

  return (
    <footer className="bg-gray-900 border-t border-gray-800 px-4 py-2 mt-auto">
      <div className="flex items-center justify-between w-full text-xs text-gray-500">
        <div className="flex items-center gap-4">
          <span>ODA &mdash; One Dev Army</span>
          <a
            href="/"
            className="text-blue-400 hover:text-blue-300 transition-colors"
          >
            &larr; Classic dashboard
          </a>
        </div>

        <div className="flex items-center gap-4">
          {/* MISSING 11: YOLO quick-toggle */}
          {board && (
            <button
              type="button"
              onClick={() => toggleYolo.mutate()}
              disabled={toggleYolo.isPending}
              className={`flex items-center gap-1 px-2 py-0.5 rounded border text-xs font-medium transition-colors disabled:opacity-50 ${
                board.yolo_mode
                  ? 'border-yellow-600/40 bg-yellow-600/10 text-yellow-400 hover:bg-yellow-600/20'
                  : 'border-gray-700 bg-gray-800 text-gray-500 hover:bg-gray-700'
              }`}
              title={
                board.yolo_mode
                  ? 'YOLO mode ON — PRs auto-merge without review. Click to disable.'
                  : 'YOLO mode OFF — PRs require manual approval. Click to enable.'
              }
            >
              {board.yolo_mode ? '\uD83D\uDD25 YOLO' : 'YOLO'}
            </button>
          )}

          {/* MISSING 14: Rate limit with color coding */}
          {rateLimit && (
            <span className={rateLimitColor(rateLimit.remaining, rateLimit.limit)}>
              GitHub API: {rateLimit.remaining}/{rateLimit.limit}
            </span>
          )}

          {/* MISSING 9: WebSocket connection status indicator */}
          <div className="flex items-center gap-1.5" title={wsConnected ? 'WebSocket connected' : 'WebSocket disconnected'}>
            <span
              className={`w-2 h-2 rounded-full ${
                wsConnected ? 'bg-green-500' : 'bg-red-500'
              }`}
            />
            <span className="text-gray-600">
              {wsConnected ? 'WS' : 'WS'}
            </span>
          </div>
        </div>
      </div>
    </footer>
  )
}
