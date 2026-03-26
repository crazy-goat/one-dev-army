import { Link, useLocation } from 'react-router'
import {
  useBoard,
  useStartSprint,
  usePauseSprint,
  useTriggerSync,
} from '../../api/queries'

const NAV_LINKS = [
  { path: '/', label: 'Board' },
  { path: '/wizard', label: 'New Issue' },
  { path: '/settings', label: 'Settings' },
] as const

export function Navbar() {
  const location = useLocation()
  const { data: board } = useBoard()
  const startSprint = useStartSprint()
  const pauseSprint = usePauseSprint()
  const triggerSync = useTriggerSync()

  const isActive = (path: string) => location.pathname === path

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

        {/* Right: Sprint info + controls */}
        <div className="flex items-center gap-4">
          {board?.sprint_name && (
            <span className="text-sm text-gray-400">
              🏃 {board.sprint_name}
            </span>
          )}
          {board && (
            <span className="text-xs bg-gray-800 text-gray-300 px-2 py-1 rounded">
              {board.worker_count} worker{board.worker_count !== 1 ? 's' : ''}
            </span>
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
        </div>
      </div>
    </nav>
  )
}
