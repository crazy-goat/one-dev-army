import { Link } from 'react-router'
import { useBoard } from '../api/queries'
import type { Card } from '../api/types'
import { Column } from '../components/board/Column'
import { ProcessingPanel } from '../components/board/ProcessingPanel'

/**
 * Ordered column definitions for the Kanban board.
 * `key` must match the snake_case keys returned by the Go API (`columns` map).
 * `label` is the human-readable display name.
 */
const COLUMNS = [
  { key: 'backlog', label: 'Backlog', empty: 'No tickets in backlog' },
  { key: 'blocked', label: 'Blocked', empty: 'No blocked tickets' },
  { key: 'plan', label: 'Plan', empty: 'No tickets in planning' },
  { key: 'code', label: 'Code', empty: 'No tickets in coding' },
  { key: 'ai_review', label: 'AI Review', empty: 'No tickets in AI review' },
  { key: 'check_pipeline', label: 'Pipeline', empty: 'No tickets in pipeline' },
  { key: 'approve', label: 'Approve', empty: 'No tickets awaiting approval' },
  { key: 'merge', label: 'Merge', empty: 'No tickets merging' },
  { key: 'done', label: 'Done', empty: 'No completed tickets' },
  { key: 'failed', label: 'Failed', empty: 'No failed tickets' },
] as const

const EMPTY_CARDS: Card[] = []

export default function BoardPage() {
  const { data: board, isLoading, error } = useBoard()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-2 border-gray-700 border-t-blue-500 rounded-full animate-spin" />
          <span className="text-gray-500 text-sm">Loading board...</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="text-center">
          <p className="text-red-400 mb-2">
            Failed to load board: {error.message}
          </p>
          <button
            type="button"
            onClick={() => window.location.reload()}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  if (!board) return null

  return (
    <div className="flex flex-col gap-4 p-4 h-[calc(100vh-7rem)]">
      {/* Header row */}
      <div className="flex items-center justify-between flex-shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold text-white">
            {board.sprint_name || 'Sprint Board'}
          </h1>
          {board.yolo_mode && (
            <span className="text-xs bg-yellow-600/20 text-yellow-400 border border-yellow-600/30 px-2 py-0.5 rounded">
              YOLO
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {/* MISSING 2: Plan Sprint button (placeholder — v2 API not yet available) */}
          {board.can_plan_sprint && (
            <button
              type="button"
              className="px-3 py-1.5 rounded text-sm font-medium bg-blue-600 hover:bg-blue-500 text-white transition-colors"
              title="Plan Sprint — assigns backlog tickets to the current sprint"
              onClick={() => {
                // Plan sprint API doesn't exist in v2 yet; show alert as placeholder
                window.alert(
                  'Plan Sprint is not yet available in the SPA. Use the classic dashboard for now.',
                )
              }}
            >
              Plan Sprint
            </button>
          )}
          {board.can_close_sprint && (
            <Link
              to="/sprint/close"
              className="px-3 py-1.5 rounded text-sm font-medium bg-green-600 hover:bg-green-500 text-white transition-colors"
            >
              Close Sprint
            </Link>
          )}
        </div>
      </div>

      {/* Processing panel */}
      <div className="flex-shrink-0">
        <ProcessingPanel
          currentTicket={board.current_ticket}
          totalTickets={board.total_tickets}
        />
      </div>

      {/* Kanban columns — horizontal scroll */}
      <div className="flex-1 min-h-0 overflow-x-auto">
        <div className="grid grid-cols-10 gap-3 min-w-[1600px] h-full">
          {COLUMNS.map(({ key, label, empty }) => (
            <Column
              key={key}
              title={label}
              columnKey={key}
              cards={board.columns[key] ?? EMPTY_CARDS}
              emptyText={empty}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
