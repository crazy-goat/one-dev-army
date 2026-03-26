import { useState } from 'react'
import { tasksAPI } from '../../api/tasks'

interface TaskActionsProps {
  issueNumber: number
  status: string
  onActionComplete: () => void
}

export function TaskActions({ issueNumber, status, onActionComplete }: TaskActionsProps) {
  const [loading, setLoading] = useState(false)
  const [declineReason, setDeclineReason] = useState('')
  const [showDecline, setShowDecline] = useState(false)

  const handleAction = async (action: string, body?: Record<string, string>) => {
    setLoading(true)
    try {
      await tasksAPI.action(issueNumber, action, body)
      onActionComplete()
    } catch (err) {
      console.error(`Action ${action} failed:`, err)
    } finally {
      setLoading(false)
    }
  }

  // Show different actions based on status
  const isApproveColumn = status === 'Approve' || status === 'stage:awaiting-approval'
  const isFailedColumn = status === 'Failed' || status === 'stage:failed'
  const isBlockedColumn = status === 'Blocked' || status === 'stage:blocked'

  return (
    <div style={{ display: 'flex', gap: '.5rem', flexWrap: 'wrap' }}>
      {isApproveColumn && (
        <>
          <button
            className="btn btn-success"
            onClick={() => handleAction('merge')}
            disabled={loading}
          >
            Approve & Merge
          </button>
          <button
            className="btn btn-danger"
            onClick={() => setShowDecline(!showDecline)}
            disabled={loading}
          >
            Decline
          </button>
        </>
      )}

      {isFailedColumn && (
        <>
          <button
            className="btn"
            onClick={() => handleAction('retry')}
            disabled={loading}
          >
            Retry
          </button>
          <button
            className="btn"
            onClick={() => handleAction('retry-fresh')}
            disabled={loading}
          >
            Retry Fresh
          </button>
        </>
      )}

      {isBlockedColumn && (
        <button
          className="btn"
          onClick={() => handleAction('unblock')}
          disabled={loading}
        >
          Unblock
        </button>
      )}

      {!isApproveColumn && !isFailedColumn && !isBlockedColumn && (
        <>
          <button
            className="btn"
            onClick={() => handleAction('block')}
            disabled={loading}
          >
            Block
          </button>
          <button
            className="btn btn-danger"
            onClick={() => handleAction('reject')}
            disabled={loading}
          >
            Reject to Backlog
          </button>
        </>
      )}

      {showDecline && (
        <div style={{
          width: '100%',
          marginTop: '.5rem',
          display: 'flex',
          gap: '.5rem',
          alignItems: 'flex-start',
        }}>
          <textarea
            value={declineReason}
            onChange={(e) => setDeclineReason(e.target.value)}
            placeholder="Reason for declining..."
            style={{
              flex: 1,
              padding: '.5rem',
              background: 'var(--bg)',
              border: '1px solid var(--border)',
              borderRadius: '4px',
              color: 'var(--text)',
              fontSize: '.85rem',
              resize: 'vertical',
              minHeight: '60px',
            }}
          />
          <button
            className="btn btn-danger"
            onClick={() => {
              handleAction('decline', { reason: declineReason })
              setShowDecline(false)
              setDeclineReason('')
            }}
            disabled={loading}
          >
            Send
          </button>
        </div>
      )}
    </div>
  )
}
