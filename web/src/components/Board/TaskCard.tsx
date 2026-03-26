import { Link } from 'react-router-dom'
import type { TaskCard } from '../../types/board'

interface TaskCardProps {
  task: TaskCard
}

const labelIcons: Record<string, string> = {
  'type:feature': '\u2728',
  'feature': '\u2728',
  'enhancement': '\u2728',
  'type:bug': '\uD83D\uDC1B',
  'bug': '\uD83D\uDC1B',
  'size:S': '\uD83D\uDC1C',
  'size:M': '\uD83D\uDC15',
  'size:L': '\uD83D\uDC18',
  'size:XL': '\uD83E\uDD95',
  'priority:high': '\uD83D\uDD34',
  'priority:medium': '\uD83D\uDFE1',
  'priority:low': '\uD83D\uDFE2',
}

export function TaskCardComponent({ task }: TaskCardProps) {
  return (
    <div style={{
      background: 'var(--bg)',
      border: '1px solid var(--border)',
      borderRadius: '6px',
      padding: '.6rem .75rem',
      marginBottom: '.5rem',
      fontSize: '.85rem',
    }}>
      <div style={{ color: 'var(--muted)', fontSize: '.75rem' }}>
        <Link to={`/task/${task.id}`} style={{ color: 'var(--muted)', textDecoration: 'none' }}>
          #{task.id}
        </Link>
        {task.pr_url && (
          <a
            href={task.pr_url}
            target="_blank"
            rel="noopener noreferrer"
            style={{ marginLeft: '.5rem', color: 'var(--accent)', fontSize: '.7rem' }}
          >
            PR
          </a>
        )}
      </div>
      <div style={{ marginTop: '.2rem' }}>
        <Link to={`/task/${task.id}`} style={{ color: 'var(--text)', textDecoration: 'none' }}>
          {task.title}
        </Link>
      </div>

      {task.labels.length > 0 && (
        <div style={{ display: 'flex', gap: '.25rem', marginTop: '.3rem', flexWrap: 'wrap' }}>
          {task.labels.map((label) => {
            const icon = labelIcons[label]
            return (
              <span
                key={label}
                title={label}
                style={{
                  fontSize: '.65rem',
                  padding: '.1rem .4rem',
                  borderRadius: '4px',
                  background: 'var(--surface)',
                  border: '1px solid var(--border)',
                }}
              >
                {icon ? `${icon} ` : ''}{label}
              </span>
            )
          })}
        </div>
      )}

      {task.assignee && (
        <div style={{ color: 'var(--accent)', fontSize: '.75rem', marginTop: '.3rem' }}>
          @{task.assignee}
        </div>
      )}
    </div>
  )
}
