import { useState } from 'react'
import { Link } from 'react-router'

import {
  useApproveMergeIssue,
  useBlockIssue,
  useDeclineIssue,
  useRetryIssue,
  useRetryFreshIssue,
  useUnblockIssue,
  useProcessIssue,
} from '../../api/queries'
import type { Card } from '../../api/types'

/** Maps known GitHub labels to emoji icons. */
function labelIcon(label: string): string | null {
  const lower = label.toLowerCase()
  if (
    lower === 'type:feature' ||
    lower === 'feature' ||
    lower === 'enhancement'
  )
    {return '\u2728'}
  if (lower === 'type:bug' || lower === 'bug') {return '\uD83D\uDC1B'}
  if (lower === 'type:docs') {return '\uD83D\uDCDA'}
  if (lower === 'type:refactor') {return '\uD83D\uDD27'}
  if (lower === 'size:s') {return '\uD83D\uDC1C'}
  if (lower === 'size:m') {return '\uD83D\uDC15'}
  if (lower === 'size:l') {return '\uD83D\uDC18'}
  if (lower === 'size:xl') {return '\uD83E\uDD95'}
  if (lower === 'priority:high') {return '\uD83D\uDD34'}
  if (lower === 'priority:medium') {return '\uD83D\uDFE1'}
  if (lower === 'priority:low') {return '\uD83D\uDFE2'}
  return null
}

interface TaskCardProps {
  card: Card
  /** Human-readable column label (e.g. "Backlog", "AI Review"). */
  column: string
  /** Snake_case API column key (e.g. "backlog", "ai_review"). */
  columnKey: string
}

export function TaskCard({ card, column, columnKey }: TaskCardProps) {
  const approveMerge = useApproveMergeIssue()
  const decline = useDeclineIssue()
  const retry = useRetryIssue()
  const retryFresh = useRetryFreshIssue()
  const unblock = useUnblockIssue()
  const block = useBlockIssue()
  const process = useProcessIssue()

  const [declineOpen, setDeclineOpen] = useState(false)
  const [declineReason, setDeclineReason] = useState('')
  const [processConfirmOpen, setProcessConfirmOpen] = useState(false)

  const iconLabels = card.labels.filter((l) => labelIcon(l) !== null)
  const textLabels = card.labels.filter((l) => labelIcon(l) === null)

  const handleDecline = () => {
    if (!declineReason.trim()) {return}
    decline.mutate(
      { id: card.id, reason: declineReason },
      { onSuccess: () => setDeclineOpen(false) },
    )
  }

  const handleProcessConfirm = () => {
    process.mutate(card.id, {
      onSuccess: () => setProcessConfirmOpen(false),
    })
  }

  // Column-specific card border colors (keyed by display label)
  const borderColor: Record<string, string> = {
    Blocked: 'border-red-500/50',
    Failed: 'border-red-500/60',
    Plan: 'border-yellow-500/40',
    Code: 'border-blue-500/40',
    'AI Review': 'border-cyan-500/40',
    Pipeline: 'border-teal-500/40',
    Approve: 'border-purple-500/40',
    Merge: 'border-violet-500/40',
  }

  const bgTint: Record<string, string> = {
    Failed: 'bg-red-500/5',
    Plan: 'bg-yellow-500/5',
    Code: 'bg-blue-500/5',
    Approve: 'bg-purple-500/5',
    Pipeline: 'bg-teal-500/5',
    Merge: 'bg-violet-500/5',
  }

  return (
    <div
      className={`rounded-lg border p-3 text-sm relative ${
        borderColor[column] ?? 'border-gray-800'
      } ${bgTint[column] ?? 'bg-gray-950'}`}
    >
      {/* Emoji icons top-right */}
      {iconLabels.length > 0 && (
        <div className="absolute top-2 right-2 flex gap-0.5 flex-wrap justify-end max-w-[5rem]">
          {iconLabels.map((l) => (
            <span key={l} className="text-base leading-none" title={l}>
              {labelIcon(l)}
            </span>
          ))}
        </div>
      )}

      {/* Issue number */}
      <Link
        to={`/task/${String(card.id)}`}
        className="text-xs text-gray-500 hover:text-blue-400 transition-colors"
      >
        #{card.id}
      </Link>

      {/* Title */}
      <div className="mt-0.5 mr-16 text-gray-200 leading-snug">
        <Link
          to={`/task/${String(card.id)}`}
          className="hover:text-white transition-colors"
        >
          {card.title}
        </Link>
      </div>

      {/* PR link */}
      {card.pr_url !== null && card.pr_url !== undefined && (
        <div className="mt-1">
          <a
            href={card.pr_url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-purple-400 text-xs font-semibold hover:underline"
          >
            View PR &rarr;
          </a>
        </div>
      )}

      {/* Merged / Closed badge for Done column */}
      {columnKey === 'done' && (
        <div className="mt-1">
          {card.is_merged ? (
            <span className="inline-flex items-center gap-1 bg-violet-600 text-white text-[0.65rem] px-1.5 py-0.5 rounded">
              &check; Merged
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 bg-gray-600 text-white text-[0.65rem] px-1.5 py-0.5 rounded">
              &times; Closed
            </span>
          )}
        </div>
      )}

      {/* Text labels */}
      {textLabels.length > 0 && (
        <div className="flex gap-1 flex-wrap mt-1.5">
          {textLabels.map((l) => (
            <span
              key={l}
              className="text-[0.65rem] px-1.5 py-0.5 rounded bg-gray-800 border border-gray-700 text-gray-400"
            >
              {l}
            </span>
          ))}
        </div>
      )}

      {/* Assignee */}
      {card.assignee !== null && card.assignee !== undefined && (
        <div className="text-xs text-blue-400 mt-1">@{card.assignee}</div>
      )}

      {/* Worker */}
      {card.worker !== null && card.worker !== undefined && (
        <div className="text-xs text-blue-400 mt-1">{card.worker}</div>
      )}

      {/* Action buttons */}
      <div className="flex gap-1.5 mt-2 flex-wrap">
        {columnKey === 'approve' && (
          <>
            <button
              type="button"
              onClick={() => approveMerge.mutate(card.id)}
              disabled={approveMerge.isPending}
              className="px-2 py-1 text-xs rounded bg-green-600 hover:bg-green-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              &check; Approve+Merge
            </button>
            <button
              type="button"
              onClick={() => setDeclineOpen(true)}
              className="px-2 py-1 text-xs rounded bg-red-600 hover:bg-red-500 text-white font-medium transition-colors"
            >
              &times; Decline
            </button>
          </>
        )}

        {columnKey === 'failed' && (
          <>
            <button
              type="button"
              onClick={() => retry.mutate(card.id)}
              disabled={retry.isPending}
              className="px-2 py-1 text-xs rounded bg-green-600 hover:bg-green-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              ↺ Retry
            </button>
            <button
              type="button"
              onClick={() => retryFresh.mutate(card.id)}
              disabled={retryFresh.isPending}
              className="px-2 py-1 text-xs rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              ↺ Fresh Retry
            </button>
          </>
        )}

        {columnKey === 'blocked' && (
          <button
            type="button"
            onClick={() => unblock.mutate(card.id)}
            disabled={unblock.isPending}
            className="px-2 py-1 text-xs rounded bg-green-600 hover:bg-green-500 text-white font-medium transition-colors disabled:opacity-50"
          >
            Unblock
          </button>
        )}

        {/* MISSING 1: Block button in Backlog column */}
        {columnKey === 'backlog' && (
          <>
            <button
              type="button"
              onClick={() => setProcessConfirmOpen(true)}
              disabled={process.isPending}
              className="px-2 py-1 text-xs rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              {process.isPending ? 'Queuing...' : '\u25B6 Process'}
            </button>
            <button
              type="button"
              onClick={() => block.mutate(card.id)}
              disabled={block.isPending}
              className="px-2 py-1 text-xs rounded bg-red-600 hover:bg-red-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              {block.isPending ? 'Blocking...' : '\uD83D\uDEAB Block'}
            </button>
          </>
        )}
      </div>

      {/* Decline modal */}
      {declineOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
          onClick={(e) => {
            if (e.target === e.currentTarget) {setDeclineOpen(false)}
          }}
        >
          <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-[500px] max-w-[90vw]">
            <h3 className="text-white font-semibold mb-1">
              Decline #{card.id}
            </h3>
            <p className="text-gray-400 text-sm mb-4">{card.title}</p>
            <label className="text-sm font-semibold text-gray-300 block mb-1">
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

      {/* MISSING 13: Process confirmation modal */}
      {processConfirmOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center"
          onClick={(e) => {
            if (e.target === e.currentTarget) {setProcessConfirmOpen(false)}
          }}
        >
          <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-[400px] max-w-[90vw]">
            <h3 className="text-white font-semibold mb-1">
              Process #{card.id}?
            </h3>
            <p className="text-gray-400 text-sm mb-4">{card.title}</p>
            <p className="text-gray-500 text-sm mb-4">
              This will queue the ticket for automated processing through the
              full pipeline (plan, code, review, merge).
            </p>
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => setProcessConfirmOpen(false)}
                className="px-3 py-1.5 text-sm rounded bg-gray-700 hover:bg-gray-600 text-gray-300 transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleProcessConfirm}
                disabled={process.isPending}
                className="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
              >
                {process.isPending ? 'Queuing...' : 'Confirm & Process'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
