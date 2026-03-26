import { Component, ReactNode } from 'react'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  hasError: boolean
  error?: Error
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback
      }

      return (
        <div style={{
          padding: '2rem',
          textAlign: 'center',
          color: 'var(--red)',
        }}>
          <h2 style={{ marginBottom: '.5rem' }}>Something went wrong</h2>
          <p style={{ color: 'var(--muted)', fontSize: '.9rem' }}>
            {this.state.error?.message ?? 'An unexpected error occurred'}
          </p>
          <button
            className="btn"
            onClick={() => this.setState({ hasError: false, error: undefined })}
            style={{ marginTop: '1rem' }}
          >
            Try Again
          </button>
        </div>
      )
    }

    return this.props.children
  }
}
