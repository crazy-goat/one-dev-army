import { useState } from 'react'
import {
  useApproveMergeIssue,
  useDeclineIssue,
  useRetryIssue,
  useRetryFreshIssue,
  useUnblockIssue,
  useProcessIssue,
} from '../../api/queries'

interface ActionButtonsProps {
  issueNumber: number
  status?: string
}

/**
 * Renders context-appropriate action buttons for a task based on its status.
 */
export function ActionButtons({ issueNumber, status }: ActionButtonsProps) {
  const approveMerge = useApproveMergeIssue()
  const decline = useDeclineIssue()
  const retry = useRetryIssue()
  const retryFresh = useRetryFreshIssue()
  const unblock = useUnblockIssue()
  const process = useProcessIssue()

  const [declineOpen, setDeclineOpen] = useState(false)
  const [declineReason, setDeclineReason] = useState('')

  const handleDecline = () => {
    if (!declineReason.trim()) return
    decline.mutate(
      { id: issueNumber, reason: declineReason },
      { onSuccess: () => setDeclineOpen(false) },
    )
  }

  const normalizedStatus = status?.toLowerCase().replace(/[_-]/g, '') ?? ''

  // Determine which actions to show based on status
  const showApprove =
    normalizedStatus.includes('approve') ||
    normalizedStatus.includes('awaitingapproval')
  const showFailed = normalizedStatus.includes('failed')
  const showBlocked = normalizedStatus.includes('blocked')
  const showBacklog = normalizedStatus.includes('backlog')

  if (!showApprove && !showFailed && !showBlocked && !showBacklog) {
    return null
  }

  return (
    <>
      <div className="flex gap-2 flex-wrap">
        {showApprove && (
          <>
            <button
              type="button"
              onClick={() => approveMerge.mutate(issueNumber)}
              disabled={approveMerge.isPending}
              className="px-4 py-2 rounded-lg bg-green-600 hover:bg-green-500 text-white font-medium text-sm transition-colors disabled:opacity-50"
            >
              &check; Approve &amp; Merge
            </button>
            <button
              type="button"
              onClick={() => setDeclineOpen(true)}
              className="px-4 py-2 rounded-lg bg-red-600 hover:bg-red-500 text-white font-medium text-sm transition-colors"
            >
              &times; Decline
            </button>
          </>
        )}

        {showFailed && (
          <>
            <button
              type="button"
              onClick={() => retry.mutate(issueNumber)}
              disabled={retry.isPending}
              className="px-4 py-2 rounded-lg bg-green-600 hover:bg-green-500 text-white font-medium text-sm transition-colors disabled:opacity-50"
            >
              &circlearrowleft; Retry
            </button>
            <button
              type="button"
              onClick={() => retryFresh.mutate(issueNumber)}
              disabled={retryFresh.isPending}
              className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-sm transition-colors disabled:opacity-50"
            >
              &circlearrowleft; Fresh Retry
            </button>
          </>
        )}

        {showBlocked && (
          <button
            type="button"
            onClick={() => unblock.mutate(issueNumber)}
            disabled={unblock.isPending}
            className="px-4 py-2 rounded-lg bg-green-600 hover:bg-green-500 text-white font-medium text-sm transition-colors disabled:opacity-50"
          >
            Unblock
          </button>
        )}

        {showBacklog && (
          <button
            type="button"
            onClick={() => process.mutate(issueNumber)}
            disabled={process.isPending}
            className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-white font-medium text-sm transition-colors disabled:opacity-50"
          >
            {process.isPending ? 'Queuing...' : '\u25B6 Process'}
          </button>
        )}
      </div>

      {/* Decline modal */}
      {declineOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
          onClick={(e) => {
            if (e.target === e.currentTarget) setDeclineOpen(false)
          }}
        >
          <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-[500px] max-w-[90vw]">
            <h3 className="text-white font-semibold mb-1">
              Decline #{issueNumber}
            </h3>
            <label className="text-sm font-semibold text-gray-300 block mb-1 mt-4">
              What needs to be fixed?
            </label>
            <textarea
              value={declineReason}
              onChange={(e) => setDeclineReason(e.target.value)}
              rows={5}
              className="w-full p-2 border border-gray-700 rounded-md bg-gray-800 text-gray-200 text-sm resize-y focus:outline-none focus:border-blue-500"
              placeholder="Describe what's wrong and what the AI should fix..."
            />
            <div className="flex gap-2 justify-end mt-4">
              <button
                type="button"
                onClick={() => setDeclineOpen(false)}
                className="px-3 py-1.5 text-sm rounded bg-gray-700 hover:bg-gray-600 text-gray-300 transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleDecline}
                disabled={decline.isPending || !declineReason.trim()}
                className="px-3 py-1.5 text-sm rounded bg-red-600 hover:bg-red-500 text-white font-medium transition-colors disabled:opacity-50"
              >
                Decline &amp; Send Back
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
