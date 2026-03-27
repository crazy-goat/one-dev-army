interface StepIndicatorProps {
  currentStep: number
  steps: string[]
}

export function StepIndicator({ currentStep, steps }: StepIndicatorProps) {
  return (
    <div className="flex items-center justify-center gap-2 mb-8">
      {steps.map((label, i) => {
        const stepNum = i + 1
        const isActive = stepNum === currentStep
        const isCompleted = stepNum < currentStep

        return (
          <div key={label} className="flex items-center gap-2">
            {i > 0 && <div className={`w-8 h-px ${isCompleted ? 'bg-blue-500' : 'bg-gray-700'}`} />}
            <div className="flex items-center gap-1.5">
              <div
                className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-semibold ${
                  isActive
                    ? 'bg-blue-600 text-white'
                    : isCompleted
                      ? 'bg-blue-600/30 text-blue-400'
                      : 'bg-gray-800 text-gray-500'
                }`}
              >
                {isCompleted ? '\u2713' : String(stepNum)}
              </div>
              <span
                className={`text-xs font-medium ${
                  isActive ? 'text-white' : isCompleted ? 'text-blue-400' : 'text-gray-500'
                }`}
              >
                {label}
              </span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
