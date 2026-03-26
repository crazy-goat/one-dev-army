import { useState } from 'react'

const LANGUAGES = [
  { value: 'en-US', label: '\uD83C\uDDFA\uD83C\uDDF8 English' },
  { value: 'pl-PL', label: '\uD83C\uDDF5\uD83C\uDDF1 Polski' },
  { value: 'de-DE', label: '\uD83C\uDDE9\uD83C\uDDEA Deutsch' },
  { value: 'es-ES', label: '\uD83C\uDDEA\uD83C\uDDF8 Espa\u00F1ol' },
  { value: 'fr-FR', label: '\uD83C\uDDEB\uD83C\uDDF7 Fran\u00E7ais' },
  { value: 'pt-PT', label: '\uD83C\uDDF5\uD83C\uDDF9 Portugu\u00EAs' },
  { value: 'it-IT', label: '\uD83C\uDDEE\uD83C\uDDF9 Italiano' },
  { value: 'nl-NL', label: '\uD83C\uDDF3\uD83C\uDDF1 Nederlands' },
  { value: 'ru-RU', label: '\uD83C\uDDF7\uD83C\uDDFA \u0420\u0443\u0441\u0441\u043A\u0438\u0439' },
  { value: 'zh-CN', label: '\uD83C\uDDE8\uD83C\uDDF3 \u4E2D\u6587' },
  { value: 'ja-JP', label: '\uD83C\uDDEF\uD83C\uDDF5 \u65E5\u672C\u8A9E' },
  { value: 'ko-KR', label: '\uD83C\uDDF0\uD83C\uDDF7 \uD55C\uAD6D\uC5B4' },
] as const

interface IdeaFormProps {
  onSubmit: (data: {
    type: string
    idea: string
    language: string
    addToSprint: boolean
  }) => void
  isLoading: boolean
}

export function IdeaForm({ onSubmit, isLoading }: IdeaFormProps) {
  const [type, setType] = useState<string | null>(null)
  const [idea, setIdea] = useState('')
  const [language, setLanguage] = useState('en-US')
  const [addToSprint, setAddToSprint] = useState(true)

  // Type selection screen
  if (!type) {
    return (
      <div>
        <h2 className="text-xl font-bold text-white mb-6 text-center">
          Select Issue Type
        </h2>
        <div className="grid grid-cols-2 gap-4 max-w-md mx-auto">
          <button
            type="button"
            onClick={() => setType('feature')}
            className="border-2 border-gray-700 rounded-lg p-6 text-center hover:border-blue-500 hover:-translate-y-0.5 transition-all bg-gray-900"
          >
            <div className="text-3xl mb-2">{'\u2728'}</div>
            <div className="font-semibold text-gray-200">Feature</div>
            <div className="text-xs text-gray-500 mt-1">
              New functionality or enhancement
            </div>
          </button>
          <button
            type="button"
            onClick={() => setType('bug')}
            className="border-2 border-gray-700 rounded-lg p-6 text-center hover:border-red-500 hover:-translate-y-0.5 transition-all bg-gray-900"
          >
            <div className="text-3xl mb-2">{'\uD83D\uDC1B'}</div>
            <div className="font-semibold text-gray-200">Bug</div>
            <div className="text-xs text-gray-500 mt-1">
              Something is not working correctly
            </div>
          </button>
        </div>
      </div>
    )
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!idea.trim()) return
    onSubmit({ type, idea: idea.trim(), language, addToSprint })
  }

  return (
    <div>
      <h2 className="text-xl font-bold text-white mb-6">
        Create New {type === 'bug' ? 'Bug Report' : 'Feature'}
      </h2>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <label
            htmlFor="wizard-idea"
            className="block text-sm text-gray-400 mb-2"
          >
            Describe your {type === 'bug' ? 'bug' : 'feature idea'}:
          </label>
          <textarea
            id="wizard-idea"
            value={idea}
            onChange={(e) => setIdea(e.target.value)}
            rows={6}
            required
            className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm resize-y focus:outline-none focus:border-blue-500 transition-colors font-sans"
            placeholder={
              type === 'bug'
                ? 'Describe the bug, steps to reproduce, and expected behavior...'
                : 'Describe the feature, who it\'s for, and what problem it solves...'
            }
          />
        </div>

        <div className="mb-4">
          <label
            htmlFor="wizard-language"
            className="block text-sm text-gray-400 mb-2"
          >
            Language:
          </label>
          <select
            id="wizard-language"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            className="px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm focus:outline-none focus:border-blue-500 transition-colors cursor-pointer"
          >
            {LANGUAGES.map((l) => (
              <option key={l.value} value={l.value}>
                {l.label}
              </option>
            ))}
          </select>
        </div>

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

        <div className="flex justify-between items-center">
          <button
            type="button"
            onClick={() => setType(null)}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            &larr; Back
          </button>
          <button
            type="submit"
            disabled={isLoading || !idea.trim()}
            className="px-6 py-2 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors disabled:opacity-50"
          >
            {isLoading ? (
              <span className="flex items-center gap-2">
                <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Refining...
              </span>
            ) : (
              'Refine with AI'
            )}
          </button>
        </div>
      </form>
    </div>
  )
}
