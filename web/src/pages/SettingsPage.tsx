import { useState, useEffect } from 'react'

import { useSettings, useSaveSettings, useToggleYolo } from '../api/queries'
import { ModelSelector } from '../components/settings/ModelSelector'
import { YoloToggle } from '../components/settings/YoloToggle'

const MODEL_FIELDS = [
  { key: 'setup' as const, label: 'Setup' },
  { key: 'planning' as const, label: 'Planning' },
  { key: 'orchestration' as const, label: 'Orchestration' },
  { key: 'code' as const, label: 'Code' },
  { key: 'code_heavy' as const, label: 'Code Heavy' },
] as const

type ModelKey = (typeof MODEL_FIELDS)[number]['key']

export default function SettingsPage() {
  const { data: settings, isLoading, error } = useSettings()
  const saveSettings = useSaveSettings()
  const toggleYolo = useToggleYolo()

  // Local form state
  const [models, setModels] = useState<Record<ModelKey, string>>({
    setup: '',
    planning: '',
    orchestration: '',
    code: '',
    code_heavy: '',
  })
  const [yoloMode, setYoloMode] = useState(false)
  const [sprintAutoStart, setSprintAutoStart] = useState(false)
  const [feedback, setFeedback] = useState<{
    type: 'success' | 'error'
    message: string
  } | null>(null)

  // Sync server state into local form state
  useEffect(() => {
    if (!settings) {
      return
    }
    setModels({
      setup: settings.config.Setup.Model,
      planning: settings.config.Planning.Model,
      orchestration: settings.config.Orchestration.Model,
      code: settings.config.Code.Model,
      code_heavy: settings.config.CodeHeavy.Model,
    })
    setYoloMode(settings.yolo_mode)
    setSprintAutoStart(settings.sprint_auto_start)
  }, [settings])

  const handleSave = () => {
    setFeedback(null)
    saveSettings.mutate(
      {
        setup_model: models.setup,
        planning_model: models.planning,
        orchestration_model: models.orchestration,
        code_model: models.code,
        code_heavy_model: models.code_heavy,
        yolo_mode: yoloMode,
        sprint_auto_start: sprintAutoStart,
      },
      {
        onSuccess: () => setFeedback({ type: 'success', message: 'Settings saved successfully!' }),
        onError: err =>
          setFeedback({
            type: 'error',
            message: `Failed to save: ${err.message}`,
          }),
      }
    )
  }

  const handleYoloToggle = () => {
    toggleYolo.mutate(undefined, {
      onSuccess: data => setYoloMode(data.yolo_mode),
    })
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-2 border-gray-700 border-t-blue-500 rounded-full animate-spin" />
          <span className="text-gray-500 text-sm">Loading settings...</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="text-center">
          <p className="text-red-400 mb-2">Failed to load settings: {error.message}</p>
          <button
            type="button"
            onClick={() => window.location.reload()}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  if (!settings) {
    return null
  }

  return (
    <div className="max-w-3xl mx-auto p-4 pb-12">
      <h1 className="text-xl font-bold text-white mb-6">LLM Configuration Settings</h1>

      {/* Feedback */}
      {feedback && (
        <div
          className={`p-3 rounded-lg mb-4 text-sm ${
            feedback.type === 'success'
              ? 'bg-green-500/10 border border-green-500/30 text-green-400'
              : 'bg-red-500/10 border border-red-500/30 text-red-400'
          }`}
        >
          {feedback.message}
        </div>
      )}

      {/* Models Section */}
      <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
        <h2 className="text-lg font-semibold text-blue-400 mb-1">Models</h2>
        <p className="text-sm text-gray-500 mb-4">
          Select a model for each operational mode. Each mode uses its own dedicated model.
        </p>

        {MODEL_FIELDS.map(({ key, label }) => (
          <ModelSelector
            key={key}
            label={label}
            value={models[key]}
            models={settings.available_models}
            onChange={modelId => setModels(prev => ({ ...prev, [key]: modelId }))}
          />
        ))}
      </section>

      {/* Pipeline Settings */}
      <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
        <h2 className="text-lg font-semibold text-blue-400 mb-4">Pipeline Settings</h2>
        <YoloToggle
          label="Enable YOLO Mode"
          description="When enabled, tickets automatically transition from awaiting-approval to merge without requiring manual user approval."
          checked={yoloMode}
          onChange={handleYoloToggle}
          warning="YOLO mode is active! All PRs will be auto-merged without your review."
        />
      </section>

      {/* Sprint Settings */}
      <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
        <h2 className="text-lg font-semibold text-blue-400 mb-4">Sprint Settings</h2>
        <YoloToggle
          label="Auto-start sprints"
          description="When enabled, new sprints automatically start upon creation."
          checked={sprintAutoStart}
          onChange={setSprintAutoStart}
        />
      </section>

      {/* Save */}
      <div className="border-t border-gray-800 pt-4">
        <button
          type="button"
          onClick={handleSave}
          disabled={saveSettings.isPending}
          className="px-6 py-2.5 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-lg text-sm transition-colors disabled:opacity-50"
        >
          {saveSettings.isPending ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  )
}
