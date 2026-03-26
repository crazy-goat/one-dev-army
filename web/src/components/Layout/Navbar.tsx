import { Link, useLocation } from 'react-router-dom'

export function Navbar() {
  const location = useLocation()

  const isActive = (path: string) =>
    location.pathname === path ? 'active' : ''

  return (
    <nav style={{
      background: 'var(--surface)',
      borderBottom: '1px solid var(--border)',
      padding: '.75rem 1.5rem',
      display: 'flex',
      alignItems: 'center',
      gap: '2rem',
    }}>
      <span style={{ fontWeight: 700, fontSize: '1.1rem' }}>ODA</span>
      <div style={{ display: 'flex', gap: '1rem' }}>
        <Link
          to="/"
          className={isActive('/')}
          style={{
            padding: '.4rem .8rem',
            borderRadius: '6px',
            color: location.pathname === '/' ? 'var(--text)' : 'var(--muted)',
            background: location.pathname === '/' ? 'var(--border)' : 'transparent',
            textDecoration: 'none',
            fontSize: '.9rem',
          }}
        >
          Sprint Board
        </Link>
        <Link
          to="/settings"
          className={isActive('/settings')}
          style={{
            padding: '.4rem .8rem',
            borderRadius: '6px',
            color: location.pathname === '/settings' ? 'var(--text)' : 'var(--muted)',
            background: location.pathname === '/settings' ? 'var(--border)' : 'transparent',
            textDecoration: 'none',
            fontSize: '.9rem',
          }}
        >
          Settings
        </Link>
      </div>
    </nav>
  )
}
