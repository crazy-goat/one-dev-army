import { useRateLimit } from '../../api/queries'

export function Footer() {
  const { data: rateLimit } = useRateLimit()

  return (
    <footer className="bg-gray-900 border-t border-gray-800 px-4 py-2 mt-auto">
      <div className="flex items-center justify-between max-w-screen-2xl mx-auto text-xs text-gray-500">
        <div className="flex items-center gap-4">
          <span>ODA — One Dev Army</span>
          <a
            href="/"
            className="text-blue-400 hover:text-blue-300 transition-colors"
          >
            ← Classic dashboard
          </a>
        </div>
        {rateLimit && (
          <span>
            GitHub API: {rateLimit.remaining}/{rateLimit.limit}
          </span>
        )}
      </div>
    </footer>
  )
}
