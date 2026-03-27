import { useState } from 'react'
import { Link } from 'react-router'

import { api } from '../api/client'
import type { SprintClosePreview, SprintCloseResult } from '../api/types'

type BumpType = 'major' | 'minor' | 'patch'

const BUMP_OPTIONS: {
  value: BumpType
  label: string
  description: string
}[] = [
  {
    value: 'major',
    label: 'Major Release',
    description: 'Breaking changes, incompatible API modifications',
  },
  {
    value: 'minor',
    label: 'Minor Release',
    description: 'New features, backwards-compatible additions',
  },
  {
    value: 'patch',
    label: 'Patch Release',
    description: 'Bug fixes, backwards-compatible corrections',
  },
]

export default function SprintClosePage() {
  const [bumpType, setBumpType] = useState<BumpType>('patch')
  const [preview, setPreview] = useState<SprintClosePreview | null>(null)
  const [result, setResult] = useState<SprintCloseResult | null>(null)
  const [releaseTitle, setReleaseTitle] = useState('')
  const [releaseBody, setReleaseBody] = useState('')
  const [isLoadingPreview, setIsLoadingPreview] = useState(false)
  const [isClosing, setIsClosing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handlePreview = async (bump: BumpType) => {
    setBumpType(bump)
    setIsLoadingPreview(true)
    setError(null)
    try {
      const data = await api.previewSprintClose(bump)
      setPreview(data)
      setReleaseTitle(data.release_title)
      setReleaseBody(data.release_body)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate preview')
    } finally {
      setIsLoadingPreview(false)
    }
  }

  const handleClose = async () => {
    setIsClosing(true)
    setError(null)
    try {
      const data = await api.confirmSprintClose({
        bump_type: bumpType,
        release_title: releaseTitle,
        release_body: releaseBody,
      })
      setResult(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to close sprint')
    } finally {
      setIsClosing(false)
    }
  }

  // Success state
  if (result !== null && result.success === true) {
    return (
      <div className="max-w-2xl mx-auto p-4 py-12">
        <div className="text-center mb-8">
          <div className="text-4xl mb-4">{'\u2705'}</div>
          <h1 className="text-2xl font-bold text-white mb-2">Sprint Closed Successfully</h1>
          {result.warning !== undefined && (
            <p className="text-yellow-400 text-sm mt-2">{result.warning}</p>
          )}
        </div>

        <div className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6 space-y-3">
          <div className="flex justify-between text-sm">
            <span className="text-gray-500">Tag</span>
            <span className="text-gray-200 font-mono">{result.tag_name}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-gray-500">Release</span>
            <span className="text-gray-200">{result.release_title}</span>
          </div>
          {result.release_url && (
            <div className="flex justify-between text-sm">
              <span className="text-gray-500">URL</span>
              <a
                href={result.release_url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-400 hover:text-blue-300 transition-colors"
              >
                View on GitHub &rarr;
              </a>
            </div>
          )}
          <div className="flex justify-between text-sm">
            <span className="text-gray-500">Milestone</span>
            <span className="text-gray-200">{result.milestone_title}</span>
          </div>
          {result.new_sprint_title !== undefined && (
            <div className="flex justify-between text-sm">
              <span className="text-gray-500">New Sprint</span>
              <span className="text-green-400">{result.new_sprint_title}</span>
            </div>
          )}
        </div>

        <div className="text-center">
          <Link
            to="/"
            className="px-6 py-2.5 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors inline-block"
          >
            Back to Board
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto p-4 py-8">
      {/* Header */}
      <div className="text-center mb-8">
        <h1 className="text-2xl font-bold text-white mb-1">Close Sprint &amp; Release</h1>
        <p className="text-gray-500 text-sm">
          Select the version bump type and review the generated release notes
        </p>
      </div>

      {/* Error */}
      {error !== null && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-6 text-sm text-red-400 text-center">
          {error}
        </div>
      )}

      <div className="grid grid-cols-[minmax(auto,300px)_1fr] gap-6">
        {/* Left panel: Version bump selector */}
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-6">
          {/* Version display */}
          {preview && (
            <div className="bg-gray-950 border border-gray-800 rounded-lg p-4 mb-6 text-center">
              <div className="text-xs text-gray-500 mb-1">Current Version</div>
              <div className="text-2xl font-semibold font-mono text-gray-200">
                {preview.current_version}
              </div>
              <div className="text-gray-600 my-2">{'\u2193'}</div>
              <div className="text-xs text-gray-500 mb-1">New Version</div>
              <div className="text-2xl font-semibold font-mono text-blue-400">
                {preview.new_version}
              </div>
            </div>
          )}

          {/* Bump options */}
          <div className="space-y-2 mb-6">
            {BUMP_OPTIONS.map(opt => (
              <label
                key={opt.value}
                className={`flex items-center p-3 bg-gray-950 border-2 rounded-lg cursor-pointer transition-all ${
                  bumpType === opt.value
                    ? 'border-blue-500'
                    : 'border-gray-800 hover:border-gray-600'
                }`}
              >
                <input
                  type="radio"
                  name="bump_type"
                  value={opt.value}
                  checked={bumpType === opt.value}
                  onChange={() => setBumpType(opt.value)}
                  className="mr-3 accent-blue-500"
                />
                <div>
                  <div className="text-sm font-medium text-gray-200">{opt.label}</div>
                  <div className="text-xs text-gray-500">{opt.description}</div>
                </div>
              </label>
            ))}
          </div>

          {/* Actions */}
          <div className="flex flex-col gap-2">
            <button
              type="button"
              onClick={() => void handlePreview(bumpType)}
              disabled={isLoadingPreview}
              className="w-full px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
            >
              {isLoadingPreview ? (
                <span className="flex items-center justify-center gap-2">
                  <span className="w-4 h-4 border-2 border-gray-500 border-t-white rounded-full animate-spin" />
                  Generating...
                </span>
              ) : (
                'Preview Release Notes'
              )}
            </button>

            {preview && (
              <>
                <Link
                  to="/"
                  className="w-full px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm font-medium transition-colors text-center"
                >
                  Cancel
                </Link>
                <button
                  type="button"
                  onClick={() => void handleClose()}
                  disabled={isClosing}
                  className="w-full px-4 py-2 bg-green-600 hover:bg-green-500 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
                >
                  {isClosing ? (
                    <span className="flex items-center justify-center gap-2">
                      <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                      Closing...
                    </span>
                  ) : (
                    'Create Tag & Close Sprint'
                  )}
                </button>
              </>
            )}
          </div>
        </div>

        {/* Right panel: Preview */}
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-6 min-h-[400px]">
          {isLoadingPreview ? (
            <div className="flex flex-col items-center justify-center h-[300px] text-gray-500">
              <div className="w-10 h-10 border-3 border-gray-700 border-t-blue-500 rounded-full animate-spin mb-4" />
              <span className="text-sm">Generating release notes with LLM...</span>
            </div>
          ) : preview ? (
            <>
              {/* Preview header */}
              <div className="flex justify-between items-center mb-4 pb-4 border-b border-gray-800">
                <h3 className="text-blue-400 font-semibold">Release Preview</h3>
                <span className="text-xs text-gray-500 font-mono bg-gray-950 px-2 py-1 rounded">
                  {preview.tag_name}
                </span>
              </div>

              {/* Editable title */}
              <div className="mb-4">
                <label className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                  Release Title
                </label>
                <input
                  type="text"
                  value={releaseTitle}
                  onChange={e => setReleaseTitle(e.target.value)}
                  className="w-full px-3 py-2 bg-gray-950 border border-gray-700 rounded-lg text-gray-200 text-sm font-semibold focus:outline-none focus:border-blue-500 transition-colors"
                />
              </div>

              {/* Editable body */}
              <div className="mb-4">
                <label className="text-xs text-gray-500 uppercase tracking-wider block mb-1">
                  Release Notes
                </label>
                <textarea
                  value={releaseBody}
                  onChange={e => setReleaseBody(e.target.value)}
                  rows={12}
                  className="w-full px-3 py-2 bg-gray-950 border border-gray-700 rounded-lg text-gray-300 text-sm font-mono resize-y focus:outline-none focus:border-blue-500 transition-colors leading-relaxed"
                />
              </div>

              {/* Closed issues */}
              {preview.closed_issues.length > 0 && (
                <div>
                  <h4 className="text-xs text-gray-500 uppercase tracking-wider mb-2">
                    Closed Issues ({preview.closed_issues.length})
                  </h4>
                  <div className="space-y-1">
                    {preview.closed_issues.map(issue => (
                      <div key={issue.number} className="text-sm text-gray-400 flex gap-2">
                        <span className="text-blue-400 font-mono">#{issue.number}</span>
                        <span>{issue.title}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {preview.llm_generated && (
                <div className="mt-4 pt-3 border-t border-gray-800 text-xs text-gray-600 text-center">
                  Release notes generated by LLM
                </div>
              )}
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-[300px] text-gray-600 text-center">
              <div className="text-4xl mb-4 opacity-50">{'\uD83D\uDCE6'}</div>
              <p className="text-sm">
                Select a version bump type and click &quot;Preview Release Notes&quot; to generate
                release notes.
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
