import { Link } from 'react-router-dom'
import type { TaskInfo } from '../../types/board'

interface ProcessingPanelProps {
  currentTicket?: TaskInfo
}

export function ProcessingPanel({ currentTicket }: ProcessingPanelProps) {
  const isActive = !!currentTicket

  return (
    <div style={{
      background: isActive ? 'rgba(52,152,219,0.08)' : 'rgba(108,117,125,0.08)',
      border: `1px solid ${isActive ? 'rgba(52,152,219,0.2)' : 'rgba(108,117,125,0.2)'}`,
      borderRadius: '8px',
      padding: '.75rem 1rem',
      display: 'flex',
      alignItems: 'center',
      minHeight: '80px',
      flexShrink: 0,
    }}>
      <span style={{
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: isActive ? '#3498db' : '#6c757d',
        marginRight: '1rem',
        animation: isActive ? 'pulse 2s infinite' : 'none',
        display: 'inline-block',
        flexShrink: 0,
      }} />

      {currentTicket ? (
        <div>
          <Link
            to={`/task/${currentTicket.number}`}
            style={{ textDecoration: 'none', color: 'var(--text)' }}
          >
            <div style={{ color: 'var(--muted)', fontSize: '.8rem' }}>
              #{currentTicket.number}
            </div>
            <div style={{ fontSize: '1.1rem', fontWeight: 600 }}>
              {currentTicket.title}
            </div>
          </Link>
          <div style={{ marginTop: '.5rem', display: 'flex', gap: '.5rem' }}>
            {currentTicket.priority && (
              <span style={{
                fontSize: '.7rem',
                padding: '.15rem .5rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
              }}>
                {currentTicket.priority}
              </span>
            )}
            {currentTicket.type && (
              <span style={{
                fontSize: '.7rem',
                padding: '.15rem .5rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
              }}>
                {currentTicket.type}
              </span>
            )}
            {currentTicket.size && (
              <span style={{
                fontSize: '.7rem',
                padding: '.15rem .5rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
              }}>
                size:{currentTicket.size}
              </span>
            )}
          </div>
        </div>
      ) : (
        <span style={{ color: 'var(--muted)', fontSize: '.85rem' }}>
          No active ticket &mdash; Worker ready
        </span>
      )}
    </div>
  )
}
