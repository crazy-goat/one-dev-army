import { Link, useNavigate } from 'react-router'

import { useBoard } from '../api/queries'
import type { Card } from '../api/types'
import { Column } from '../components/board/Column'
import { ProcessingPanel } from '../components/board/ProcessingPanel'
import { SprintProgressPanel } from '../components/board/SprintProgressPanel'

/**
 * Column definitions for the 3-region Kanban board layout.
 *
 * Layout:
 * - Left (15%): Backlog + Blocked (stacked vertically)
 * - Center (70%): 6 pipeline columns (Plan, Code, AI Review, Pipeline, Approve, Merge)
 *                   + Processing Panel below
 * - Right (15%): Done + Failed (stacked vertically)
 */

// Center pipeline columns
const CENTER_COLUMNS = [
  { key: 'plan', label: 'Plan', empty: 'No tickets in planning' },
  { key: 'code', label: 'Code', empty: 'No tickets in coding' },
  { key: 'ai_review', label: 'AI Review', empty: 'No tickets in AI review' },
  { key: 'check_pipeline', label: 'Pipeline', empty: 'No tickets in pipeline' },
  { key: 'approve', label: 'Approve', empty: 'No tickets awaiting approval' },
  { key: 'merge', label: 'Merge', empty: 'No tickets merging' },
] as const

// Right side columns (stacked)
const RIGHT_COLUMNS = [
  { key: 'done', label: 'Done', empty: 'No completed tickets' },
  { key: 'failed', label: 'Failed', empty: 'No failed tickets' },
] as const

const EMPTY_CARDS: Card[] = []

export default function BoardPage() {
  const navigate = useNavigate()
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
          <p className="text-red-400 mb-2">Failed to load board: {error.message}</p>
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

  if (!board) {
    return null
  }

  return (
    <div className="flex flex-col gap-4 p-4 h-[calc(100vh-7rem)]">
      {/* Header row */}
      <div className="flex items-center justify-between flex-shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold text-white">{board.sprint_name || 'Sprint Board'}</h1>
          {board.yolo_mode && (
            <span className="text-xs bg-yellow-600/20 text-yellow-400 border border-yellow-600/30 px-2 py-0.5 rounded">
              YOLO
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {board.total_tickets === 0 && (
            <button
              type="button"
              onClick={() => void navigate('/sprint/plan')}
              className="px-3 py-1.5 rounded text-sm font-medium bg-green-600 hover:bg-green-500 text-white transition-colors"
              title="Plan Sprint — AI-powered ticket selection"
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

      {/* 3-Region Board Layout */}
      <div className="flex-1 min-h-0 grid grid-cols-[15%_1fr_15%] gap-4">
        {/* Left: Backlog + Blocked (stacked) */}
        <div className="flex flex-col gap-4 min-h-0">
          <div className="flex-1 min-h-0">
            <Column
              title="Backlog"
              columnKey="backlog"
              cards={board.columns.backlog ?? EMPTY_CARDS}
              emptyText="No tickets in backlog"
              yoloMode={board.yolo_mode}
            />
          </div>
          <div
            className={
              (board.columns.blocked?.length ?? 0) === 0 ? 'flex-shrink-0' : 'flex-1 min-h-0'
            }
          >
            <Column
              title="Blocked"
              columnKey="blocked"
              cards={board.columns.blocked ?? EMPTY_CARDS}
              emptyText="No blocked tickets"
              yoloMode={board.yolo_mode}
            />
          </div>
        </div>

        {/* Center: Pipeline columns + Processing Panel */}
        <div className="flex flex-col gap-4 min-h-0">
          {/* Pipeline columns (top 40%) */}
          <div className="grid grid-cols-6 gap-3 h-[40%] min-h-0">
            {CENTER_COLUMNS.map(({ key, label, empty }) => (
              <Column
                key={key}
                title={label}
                columnKey={key}
                cards={board.columns[key] ?? EMPTY_CARDS}
                emptyText={empty}
                yoloMode={board.yolo_mode}
              />
            ))}
          </div>

          {/* Processing Panel (bottom 60%) */}
          <div className="h-[60%] min-h-0">
            <ProcessingPanel
              currentTicket={board.current_ticket}
              totalTickets={board.total_tickets}
            />
          </div>
        </div>

        {/* Right: Sprint Progress + Done + Failed (stacked) */}
        <div className="flex flex-col gap-4 min-h-0">
          <SprintProgressPanel
            totalTickets={board.total_tickets}
            completedTickets={(board.columns.done ?? EMPTY_CARDS).length}
          />
          {RIGHT_COLUMNS.map(({ key, label, empty }) => (
            <div key={key} className="flex-1 min-h-0">
              <Column
                title={label}
                columnKey={key}
                cards={board.columns[key] ?? EMPTY_CARDS}
                emptyText={empty}
                yoloMode={board.yolo_mode}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
