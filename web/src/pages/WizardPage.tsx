import { useState } from 'react'

import { api } from '../api/client'
import type { WizardSession, CreatedIssue } from '../api/types'
import { CreateConfirm } from '../components/wizard/CreateConfirm'
import { IdeaForm } from '../components/wizard/IdeaForm'
import { RefinePreview } from '../components/wizard/RefinePreview'
import { StepIndicator } from '../components/wizard/StepIndicator'

type WizardStep = 'idea' | 'review' | 'confirm'

const STEP_LABELS = ['Idea', 'Review', 'Confirm']

function stepToNumber(step: WizardStep): number {
  switch (step) {
    case 'idea':
      return 1
    case 'review':
      return 2
    case 'confirm':
      return 3
  }
}

export default function WizardPage() {
  const [step, setStep] = useState<WizardStep>('idea')
  const [session, setSession] = useState<WizardSession | null>(null)
  const [createdIssues, setCreatedIssues] = useState<CreatedIssue[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleIdeaSubmit = async (data: {
    type: string
    idea: string
    language: string
    addToSprint: boolean
  }) => {
    setIsLoading(true)
    setError(null)
    try {
      // Step 1: Create session
      const newSession = await api.createWizardSession(data.type)

      // Step 2: Refine with AI
      const refined = await api.refineWizardSession(newSession.id, {
        idea: data.idea,
        language: data.language,
      })

      setSession({ ...refined, add_to_sprint: data.addToSprint })
      setStep('review')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to refine idea',
      )
    } finally {
      setIsLoading(false)
    }
  }

  const handleCreateIssue = async (title: string, addToSprint: boolean) => {
    if (!session) {return}
    setIsLoading(true)
    setError(null)
    try {
      const result = await api.createWizardIssue(session.id, {
        title: title || undefined,
        add_to_sprint: addToSprint,
      })
      // Go API returns { success: true, issue: CreatedIssue }
      setCreatedIssues([result.issue])
      setStep('confirm')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to create issue',
      )
    } finally {
      setIsLoading(false)
    }
  }

  // MISSING 4: Regenerate handler — re-refine with current description
  const handleRegenerate = async (currentDescription: string) => {
    if (!session) {return}
    setIsLoading(true)
    setError(null)
    try {
      const refined = await api.refineWizardSession(session.id, {
        idea: currentDescription,
        language: session.language,
      })
      setSession((prev) =>
        prev ? { ...prev, ...refined, add_to_sprint: prev.add_to_sprint } : refined,
      )
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to regenerate',
      )
    } finally {
      setIsLoading(false)
    }
  }

  const handleReset = () => {
    setStep('idea')
    setSession(null)
    setCreatedIssues([])
    setError(null)
  }

  return (
    <div className="max-w-2xl mx-auto p-4 py-8">
      <StepIndicator currentStep={stepToNumber(step)} steps={STEP_LABELS} />

      {/* Error banner */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 mb-6 text-sm text-red-400 flex items-center justify-between">
          <span>{error}</span>
          <button
            type="button"
            onClick={() => setError(null)}
            className="text-red-400 hover:text-red-300 ml-4 text-lg leading-none"
          >
            &times;
          </button>
        </div>
      )}

      {/* Step content */}
      {step === 'idea' && (
        <IdeaForm onSubmit={(data) => void handleIdeaSubmit(data)} isLoading={isLoading} />
      )}

      {step === 'review' && session && (
        <RefinePreview
          session={session}
          onBack={() => setStep('idea')}
          onCreateIssue={(title, addToSprint) => void handleCreateIssue(title, addToSprint)}
          onRegenerate={(desc) => void handleRegenerate(desc)}
          isLoading={isLoading}
        />
      )}

      {step === 'confirm' && (
        <CreateConfirm
          createdIssues={createdIssues}
          onCreateAnother={handleReset}
        />
      )}
    </div>
  )
}
