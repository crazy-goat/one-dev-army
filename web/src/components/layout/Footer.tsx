import { useRateLimit, useToggleYolo, useBoard, useTriggerSync } from '../../api/queries'
import { useAppContext } from '../../App'
import type { RateLimit, APILimit } from '../../api/types'

/** Calculate usage percentage for an API limit */
function getUsagePercentage(limit: APILimit | null): number {
  if (!limit || limit.limit === 0) return 0
  return ((limit.limit - limit.remaining) / limit.limit) * 100
}

/** Get the worst API limit with highest usage percentage */
function getWorstLimit(rateLimit: RateLimit): APILimit | null {
  const limits: APILimit[] = []
  if (rateLimit.core) limits.push(rateLimit.core)
  if (rateLimit.graphql) limits.push(rateLimit.graphql)
  if (rateLimit.search) limits.push(rateLimit.search)
  
  if (limits.length === 0) return null
  
  return limits.reduce((worst, current) => {
    const worstPct = getUsagePercentage(worst)
    const currentPct = getUsagePercentage(current)
    return currentPct > worstPct ? current : worst
  })
}

/** Get the worst usage percentage across all API types */
function getWorstPercentage(rateLimit: RateLimit): number {
  const worst = getWorstLimit(rateLimit)
  if (!worst) return 0
  return getUsagePercentage(worst)
}

/** Returns a color class based on the usage percentage. */
function rateLimitColor(percentage: number): string {
  if (percentage > 80) return 'text-red-400'
  if (percentage > 50) return 'text-yellow-400'
  return 'text-green-400'
}

/** Returns background color class for rate limit badge. */
function rateLimitBgColor(percentage: number): string {
  if (percentage > 80) return 'bg-red-900/30'
  if (percentage > 50) return 'bg-yellow-900/30'
  return 'bg-green-900/30'
}

export function Footer() {
  const { data: rateLimit } = useRateLimit()
  const { data: board } = useBoard()
  const toggleYolo = useToggleYolo()
  const triggerSync = useTriggerSync()
  const { wsConnected } = useAppContext()

  return (
    <footer className="bg-[#161b22] border-t border-[#30363d] px-6 py-2 fixed bottom-0 left-0 right-0 flex justify-between items-center text-[0.8rem] text-[#8b949e] z-[100]">
      {/* Left side */}
      <div className="flex items-center gap-3">
        {/* WebSocket status */}
        <span 
          className={`flex items-center justify-center w-5 h-5 rounded-full ${
            wsConnected ? 'text-green-500' : 'text-red-500'
          }`}
          title={wsConnected ? 'Connected' : 'Disconnected'}
        >
          {wsConnected ? (
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
              <path d="M5 12l5 5 9-9"/>
            </svg>
          ) : (
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          )}
        </span>

        {/* Sync button */}
        <button
          type="button"
          onClick={() => triggerSync.mutate()}
          disabled={triggerSync.isPending}
          className="px-3 py-1.5 rounded border border-[#30363d] bg-[#21262d] text-[#c9d1d9] text-xs font-medium hover:bg-[#30363d] hover:border-[#58a6ff] transition-colors disabled:opacity-50 flex items-center gap-1.5"
        >
          {triggerSync.isPending && <span className="animate-spin">⟳</span>}
          <span>Sync</span>
        </button>

        {/* Try new dashboard link */}
        <a 
          href="/new/" 
          className="text-[#58a6ff] hover:text-[#79c0ff] transition-colors text-xs ml-4 flex items-center gap-1"
        >
          <span>🚀</span>
          <span>Try new dashboard</span>
        </a>
      </div>

      {/* Right side */}
      <div className="flex items-center gap-3">
        {/* Worker status */}
        {board && (
          <div 
            className={`flex items-center gap-2 px-3 py-1.5 rounded border text-xs font-medium cursor-pointer transition-colors ${
              board.paused 
                ? 'border-[#30363d] bg-[#21262d] text-[#8b949e] hover:bg-[#30363d]' 
                : 'border-dashed border-[#30363d] bg-[#161b22] text-[#8b949e]'
            }`}
            title={board.paused ? 'Worker is paused. Click to start.' : 'Worker is idle. Click to start.'}
          >
            <span className="flex items-center justify-center w-3 h-3">
              {board.paused ? (
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-3 h-3">
                  <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
                </svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-3 h-3">
                  <circle cx="12" cy="12" r="10"/>
                </svg>
              )}
            </span>
            <span className="uppercase tracking-wider">
              {board.paused ? 'Paused' : 'Idle'}
            </span>
          </div>
        )}

        {/* YOLO / SAFE MODE */}
        {board && (
          <button
            type="button"
            onClick={() => toggleYolo.mutate()}
            disabled={toggleYolo.isPending}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded border text-xs font-medium transition-colors disabled:opacity-50 ${
              board.yolo_mode
                ? 'border-[#d29922]/40 bg-[#d29922]/10 text-[#d29922] hover:bg-[#d29922]/20'
                : 'border-[#30363d] bg-[#21262d] text-[#8b949e] hover:bg-[#30363d]'
            }`}
            title={
              board.yolo_mode
                ? 'YOLO mode ON — PRs auto-merge without review. Click to disable.'
                : 'SAFE MODE — All changes require manual approval. Click to enable YOLO mode.'
            }
          >
            <span>{board.yolo_mode ? '⚡' : '🔒'}</span>
            <span className="uppercase tracking-wider">
              {board.yolo_mode ? 'YOLO MODE' : 'SAFE MODE'}
            </span>
          </button>
        )}

        {/* GitHub API usage */}
        {rateLimit && (
          <div 
            className={`px-3 py-1.5 rounded border text-xs font-medium ${rateLimitBgColor(getWorstPercentage(rateLimit))} border-[#30363d]`}
            title="GitHub API rate limit usage"
          >
            <span className={rateLimitColor(getWorstPercentage(rateLimit))}>
              GitHub API usage: {Math.round(getWorstPercentage(rateLimit))}%
            </span>
          </div>
        )}
      </div>
    </footer>
  )
}
