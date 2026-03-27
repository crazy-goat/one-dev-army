import { useRateLimit, useToggleYolo, useBoard, useTriggerSync, useStartSprint, usePauseSprint } from '../../api/queries'
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

/** Format reset time to human readable format */
function formatResetTime(resetTimestamp: number): string {
  if (resetTimestamp === 0) return 'Unknown'
  
  const resetTime = new Date(resetTimestamp * 1000)
  const now = new Date()
  const diffMs = resetTime.getTime() - now.getTime()
  const diffMinutes = Math.ceil(diffMs / 60000)
  
  if (diffMinutes <= 0) return 'Resets soon'
  if (diffMinutes < 1) return 'Resets in <1 min'
  if (diffMinutes < 60) return `Resets in ${diffMinutes} min`
  
  const hours = Math.floor(diffMinutes / 60)
  const remainingMinutes = diffMinutes % 60
  if (remainingMinutes === 0) return `Resets in ${hours} hr`
  return `Resets in ${hours} hr ${remainingMinutes} min`
}

/** Get color for API limit based on usage percentage */
function getApiLimitColor(percentage: number): string {
  if (percentage > 80) return 'text-red-400'
  if (percentage > 50) return 'text-yellow-400'
  return 'text-green-400'
}

export function Footer() {
  const { data: rateLimit } = useRateLimit()
  const { data: board } = useBoard()
  const toggleYolo = useToggleYolo()
  const triggerSync = useTriggerSync()
  const startSprint = useStartSprint()
  const pauseSprint = usePauseSprint()
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
        {/* Worker status with tooltip */}
        {board && (
          <div className="relative group">
            <div 
              className={`flex items-center gap-2 px-3 py-1.5 rounded border text-xs font-medium cursor-pointer transition-colors ${
                board.paused 
                  ? 'border-[#30363d] bg-[#21262d] text-[#8b949e] hover:bg-[#30363d]' 
                  : 'border-dashed border-[#30363d] bg-[#161b22] text-[#8b949e]'
              }`}
              onClick={() => {
                if (board.paused) {
                  startSprint.mutate()
                } else {
                  pauseSprint.mutate()
                }
              }}
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
            
            {/* Tooltip */}
            <div className="absolute bottom-full right-0 mb-2 hidden group-hover:block w-[260px] bg-[#161b22] border border-[#30363d] rounded-md shadow-lg z-[1000]">
              <div className="p-3">
                <div className={`font-semibold text-sm mb-2 pb-2 border-b border-[#30363d] ${board.paused ? 'text-[#d29922]' : 'text-[#e6edf3]'}`}>
                  {board.paused ? 'Worker Paused' : 'Worker Idle'}
                </div>
                
                <div className="text-xs text-[#8b949e] leading-relaxed">
                  {board.paused 
                    ? 'Worker is currently paused. No tickets are being processed. Click to resume.'
                    : 'Worker is idle and waiting for tickets. When tickets are available, processing will begin automatically.'
                  }
                </div>
                
                {board.worker_count > 0 && (
                  <div className="mt-2 pt-2 border-t border-[#30363d] text-[10px] text-[#8b949e]">
                    {board.worker_count} worker{board.worker_count !== 1 ? 's' : ''} ready
                  </div>
                )}
              </div>
              
              {/* Arrow */}
              <div className="absolute top-full right-4 w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-t-[6px] border-t-[#161b22]" />
              <div className="absolute top-full right-[15px] w-0 h-0 border-l-[7px] border-l-transparent border-r-[7px] border-r-transparent border-t-[7px] border-t-[#30363d] -mt-[1px]" />
            </div>
          </div>
        )}

        {/* YOLO / SAFE MODE with tooltip */}
        {board && (
          <div className="relative group">
            <button
              type="button"
              onClick={() => toggleYolo.mutate()}
              disabled={toggleYolo.isPending}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded border text-xs font-medium transition-colors disabled:opacity-50 ${
                board.yolo_mode
                  ? 'border-[#d29922]/40 bg-[#d29922]/10 text-[#d29922] hover:bg-[#d29922]/20'
                  : 'border-[#30363d] bg-[#21262d] text-[#8b949e] hover:bg-[#30363d]'
              }`}
            >
              <span>{board.yolo_mode ? '⚡' : '🔒'}</span>
              <span className="uppercase tracking-wider">
                {board.yolo_mode ? 'YOLO MODE' : 'SAFE MODE'}
              </span>
            </button>
            
            {/* Tooltip */}
            <div className="absolute bottom-full right-0 mb-2 hidden group-hover:block w-[280px] bg-[#161b22] border border-[#30363d] rounded-md shadow-lg z-[1000]">
              <div className="p-3">
                <div className={`font-semibold text-sm mb-2 pb-2 border-b border-[#30363d] ${board.yolo_mode ? 'text-[#d29922]' : 'text-[#e6edf3]'}`}>
                  {board.yolo_mode ? 'YOLO Mode Enabled' : 'Safe Mode Enabled'}
                </div>
                
                <div className="text-xs text-[#8b949e] leading-relaxed">
                  {board.yolo_mode 
                    ? 'AI will auto-approve all changes without human review. Click to disable.'
                    : 'All changes require manual approval. Click to enable YOLO mode (auto-approve).'
                  }
                </div>
              </div>
              
              {/* Arrow */}
              <div className="absolute top-full right-4 w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-t-[6px] border-t-[#161b22]" />
              <div className="absolute top-full right-[15px] w-0 h-0 border-l-[7px] border-l-transparent border-r-[7px] border-r-transparent border-t-[7px] border-t-[#30363d] -mt-[1px]" />
            </div>
          </div>
        )}

        {/* GitHub API usage with tooltip */}
        {rateLimit && (
          <div className="relative group">
            <div 
              className={`px-3 py-1.5 rounded border text-xs font-medium ${rateLimitBgColor(getWorstPercentage(rateLimit))} border-[#30363d] cursor-pointer`}
            >
              <span className={rateLimitColor(getWorstPercentage(rateLimit))}>
                GitHub API usage: {Math.round(getWorstPercentage(rateLimit))}%
              </span>
            </div>
            
            {/* Tooltip */}
            <div className="absolute bottom-full right-0 mb-2 hidden group-hover:block w-[320px] bg-[#161b22] border border-[#30363d] rounded-md shadow-lg z-[1000]">
              <div className="p-3">
                <div className="text-[#e6edf3] font-semibold text-sm mb-2 pb-2 border-b border-[#30363d]">
                  GitHub API Rate Limits
                </div>
                
                <div className="space-y-2">
                  {/* Core API */}
                  {rateLimit.core && (
                    <div className="flex items-center gap-2 text-xs">
                      <span className="text-[#8b949e] w-[70px]">REST API:</span>
                      <span className={getApiLimitColor(getUsagePercentage(rateLimit.core))}>
                        {rateLimit.core.remaining}/{rateLimit.core.limit} ({Math.round(getUsagePercentage(rateLimit.core))}%)
                      </span>
                      <span className="text-[#8b949e] ml-auto">
                        {formatResetTime(rateLimit.core.reset)}
                      </span>
                    </div>
                  )}
                  
                  {/* GraphQL API */}
                  {rateLimit.graphql && (
                    <div className="flex items-center gap-2 text-xs">
                      <span className="text-[#8b949e] w-[70px]">GraphQL:</span>
                      <span className={getApiLimitColor(getUsagePercentage(rateLimit.graphql))}>
                        {rateLimit.graphql.remaining}/{rateLimit.graphql.limit} ({Math.round(getUsagePercentage(rateLimit.graphql))}%)
                      </span>
                      <span className="text-[#8b949e] ml-auto">
                        {formatResetTime(rateLimit.graphql.reset)}
                      </span>
                    </div>
                  )}
                  
                  {/* Search API */}
                  {rateLimit.search && (
                    <div className="flex items-center gap-2 text-xs">
                      <span className="text-[#8b949e] w-[70px]">Search:</span>
                      <span className={getApiLimitColor(getUsagePercentage(rateLimit.search))}>
                        {rateLimit.search.remaining}/{rateLimit.search.limit} ({Math.round(getUsagePercentage(rateLimit.search))}%)
                      </span>
                      <span className="text-[#8b949e] ml-auto">
                        {formatResetTime(rateLimit.search.reset)}
                      </span>
                    </div>
                  )}
                </div>
                
                <div className="mt-2 pt-2 border-t border-[#30363d] text-[10px] text-[#8b949e] italic">
                  Rate limits reset hourly. Click to refresh.
                </div>
              </div>
              
              {/* Arrow */}
              <div className="absolute top-full right-4 w-0 h-0 border-l-[6px] border-l-transparent border-r-[6px] border-r-transparent border-t-[6px] border-t-[#161b22]" />
              <div className="absolute top-full right-[15px] w-0 h-0 border-l-[7px] border-l-transparent border-r-[7px] border-r-transparent border-t-[7px] border-t-[#30363d] -mt-[1px]" />
            </div>
          </div>
        )}
      </div>
    </footer>
  )
}
