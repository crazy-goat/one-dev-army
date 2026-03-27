interface YoloToggleProps {
  label: string
  description: string
  checked: boolean
  onChange: (checked: boolean) => void
  warning?: string
}

export function YoloToggle({
  label,
  description,
  checked,
  onChange,
  warning,
}: YoloToggleProps) {
  return (
    <div className="flex flex-col gap-1">
      <label className="flex items-center gap-3 cursor-pointer">
        <button
          type="button"
          role="switch"
          aria-checked={checked}
          onClick={() => onChange(!checked)}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors flex-shrink-0 ${
            checked ? 'bg-blue-600' : 'bg-gray-700'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
              checked ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
        <span className="text-sm font-medium text-gray-200">{label}</span>
      </label>
      <p className="text-xs text-gray-500 ml-14">{description}</p>
      {warning !== undefined && warning !== '' && checked && (
        <p className="text-xs text-yellow-400 ml-14 mt-1">{warning}</p>
      )}
    </div>
  )
}
