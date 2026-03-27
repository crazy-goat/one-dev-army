import { useState, useRef, useEffect } from 'react'

import type { ProviderModel } from '../../api/types'

interface ModelSelectorProps {
  label: string
  value: string
  models: ProviderModel[]
  onChange: (modelId: string) => void
}

export function ModelSelector({
  label,
  value,
  models,
  onChange,
}: ModelSelectorProps) {
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(-1)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Filter models by search text
  const filtered = filter
    ? models.filter(
        (m) =>
          m.name.toLowerCase().includes(filter.toLowerCase()) ||
          m.id.toLowerCase().includes(filter.toLowerCase()) ||
          m.provider_id.toLowerCase().includes(filter.toLowerCase()),
      )
    : models

  // Group by provider
  const grouped = new Map<string, ProviderModel[]>()
  for (const m of filtered) {
    const existing = grouped.get(m.provider_id)
    if (existing) {
      existing.push(m)
    } else {
      grouped.set(m.provider_id, [m])
    }
  }

  // Close on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false)
      }
    }
    document.addEventListener('click', handleClick)
    return () => document.removeEventListener('click', handleClick)
  }, [])

  const handleSelect = (modelId: string) => {
    onChange(modelId)
    setOpen(false)
    setFilter('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {return}

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIndex((prev) => Math.min(prev + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIndex((prev) => Math.max(prev - 1, -1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const selected = filtered[selectedIndex]
      if (selected) {
        handleSelect(selected.id)
      }
    } else if (e.key === 'Escape') {
      setOpen(false)
      setSelectedIndex(-1)
    }
  }

  return (
    <div className="mb-4" ref={containerRef}>
      <label className="block text-sm font-medium text-gray-300 mb-1">
        {label}
      </label>
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={open ? filter : value}
          onChange={(e) => {
            setFilter(e.target.value)
            setSelectedIndex(-1)
          }}
          onFocus={() => {
            setOpen(true)
            setFilter('')
          }}
          onKeyDown={handleKeyDown}
          className="w-full px-3 py-2 bg-gray-950 border border-gray-700 rounded-lg text-gray-200 text-sm focus:outline-none focus:border-blue-500 transition-colors"
          placeholder="Select a model..."
          autoComplete="off"
        />

        {open && (
          <div className="absolute top-full left-0 right-0 max-h-[250px] overflow-y-auto bg-gray-900 border border-gray-700 border-t-0 rounded-b-lg z-50 shadow-lg">
            {filtered.length === 0 ? (
              <div className="px-3 py-2 text-gray-500 italic text-sm text-center">
                No models found
              </div>
            ) : (
              [...grouped.entries()].sort().map(([provider, providerModels]) => (
                <div key={provider}>
                  <div className="px-3 py-1 text-xs font-semibold text-gray-500 bg-gray-950 border-b border-gray-800">
                    {provider}
                  </div>
                  {providerModels.map((m) => {
                    const idx = filtered.indexOf(m)
                    return (
                      <button
                        key={m.id}
                        type="button"
                        onClick={() => handleSelect(m.id)}
                        className={`w-full text-left px-3 py-2 text-sm border-b border-gray-800 last:border-b-0 transition-colors ${
                          idx === selectedIndex
                            ? 'bg-blue-600 text-white'
                            : 'text-gray-300 hover:bg-gray-800'
                        }`}
                      >
                        <div className="font-medium">{m.name}</div>
                        <div
                          className={`text-xs ${idx === selectedIndex ? 'text-blue-200' : 'text-gray-500'}`}
                        >
                          {m.id}
                        </div>
                      </button>
                    )
                  })}
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  )
}
