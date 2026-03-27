import { Link, useLocation } from 'react-router'
import {
  useBoard,
  useStartSprint,
  usePauseSprint,
  useTriggerSync,
  useWorkers,
} from '../../api/queries'

const NAV_LINKS = [
  { path: '/', label: 'Board' },
  { path: '/wizard', label: 'New Issue' },
  { path: '/settings', label: 'Settings' },
] as const

/** Format elapsed milliseconds to a human-readable string. */
function formatElapsed(ms: number): string {
  const secs = Math.floor(ms / 1000)
  if (secs < 60) return `${String(secs)}s`
  const mins = Math.floor(secs / 60)
  const remainSecs = secs % 60
  return `${String(mins)}m ${String(remainSecs)}s`
}

export function Navbar() {
  const location = useLocation()
  const { data: board } = useBoard()
  const { data: workersData } = useWorkers()
  const startSprint = useStartSprint()
  const pauseSprint = usePauseSprint()
  const triggerSync = useTriggerSync()

  const isActive = (path: string) => location.pathname === path

  // MISSING 10: Find active workers for detail display
  const activeWorkers = workersData?.workers.filter(
    (w) => w.status === 'working' || w.status === 'busy',
  ) ?? []

  return (
    <nav className="bg-gray-900 border-b border-gray-800 px-4 py-3">
      <div className="flex items-center justify-between max-w-screen-2xl mx-auto">
        {/* Left: Logo + Navigation */}
        <div className="flex items-center gap-6">
          <Link to="/" className="text-xl font-bold text-white">
            ⚔️ ODA
          </Link>
          <div className="flex gap-1">
            {NAV_LINKS.map(({ path, label }) => (
              <Link
                key={path}
                to={path}
                className={`px-3 py-1.5 rounded text-sm font-medium transition-colors ${
                  isActive(path)
                    ? 'bg-gray-700 text-white'
                    : 'text-gray-400 hover:text-white hover:bg-gray-800'
                }`}
              >
                {label}
              </Link>
            ))}
          </div>
        </div>

        {/* Right: Sprint info + worker detail + controls */}
        <div className="flex items-center gap-4">
          {/* MISSING 10: Worker status detail */}
          {activeWorkers.length > 0 ? (
            <div className="flex items-center gap-2">
              {activeWorkers.slice(0, 2).map((w) => (
                <div
                  key={w.id}
                  className="flex items-center gap-1.5 text-xs bg-gray-800 border border-gray-700 px-2 py-1 rounded"
                  title={`Worker ${w.id}: ${w.stage ?? 'working'} on #${String(w.task_id ?? 0)}`}
                >
                  <span className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse" />
                  {w.task_id ? (
                    <Link
                      to={`/task/${String(w.task_id)}`}
                      className="text-blue-400 hover:text-blue-300 transition-colors"
                    >
                      #{w.task_id}
                    </Link>
                  ) : (
                    <span className="text-gray-400">idle</span>
                  )}
                  {w.stage && (
                    <span className="text-gray-500 capitalize">{w.stage}</span>
                  )}
                  {w.elapsed_ms != null && w.elapsed_ms > 0 && (
                    <span className="text-gray-600">
                      {formatElapsed(w.elapsed_ms)}
                    </span>
                  )}
                </div>
              ))}
              {activeWorkers.length > 2 && (
                <span className="text-xs text-gray-500">
                  +{activeWorkers.length - 2} more
                </span>
              )}
            </div>
          ) : (
            board && (
              <span className="text-xs bg-gray-800 text-gray-300 px-2 py-1 rounded">
                {board.worker_count} worker{board.worker_count !== 1 ? 's' : ''}
              </span>
            )
          )}

          {board && (
            <button
              type="button"
              onClick={() =>
                board.paused ? startSprint.mutate() : pauseSprint.mutate()
              }
              className={`px-3 py-1.5 rounded text-sm font-medium transition-colors ${
                board.paused
                  ? 'bg-green-600 hover:bg-green-500 text-white'
                  : 'bg-yellow-600 hover:bg-yellow-500 text-white'
              }`}
              disabled={startSprint.isPending || pauseSprint.isPending}
            >
              {board.paused ? '▶ Start' : '⏸ Pause'}
            </button>
          )}
          <button
            type="button"
            onClick={() => triggerSync.mutate()}
            className="px-3 py-1.5 rounded text-sm font-medium text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
            disabled={triggerSync.isPending}
          >
            🔄 Sync
          </button>
          {board && board.opencode_port > 0 && (
            <button
              type="button"
              onClick={() => window.open(`http://localhost:${board.opencode_port}`, '_blank')}
              className="px-3 py-1.5 rounded text-sm font-medium bg-gray-800 hover:bg-gray-700 text-gray-300 transition-colors"
            >
              💬 Chat
            </button>
          )}
        </div>
      </div>
    </nav>
  )
}
