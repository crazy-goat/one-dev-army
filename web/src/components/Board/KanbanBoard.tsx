import useSWR from 'swr'
import { boardAPI } from '../../api/board'
import { KanbanColumn } from './KanbanColumn'
import { ProcessingPanel } from './ProcessingPanel'
import { SprintControls } from './SprintControls'

export function KanbanBoard() {
  const { data: board, error, isLoading } = useSWR('board', boardAPI.getBoard, {
    refreshInterval: 5000,
  })

  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', color: 'var(--muted)' }}>
        Loading board...
      </div>
    )
  }

  if (error) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', color: 'var(--red)' }}>
        Error loading board: {error.message}
      </div>
    )
  }

  if (!board) return null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: '1rem' }}>
      <SprintControls />

      <div style={{
        display: 'grid',
        gridTemplateColumns: '15% 1fr 15%',
        gap: '1rem',
        flex: 1,
        minHeight: 0,
      }}>
        {/* Left column: Backlog + Blocked */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem', overflow: 'hidden' }}>
          <KanbanColumn title="Backlog" tasks={board.backlog} />
          <KanbanColumn title="Blocked" tasks={board.blocked} variant="blocked" />
        </div>

        {/* Center: Pipeline columns + Processing panel */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', overflow: 'hidden' }}>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(6, 1fr)',
            gap: '.75rem',
            flex: 1,
            minHeight: 0,
          }}>
            <KanbanColumn title="Plan" tasks={board.plan} variant="plan" />
            <KanbanColumn title="Code" tasks={board.code} variant="code" />
            <KanbanColumn title="AI Review" tasks={board.ai_review} />
            <KanbanColumn title="Check Pipeline" tasks={board.check_pipeline} variant="pipeline" />
            <KanbanColumn title="Approve" tasks={board.approve} variant="approve" />
            <KanbanColumn title="Merge" tasks={board.merge} variant="merge" />
          </div>
          <ProcessingPanel currentTicket={board.current_ticket} />
        </div>

        {/* Right column: Done + Failed */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem', overflow: 'hidden' }}>
          <KanbanColumn title="Done" tasks={board.done} variant="done" />
          <KanbanColumn title="Failed" tasks={board.failed} variant="failed" />
        </div>
      </div>
    </div>
  )
}
