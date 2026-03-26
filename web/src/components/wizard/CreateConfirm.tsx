import type { CreatedIssue } from '../../api/types'

interface CreateConfirmProps {
  createdIssues: CreatedIssue[]
  onCreateAnother: () => void
}

export function CreateConfirm({
  createdIssues,
  onCreateAnother,
}: CreateConfirmProps) {
  const hasErrors = createdIssues.some((i) => !i.success)
  const epic = createdIssues.find((i) => i.is_epic)
  const subtasks = createdIssues.filter((i) => !i.is_epic)
  const isSingle = createdIssues.length === 1

  return (
    <div>
      <h2 className="text-xl font-bold text-white mb-6">
        {'\u2705'}{' '}
        {isSingle ? 'Issue Created Successfully' : 'Issues Created Successfully'}
      </h2>

      {hasErrors && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-4 text-sm text-red-400">
          <strong>{'\u26A0\uFE0F'} Some sub-tasks failed to create.</strong>{' '}
          Check the list below for details.
        </div>
      )}

      {/* Single issue or Epic */}
      {isSingle && createdIssues[0] ? (
        <div className="mb-6">
          <div className="bg-gray-900 border-2 border-blue-500/40 rounded-lg p-4 bg-gradient-to-br from-gray-900 to-blue-500/5">
            <a
              href={createdIssues[0].url}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-3 text-gray-200 hover:text-white transition-colors"
            >
              <span className="text-blue-400 font-semibold min-w-[3rem]">
                #{createdIssues[0].number}
              </span>
              <span className="flex-1">{createdIssues[0].title}</span>
            </a>
          </div>
        </div>
      ) : (
        <>
          {/* Epic */}
          {epic && (
            <div className="mb-4">
              <h3 className="text-sm font-semibold text-gray-400 mb-2">
                {'\uD83D\uDCCB'} Epic
              </h3>
              <div className="bg-gray-900 border-2 border-blue-500/40 rounded-lg p-4 bg-gradient-to-br from-gray-900 to-blue-500/5">
                <a
                  href={epic.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-3 text-gray-200 hover:text-white transition-colors"
                >
                  <span className="text-blue-400 font-semibold min-w-[3rem]">
                    #{epic.number}
                  </span>
                  <span className="flex-1">{epic.title}</span>
                  <span className="bg-blue-600 text-white text-xs px-2 py-0.5 rounded font-semibold">
                    EPIC
                  </span>
                </a>
              </div>
            </div>
          )}

          {/* Subtasks */}
          {subtasks.length > 0 && (
            <div className="mb-6">
              <h3 className="text-sm font-semibold text-gray-400 mb-2">
                {'\uD83D\uDCDD'} Sub-tasks ({subtasks.length})
              </h3>
              <div className="flex flex-col gap-2">
                {subtasks.map((issue, i) => (
                  <div
                    key={issue.number || `err-${String(i)}`}
                    className={`bg-gray-900 border rounded-lg p-3 ${
                      issue.success
                        ? 'border-gray-800'
                        : 'border-red-500/30 bg-red-500/5'
                    }`}
                  >
                    {issue.success ? (
                      <a
                        href={issue.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-3 text-gray-200 hover:text-white transition-colors"
                      >
                        <span className="text-blue-400 font-semibold min-w-[3rem]">
                          #{issue.number}
                        </span>
                        <span className="flex-1">{issue.title}</span>
                      </a>
                    ) : (
                      <div className="flex items-center gap-3 text-red-400">
                        <span className="font-semibold min-w-[3rem]">
                          {'\u274C'}
                        </span>
                        <span className="flex-1">{issue.title}</span>
                        {issue.error && (
                          <span className="text-sm ml-auto">{issue.error}</span>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}

      {/* Actions */}
      <div className="flex justify-between items-center">
        <a
          href="/new/"
          className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors"
        >
          Close Wizard
        </a>
        <button
          type="button"
          onClick={onCreateAnother}
          className="px-4 py-2 bg-green-600 hover:bg-green-500 text-white font-medium rounded-lg text-sm transition-colors"
        >
          + Create Another
        </button>
      </div>
    </div>
  )
}
