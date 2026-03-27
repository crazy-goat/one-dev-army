import { Link, useLocation } from 'react-router'

import { useBoard } from '../../api/queries'

const NAV_LINKS = [
  { path: '/', label: 'Sprint Board' },
  { path: '/settings', label: 'Settings' },
] as const

export function Navbar() {
  const location = useLocation()
  const { data: board } = useBoard()

  const isActive = (path: string) => location.pathname === path

  return (
    <nav className="bg-gray-900 border-b border-gray-800 px-4 py-3">
      <div className="flex items-center justify-between w-full">
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

        {/* Right: Chat + New Issue */}
        <div className="flex items-center gap-3">
          {board && board.opencode_port > 0 && (
            <button
              type="button"
              onClick={() => window.open(`http://localhost:${board.opencode_port}`, '_blank')}
              className="px-3 py-1.5 rounded text-sm font-medium text-gray-300 hover:text-white transition-colors"
            >
              💬 Chat
            </button>
          )}
          <Link
            to="/wizard"
            className="px-4 py-1.5 rounded text-sm font-medium bg-blue-600 hover:bg-blue-500 text-white transition-colors"
          >
            + New Issue
          </Link>
        </div>
      </div>
    </nav>
  )
}
