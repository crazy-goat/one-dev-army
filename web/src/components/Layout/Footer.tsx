import { useStore } from '../../store'

export function Footer() {
  const wsConnected = useStore((s) => s.wsConnected)
  const workerStatus = useStore((s) => s.workerStatus)

  const switchToOld = () => {
    document.cookie = 'oda_dashboard_version=old; path=/'
    window.location.href = '/'
  }

  return (
    <footer style={{
      background: 'var(--surface)',
      borderTop: '1px solid var(--border)',
      padding: '.5rem 1.5rem',
      display: 'flex',
      alignItems: 'center',
      gap: '.75rem',
      fontSize: '.8rem',
      color: 'var(--muted)',
    }}>
      {/* WebSocket status */}
      <span
        title={wsConnected ? 'Connected' : 'Disconnected'}
        style={{ color: wsConnected ? 'var(--green)' : 'var(--red)', fontSize: '1rem' }}
      >
        {wsConnected ? '\u2713' : '\u2717'}
      </span>

      {/* Worker status */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: '.5rem',
        padding: '.25rem .5rem',
        borderRadius: '4px',
        background: 'var(--bg)',
        border: '1px solid var(--border)',
      }}>
        <span style={{
          width: 10,
          height: 10,
          borderRadius: '50%',
          background: workerStatus?.active ? 'var(--green)' : 'var(--muted)',
          animation: workerStatus?.active ? 'pulse 1.5s ease-in-out infinite' : 'none',
          display: 'inline-block',
        }} />
        <span style={{ fontSize: '.75rem' }}>
          {workerStatus?.active
            ? `#${workerStatus.issue_id} ${workerStatus.step}`
            : 'Worker idle'}
        </span>
      </div>

      {/* Spacer */}
      <div style={{ flex: 1 }} />

      {/* Version switch */}
      <span style={{ fontSize: '.75rem' }}>New Dashboard (Beta)</span>
      <button
        onClick={switchToOld}
        className="btn"
        style={{ fontSize: '.75rem', padding: '.2rem .5rem' }}
      >
        Back to Old Version
      </button>
    </footer>
  )
}
