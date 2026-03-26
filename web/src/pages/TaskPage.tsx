import { Link, useParams } from 'react-router'
import { useIssue, useIssueSteps } from '../api/queries'
import { StepList } from '../components/task/StepList'
import { LogViewer } from '../components/task/LogViewer'
import { ActionButtons } from '../components/task/ActionButtons'

export default function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const issueNumber = Number(id)

  const {
    data: issue,
    isLoading: issueLoading,
    error: issueError,
  } = useIssue(issueNumber)
  const { data: steps } = useIssueSteps(issueNumber)

  if (issueLoading) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-2 border-gray-700 border-t-blue-500 rounded-full animate-spin" />
          <span className="text-gray-500 text-sm">Loading task...</span>
        </div>
      </div>
    )
  }

  if (issueError) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="text-center">
          <p className="text-red-400 mb-2">
            Failed to load task: {issueError.message}
          </p>
          <Link
            to="/"
            className="text-blue-400 hover:text-blue-300 text-sm transition-colors"
          >
            &larr; Back to board
          </Link>
        </div>
      </div>
    )
  }

  if (!issue) return null

  return (
    <div className="max-w-4xl mx-auto p-4">
      {/* Header */}
      <div className="mb-6">
        <Link
          to="/"
          className="text-gray-500 hover:text-gray-300 text-sm transition-colors"
        >
          &larr; Back to board
        </Link>
        <div className="flex items-start justify-between mt-2 gap-4">
          <h1 className="text-xl font-bold text-white">
            <span className="text-gray-500 font-normal">
              #{issue.issue_number}
            </span>{' '}
            {issue.issue_title}
          </h1>
          {issue.is_active && issue.status && (
            <span className="flex-shrink-0 px-3 py-1 rounded bg-green-600 text-white text-sm font-semibold">
              {issue.status}
            </span>
          )}
        </div>
      </div>

      {/* Action buttons */}
      <div className="mb-6">
        <ActionButtons
          issueNumber={issue.issue_number}
          status={issue.status}
        />
      </div>

      {/* Live output for active tasks */}
      {issue.is_active && (
        <div className="mb-6">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-gray-500 mb-3">
            Live Output
          </h2>
          <LogViewer issueNumber={issue.issue_number} />
        </div>
      )}

      {/* Pipeline steps */}
      <div>
        <h2 className="text-sm font-semibold uppercase tracking-wider text-gray-500 mb-3">
          Pipeline Steps
        </h2>
        <StepList steps={steps ?? issue.steps} />
      </div>
    </div>
  )
}
