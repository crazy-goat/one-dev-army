import { TaskCardComponent } from './TaskCard'
import type { TaskCard } from '../../types/board'

interface KanbanColumnProps {
  title: string
  tasks: TaskCard[]
  variant?: 'default' | 'blocked' | 'plan' | 'code' | 'pipeline' | 'approve' | 'merge' | 'done' | 'failed'
}

const variantBorders: Record<string, string> = {
  default: 'var(--border)',
  blocked: 'var(--red)',
  plan: '#f39c12',
  code: '#3498db',
  pipeline: '#17a2b8',
  approve: '#9b59b6',
  merge: '#6f42c1',
  done: 'var(--green)',
  failed: '#e74c3c',
}

export function KanbanColumn({ title, tasks, variant = 'default' }: KanbanColumnProps) {
  const borderColor = variantBorders[variant] ?? variantBorders.default

  return (
    <div style={{
      background: 'var(--surface)',
      border: `1px solid ${borderColor}`,
      borderRadius: '8px',
      padding: '.75rem',
      overflow: 'hidden',
      display: 'flex',
      flexDirection: 'column',
      minHeight: 0,
    }}>
      <div style={{
        fontSize: '.8rem',
        fontWeight: 600,
        textTransform: 'uppercase',
        letterSpacing: '.05em',
        color: 'var(--muted)',
        marginBottom: '.75rem',
        display: 'flex',
        justifyContent: 'space-between',
        flexShrink: 0,
      }}>
        {title}
        <span style={{
          background: 'var(--border)',
          padding: '.1rem .5rem',
          borderRadius: '10px',
          fontSize: '.75rem',
        }}>
          {tasks.length}
        </span>
      </div>

      <div style={{ overflowY: 'auto', flex: 1 }}>
        {tasks.map((task) => (
          <TaskCardComponent key={task.id} task={task} />
        ))}

        {tasks.length === 0 && (
          <div style={{
            color: 'var(--muted)',
            fontSize: '.85rem',
            textAlign: 'center',
            padding: '2rem',
            fontStyle: 'italic',
          }}>
            No tickets
          </div>
        )}
      </div>
    </div>
  )
}
