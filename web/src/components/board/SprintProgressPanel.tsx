interface SprintProgressPanelProps {
  totalTickets: number
  completedTickets: number
}

export function SprintProgressPanel({ totalTickets, completedTickets }: SprintProgressPanelProps) {
  if (totalTickets === 0) {
    return null
  }

  const percentage = Math.round((completedTickets / totalTickets) * 100)

  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg p-3 flex items-center gap-4">
      <span className="text-xs font-semibold uppercase tracking-wider text-gray-500">
        Sprint Progress
      </span>
      <div className="flex items-center gap-3 text-sm">
        <span className="text-gray-400">
          <span className="text-gray-200 font-medium">{totalTickets}</span> total
        </span>
        <span className="text-gray-600">|</span>
        <span className="text-gray-400">
          <span className="text-gray-200 font-medium">{completedTickets}</span> done
        </span>
        <span className="text-gray-600">|</span>
        <span className="text-green-400 font-semibold">{percentage}%</span>
      </div>
      <div className="flex-1 h-1.5 bg-gray-800 rounded-full overflow-hidden">
        <div
          className="h-full bg-green-500 rounded-full transition-all duration-500"
          style={{ width: `${String(percentage)}%` }}
        />
      </div>
    </div>
  )
}
