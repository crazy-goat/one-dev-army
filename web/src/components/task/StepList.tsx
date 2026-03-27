import { useState } from 'react'

import type { TaskStep } from '../../api/types'

interface StepListProps {
  steps: TaskStep[]
}

const statusColors: Record<string, string> = {
  running: 'bg-yellow-500 text-white',
  done: 'bg-green-600 text-white',
  failed: 'bg-red-500 text-white',
  pending: 'bg-gray-600 text-white',
}

function formatDuration(start?: string, end?: string): string | null {
  if (!start || !end) {return null}
  const ms = new Date(end).getTime() - new Date(start).getTime()
  if (ms < 1000) {return `${String(ms)}ms`}
  const secs = Math.floor(ms / 1000)
  if (secs < 60) {return `${String(secs)}s`}
  const mins = Math.floor(secs / 60)
  const remainSecs = secs % 60
  return `${String(mins)}m ${String(remainSecs)}s`
}

function StepItem({ step }: { step: TaskStep }) {
  const [open, setOpen] = useState(step.status === 'running')

  const duration = formatDuration(step.started_at, step.finished_at)

  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
      {/* Header */}
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex justify-between items-center w-full px-4 py-3 text-left hover:bg-white/[0.02] transition-colors"
      >
        <div className="flex items-center gap-3">
          <span className="font-semibold text-sm text-gray-200">
            {step.step_name}
          </span>
          {duration && (
            <span className="text-xs text-gray-500">{duration}</span>
          )}
          {step.llm_model && (
            <span className="text-xs text-gray-600 font-mono">
              {step.llm_model}
            </span>
          )}
        </div>
        <span
          className={`px-2 py-0.5 rounded text-xs font-semibold ${statusColors[step.status] ?? 'bg-gray-700 text-gray-300'}`}
        >
          {step.status}
        </span>
      </button>

      {/* Body */}
      {open && (
        <div className="border-t border-gray-800 px-4 pb-4">
          {step.prompt && (
            <div className="mt-3">
              <h4 className="text-xs uppercase tracking-wider text-gray-500 mb-1.5">
                Prompt
              </h4>
              <pre className="bg-gray-950 border border-gray-800 rounded p-3 text-sm text-gray-300 whitespace-pre-wrap break-words max-h-[400px] overflow-y-auto font-mono">
                {step.prompt}
              </pre>
            </div>
          )}

          {step.response && (
            <div className="mt-3">
              <h4 className="text-xs uppercase tracking-wider text-gray-500 mb-1.5">
                Response
              </h4>
              <pre className="bg-gray-950 border border-gray-800 rounded p-3 text-sm text-gray-300 whitespace-pre-wrap break-words max-h-[400px] overflow-y-auto font-mono">
                {step.response}
              </pre>
            </div>
          )}

          {step.error_msg && (
            <div className="mt-3">
              <h4 className="text-xs uppercase tracking-wider text-red-500 mb-1.5">
                Error
              </h4>
              <pre className="bg-gray-950 border border-red-500/30 rounded p-3 text-sm text-red-400 whitespace-pre-wrap break-words max-h-[400px] overflow-y-auto font-mono">
                {step.error_msg}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export function StepList({ steps }: StepListProps) {
  if (steps.length === 0) {
    return (
      <p className="text-gray-500 text-center py-12 italic">
        No steps recorded yet
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      {steps.map((step) => (
        <StepItem key={step.id} step={step} />
      ))}
    </div>
  )
}
