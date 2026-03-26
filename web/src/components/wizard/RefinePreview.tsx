import { useState, useMemo } from 'react'
import { marked } from 'marked'
import type { WizardSession } from '../../api/types'

interface RefinePreviewProps {
  session: WizardSession
  onBack: () => void
  onCreateIssue: (title: string, addToSprint: boolean) => void
  /** Called when the user clicks "Regenerate" to re-refine the description. */
  onRegenerate?: (description: string) => void
  isLoading: boolean
}

const priorityColors: Record<string, string> = {
  high: 'bg-red-500/20 text-red-400 border-red-500/40',
  medium: 'bg-yellow-500/20 text-yellow-500 border-yellow-500/40',
  low: 'bg-green-500/20 text-green-400 border-green-500/40',
}

const complexityColors: Record<string, string> = {
  S: 'bg-green-500/15 text-green-400 border-green-500/30',
  M: 'bg-teal-500/15 text-teal-400 border-teal-500/30',
  L: 'bg-blue-500/15 text-blue-400 border-blue-500/30',
  XL: 'bg-purple-500/15 text-purple-400 border-purple-500/30',
}

// Configure marked for safe rendering
marked.setOptions({
  breaks: true,
  gfm: true,
})

export function RefinePreview({
  session,
  onBack,
  onCreateIssue,
  onRegenerate,
  isLoading,
}: RefinePreviewProps) {
  const [title, setTitle] = useState(
    session.custom_title || session.generated_title || '',
  )
  const [addToSprint, setAddToSprint] = useState(session.add_to_sprint)
  const [showRaw, setShowRaw] = useState(false)
  const [description, setDescription] = useState(
    session.technical_planning || session.refined_description || '',
  )

  // Render markdown to HTML
  const renderedHtml = useMemo(() => {
    if (!description) return ''
    return marked(description) as string
  }, [description])

  return (
    <div>
      <h2 className="text-xl font-bold text-white mb-6">Review Issue</h2>

      {/* Title */}
      <div className="mb-4">
        <label
          htmlFor="wizard-title"
          className="block text-sm text-gray-400 mb-1"
        >
          Issue Title:
        </label>
        <input
          id="wizard-title"
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          maxLength={80}
          className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm focus:outline-none focus:border-blue-500 transition-colors"
        />
        <div className="flex justify-between mt-1 text-xs text-gray-500">
          <span>
            {title.length} / 80 characters
          </span>
          {title.length > 80 && (
            <span className="text-red-400">Title too long</span>
          )}
        </div>
      </div>

      {/* Badges */}
      {(session.priority || session.complexity) && (
        <div className="flex items-center gap-2 flex-wrap mb-4">
          {session.priority && (
            <span
              className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold border capitalize ${
                priorityColors[session.priority] ??
                'bg-gray-800 text-gray-400 border-gray-700'
              }`}
            >
              Priority: {session.priority}
            </span>
          )}
          {session.complexity && (
            <span
              className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold border ${
                complexityColors[session.complexity] ??
                'bg-gray-800 text-gray-400 border-gray-700'
              }`}
            >
              Size: {session.complexity}
            </span>
          )}
          <span className="text-xs text-gray-600 italic">
            Estimated by AI &mdash; labels will be added automatically
          </span>
        </div>
      )}

      {/* Description */}
      <div className="mb-4">
        <div className="flex items-center gap-2 mb-2">
          <label className="text-sm text-gray-400">Description:</label>
          <div className="flex gap-1">
            <button
              type="button"
              onClick={() => setShowRaw(false)}
              className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                !showRaw
                  ? 'bg-blue-600 border-blue-600 text-white'
                  : 'bg-gray-800 border-gray-700 text-gray-400 hover:bg-gray-700'
              }`}
            >
              Preview
            </button>
            <button
              type="button"
              onClick={() => setShowRaw(true)}
              className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                showRaw
                  ? 'bg-blue-600 border-blue-600 text-white'
                  : 'bg-gray-800 border-gray-700 text-gray-400 hover:bg-gray-700'
              }`}
            >
              Edit
            </button>
          </div>
          {/* MISSING 4: Regenerate button */}
          {onRegenerate && (
            <button
              type="button"
              onClick={() => onRegenerate(description)}
              disabled={isLoading}
              className="ml-auto px-2 py-0.5 text-xs rounded border border-yellow-600/40 bg-yellow-600/10 text-yellow-400 hover:bg-yellow-600/20 transition-colors disabled:opacity-50"
            >
              {isLoading ? 'Regenerating...' : '\u21BB Regenerate'}
            </button>
          )}
        </div>

        {showRaw ? (
          /* MISSING 3: Description editing — removed readOnly */
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={12}
            className="w-full px-3 py-2 bg-gray-950 border border-gray-700 rounded-lg text-gray-300 text-sm font-mono resize-y focus:outline-none focus:border-blue-500"
          />
        ) : (
          /* MISSING 5: Markdown rendering with marked */
          <div
            className="min-h-[200px] max-h-[400px] overflow-y-auto p-4 bg-gray-900 border border-gray-700 rounded-lg text-sm text-gray-300 leading-relaxed prose prose-invert prose-sm max-w-none"
            dangerouslySetInnerHTML={{ __html: renderedHtml }}
          />
        )}
      </div>

      {/* Add to sprint */}
      <div className="mb-6">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={addToSprint}
            onChange={(e) => setAddToSprint(e.target.checked)}
            className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-600 focus:ring-blue-500 cursor-pointer"
          />
          <span className="text-sm text-gray-300">
            Add to current sprint
          </span>
        </label>
      </div>

      {/* Actions */}
      <div className="flex justify-between items-center">
        <button
          type="button"
          onClick={onBack}
          className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
        >
          &larr; Back
        </button>
        <button
          type="button"
          onClick={() => onCreateIssue(title, addToSprint)}
          disabled={isLoading || !title.trim()}
          className="px-6 py-2 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors disabled:opacity-50"
        >
          {isLoading ? (
            <span className="flex items-center gap-2">
              <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
              Creating...
            </span>
          ) : (
            'Accept & Create Issue'
          )}
        </button>
      </div>
    </div>
  )
}
