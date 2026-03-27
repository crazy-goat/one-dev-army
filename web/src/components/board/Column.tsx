import type { Card } from '../../api/types'

import { TaskCard } from './TaskCard'

interface ColumnProps {
  title: string
  /** The snake_case API key for this column (e.g. "backlog", "ai_review"). */
  columnKey: string
  cards: Card[]
  emptyText?: string
}

/** Column header color mapping (keyed by display label). */
const titleColor: Record<string, string> = {
  Backlog: 'text-gray-400',
  Blocked: 'text-red-400',
  Plan: 'text-yellow-400',
  Code: 'text-blue-400',
  'AI Review': 'text-cyan-400',
  Pipeline: 'text-teal-400',
  Approve: 'text-purple-400',
  Merge: 'text-violet-400',
  Done: 'text-green-400',
  Failed: 'text-red-500',
}

export function Column({ title, columnKey, cards, emptyText }: ColumnProps) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg p-3 min-w-0 flex flex-col h-full">
      {/* Column header */}
      <div className="flex justify-between items-center mb-3 sticky top-0 bg-gray-900 z-[1]">
        <span
          className={`text-xs font-semibold uppercase tracking-wider ${titleColor[title] ?? 'text-gray-400'}`}
        >
          {title}
        </span>
        <span className="bg-gray-800 text-gray-400 text-xs px-2 py-0.5 rounded-full">
          {cards.length}
        </span>
      </div>

      {/* Cards */}
      <div className="flex flex-col gap-2 overflow-y-auto flex-1 min-h-0">
        {cards.length > 0 ? (
          cards.map(card => (
            <TaskCard key={card.id} card={card} column={title} columnKey={columnKey} />
          ))
        ) : (
          <p className="text-gray-600 text-sm text-center py-8 italic">
            {emptyText ?? `No tickets in ${title.toLowerCase()}`}
          </p>
        )}
      </div>
    </div>
  )
}
