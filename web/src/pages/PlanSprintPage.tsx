import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router'
import type { Sprint, ProposedIssue, ProposalJob, AssignmentProgress, TreeNode, Branch } from '../types/sprint'
import DependencyTree from '../components/DependencyTree'

export default function PlanSprintPage() {
  const navigate = useNavigate()
  const [sprint, setSprint] = useState<Sprint | null>(null)
  const [targetCount, setTargetCount] = useState<number>(10)
  const [proposal, setProposal] = useState<ProposedIssue[]>([])
  const [branches, setBranches] = useState<Branch[]>([])
  const [selectedIssues, setSelectedIssues] = useState<Set<number>>(new Set())
  const [selectedBranches, setSelectedBranches] = useState<Set<string>>(new Set())
  const [isGenerating, setIsGenerating] = useState(false)
  const [isAssigning, setIsAssigning] = useState(false)
  const [assignmentProgress, setAssignmentProgress] = useState<AssignmentProgress | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Load current sprint on mount
  useEffect(() => {
    fetch('/api/v2/sprint/current')
      .then((res) => {
        if (!res.ok) throw new Error('No active sprint')
        return res.json()
      })
      .then(setSprint)
      .catch((err) => setError(err.message))
  }, [])

  const buildTree = (issues: ProposedIssue[], branches: Branch[]): TreeNode[] => {
    const issueMap = new Map(issues.map((i) => [i.number, i]))
    const treeNodes: TreeNode[] = []

    branches.forEach((branch) => {
      const rootIssue = issueMap.get(branch.root_issue)
      if (!rootIssue) return

      const buildBranchTree = (issueNum: number, branchId: string): TreeNode | null => {
        const issue = issueMap.get(issueNum)
        if (!issue) return null

        const children = issue.dependencies
          .map((dep) => buildBranchTree(dep, branchId))
          .filter((child): child is TreeNode => child !== null)

        return {
          issue,
          children,
          branch: branchId,
        }
      }

      const rootNode = buildBranchTree(branch.root_issue, branch.id)
      if (rootNode) {
        treeNodes.push(rootNode)
      }
    })

    return treeNodes
  }

  const handleGenerate = async () => {
    setIsGenerating(true)
    setError(null)

    try {
      const response = await fetch('/api/v2/sprint/propose', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targetCount }),
      })

      if (!response.ok) throw new Error('Failed to start proposal')
      const { jobId } = await response.json()

      const pollProposal = async (): Promise<{ issues: ProposedIssue[]; branches: Branch[] }> => {
        const statusRes = await fetch(`/api/v2/sprint/propose/${jobId}`)
        const job: ProposalJob = await statusRes.json()

        if (job.status === 'failed') {
          throw new Error(job.error || 'Proposal generation failed')
        }

        if (job.status === 'completed' && job.proposal && job.branches) {
          return { issues: job.proposal, branches: job.branches }
        }

        await new Promise((resolve) => setTimeout(resolve, 1000))
        return pollProposal()
      }

      const result = await pollProposal()
      setProposal(result.issues)
      setBranches(result.branches)

      const allIssueIds = new Set(result.issues.map((p) => p.number))
      const allBranchIds = new Set(result.branches.map((b) => b.id))
      setSelectedIssues(allIssueIds)
      setSelectedBranches(allBranchIds)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setIsGenerating(false)
    }
  }

  const handleToggleBranch = (branchId: string, selected: boolean) => {
    const branch = branches.find((b) => b.id === branchId)
    if (!branch) return

    setSelectedBranches((prev) => {
      const next = new Set(prev)
      if (selected) {
        next.add(branchId)
      } else {
        next.delete(branchId)
      }
      return next
    })

    setSelectedIssues((prev) => {
      const next = new Set(prev)
      branch.issues.forEach((issueNum) => {
        if (selected) {
          next.add(issueNum)
        } else {
          next.delete(issueNum)
        }
      })
      return next
    })
  }

  const handleAssign = async () => {
    setIsAssigning(true)
    setError(null)

    const issueNumbers = Array.from(selectedIssues)
    const branchIds = Array.from(selectedBranches)

    try {
      const response = await fetch('/api/v2/sprint/assign', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ issueNumbers, branches: branchIds }),
      })

      if (!response.ok) throw new Error('Failed to start assignment')

      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (!reader) throw new Error('No response body')

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const chunk = decoder.decode(value)
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data: AssignmentProgress = JSON.parse(line.slice(6))
            setAssignmentProgress(data)

            if (data.type === 'completed') {
              setTimeout(() => navigate('/'), 1000)
              return
            }

            if (data.type === 'error') {
              throw new Error(data.error || 'Assignment failed')
            }
          }
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      setIsAssigning(false)
    }
  }

  const treeNodes = buildTree(proposal, branches)
  const selectedCount = selectedIssues.size
  const targetPercentage = Math.round((selectedCount / targetCount) * 100)
  const overcommit = targetPercentage > 100

  if (error && !sprint) {
    return (
      <div className="flex items-center justify-center flex-1 py-20">
        <div className="text-center">
          <h2 className="text-xl font-bold text-white mb-2">Error</h2>
          <p className="text-gray-400 mb-4">{error}</p>
          <button
            onClick={() => navigate('/')}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            Back to Board
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-5xl mx-auto p-4 pb-12">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-4">
          <button
            onClick={() => navigate('/')}
            className="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 rounded-lg text-sm transition-colors"
          >
            ← Back to Board
          </button>
          <h1 className="text-xl font-bold text-white">Plan Sprint</h1>
        </div>
        {sprint && (
          <div className="text-right">
            <div className="text-white font-semibold">{sprint.title}</div>
            <div className="text-sm text-gray-500">
              {new Date(sprint.start_date).toLocaleDateString()} - {new Date(sprint.end_date).toLocaleDateString()}
            </div>
          </div>
        )}
      </div>

      {/* Error message */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 text-red-400 p-3 rounded-lg mb-4 text-sm">
          {error}
        </div>
      )}

      {/* Configuration Section */}
      <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
        <div className="flex items-center gap-4">
          <label htmlFor="target-count" className="text-gray-300 font-medium">
            Target ticket count:
          </label>
          <input
            id="target-count"
            type="number"
            min={1}
            max={50}
            value={targetCount}
            onChange={(e) => setTargetCount(parseInt(e.target.value) || 10)}
            disabled={isGenerating || isAssigning}
            className="w-20 px-3 py-2 bg-gray-950 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:border-blue-500 disabled:opacity-50"
          />
          <button
            onClick={handleGenerate}
            disabled={isGenerating || isAssigning || !sprint}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isGenerating ? (
              <span className="flex items-center gap-2">
                <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Generating...
              </span>
            ) : (
              'Generate Proposal'
            )}
          </button>
        </div>
      </section>

      {/* Loading State */}
      {isGenerating && (
        <div className="flex flex-col items-center justify-center py-12">
          <div className="w-10 h-10 border-3 border-gray-700 border-t-blue-500 rounded-full animate-spin mb-4" />
          <p className="text-gray-300 font-medium">AI is selecting the best tickets for your sprint...</p>
          <p className="text-sm text-gray-500 mt-1">Analyzing dependencies and last release context</p>
        </div>
      )}

      {/* Proposal Results */}
      {proposal.length > 0 && !isGenerating && (
        <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
          <h2 className="text-lg font-semibold text-blue-400 mb-1">Proposed Tickets</h2>
          <p className="text-sm text-gray-500 mb-4">
            Branches are selected as a whole. Unchecking any issue removes the entire branch.
          </p>
          <DependencyTree
            nodes={treeNodes}
            selectedIssues={selectedIssues}
            onToggleBranch={handleToggleBranch}
          />
        </section>
      )}

      {/* Assignment Progress */}
      {isAssigning && assignmentProgress && (
        <section className="bg-gray-900 border border-gray-800 rounded-lg p-6 mb-6">
          <h3 className="text-lg font-semibold text-white mb-4">Assigning tickets to sprint...</h3>
          <div className="w-full h-2 bg-gray-800 rounded-full overflow-hidden mb-3">
            <div
              className="h-full bg-blue-500 transition-all duration-300"
              style={{ width: `${(assignmentProgress.current / assignmentProgress.total) * 100}%` }}
            />
          </div>
          <p className="text-gray-400 text-sm">
            {assignmentProgress.current} / {assignmentProgress.total} tickets assigned
          </p>
          {assignmentProgress.branch && (
            <p className="text-sm text-gray-500 mt-1">
              Processing branch: {assignmentProgress.branch}...
            </p>
          )}
        </section>
      )}

      {/* Action Bar */}
      {proposal.length > 0 && !isAssigning && (
        <div className="sticky bottom-4 bg-gray-900 border border-gray-800 rounded-lg p-4 flex items-center justify-between">
          <div className={`font-medium ${overcommit ? 'text-yellow-400' : 'text-gray-300'}`}>
            Selected: {selectedCount} / {targetCount} tickets ({targetPercentage}%)
            {overcommit && <span className="text-sm ml-2">(Over target)</span>}
          </div>
          <button
            onClick={handleAssign}
            disabled={selectedIssues.size === 0}
            className="px-6 py-2 bg-green-600 hover:bg-green-500 text-white rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Assign to Sprint
          </button>
        </div>
      )}
    </div>
  )
}
