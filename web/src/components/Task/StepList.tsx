import type { TaskStep } from '../../types/task'

interface StepListProps {
  steps: TaskStep[]
  isActive: boolean
  issueNumber: number
}

function formatTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleTimeString()
}

function formatDuration(start?: string, end?: string): string {
  if (!start || !end) return ''
  const ms = new Date(end).getTime() - new Date(start).getTime()
  const secs = Math.floor(ms / 1000)
  if (secs < 60) return `${secs}s`
  const mins = Math.floor(secs / 60)
  return `${mins}m ${secs % 60}s`
}

const statusColors: Record<string, string> = {
  pending: 'var(--muted)',
  running: 'var(--accent)',
  done: 'var(--green)',
  failed: 'var(--red)',
}

export function StepList({ steps, isActive }: StepListProps) {
  if (steps.length === 0 && !isActive) {
    return (
      <div style={{ color: 'var(--muted)', padding: '2rem', textAlign: 'center' }}>
        No processing steps recorded for this task.
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '.5rem' }}>
      {steps.map((step) => (
        <div
          key={step.id}
          style={{
            background: 'var(--surface)',
            border: '1px solid var(--border)',
            borderRadius: '8px',
            padding: '1rem',
            borderLeft: `3px solid ${statusColors[step.status] ?? 'var(--border)'}`,
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '.5rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}>
              <span style={{
                width: 8,
                height: 8,
                borderRadius: '50%',
                background: statusColors[step.status] ?? 'var(--muted)',
                display: 'inline-block',
                animation: step.status === 'running' ? 'pulse 1.5s ease-in-out infinite' : 'none',
              }} />
              <span style={{ fontWeight: 600, fontSize: '.9rem' }}>{step.step_name}</span>
              <span style={{
                fontSize: '.7rem',
                padding: '.1rem .4rem',
                borderRadius: '4px',
                background: 'var(--bg)',
                color: statusColors[step.status] ?? 'var(--muted)',
                textTransform: 'uppercase',
              }}>
                {step.status}
              </span>
            </div>
            <div style={{ fontSize: '.75rem', color: 'var(--muted)' }}>
              {formatTime(step.started_at)}
              {step.finished_at && ` (${formatDuration(step.started_at, step.finished_at)})`}
            </div>
          </div>

          {step.response && (
            <details style={{ marginTop: '.5rem' }}>
              <summary style={{ cursor: 'pointer', fontSize: '.8rem', color: 'var(--muted)' }}>
                Response
              </summary>
              <pre style={{
                marginTop: '.5rem',
                padding: '.75rem',
                background: 'var(--bg)',
                borderRadius: '4px',
                fontSize: '.8rem',
                overflow: 'auto',
                maxHeight: '300px',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}>
                {step.response}
              </pre>
            </details>
          )}

          {step.error_msg && (
            <div style={{
              marginTop: '.5rem',
              padding: '.5rem .75rem',
              background: 'rgba(248,81,73,0.1)',
              border: '1px solid rgba(248,81,73,0.2)',
              borderRadius: '4px',
              fontSize: '.8rem',
              color: 'var(--red)',
            }}>
              {step.error_msg}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
