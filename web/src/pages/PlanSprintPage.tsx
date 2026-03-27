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
      // Start proposal generation
      const response = await fetch('/api/v2/sprint/propose', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ targetCount }),
      })

      if (!response.ok) throw new Error('Failed to start proposal')
      const { jobId } = await response.json()

      // Poll for completion
      const pollProposal = async (): Promise<{ issues: ProposedIssue[]; branches: Branch[] }> => {
        const statusRes = await fetch(`/api/v2/sprint/propose/${jobId}`)
        const job: ProposalJob = await statusRes.json()

        if (job.status === 'failed') {
          throw new Error(job.error || 'Proposal generation failed')
        }

        if (job.status === 'completed' && job.proposal && job.branches) {
          return { issues: job.proposal, branches: job.branches }
        }

        // Wait and retry
        await new Promise((resolve) => setTimeout(resolve, 1000))
        return pollProposal()
      }

      const result = await pollProposal()
      setProposal(result.issues)
      setBranches(result.branches)

      // Select all by default
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

      // Read SSE stream
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
      <div style={{ maxWidth: '1200px', margin: '0 auto', padding: '20px' }}>
        <div style={{ textAlign: 'center', padding: '60px 20px' }}>
          <h2>Error</h2>
          <p>{error}</p>
          <button onClick={() => navigate('/')}>Back to Board</button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ maxWidth: '1200px', margin: '0 auto', padding: '20px' }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '20px',
          marginBottom: '30px',
          paddingBottom: '20px',
          borderBottom: '1px solid #e0e0e0',
        }}
      >
        <button
          onClick={() => navigate('/')}
          style={{
            padding: '8px 16px',
            background: '#f5f5f5',
            border: '1px solid #ddd',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          ← Back to Board
        </button>
        <h1 style={{ margin: 0, flex: 1 }}>Plan Sprint</h1>
        {sprint && (
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end' }}>
            <span style={{ fontWeight: 'bold', fontSize: '1.1em' }}>{sprint.title}</span>
            <span style={{ color: '#666', fontSize: '0.9em' }}>
              {new Date(sprint.start_date).toLocaleDateString()} -{' '}
              {new Date(sprint.end_date).toLocaleDateString()}
            </span>
          </div>
        )}
      </header>

      <main>
        <section
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '15px',
            marginBottom: '30px',
            padding: '20px',
            background: '#f9f9f9',
            borderRadius: '8px',
          }}
        >
          <label htmlFor="target-count" style={{ fontWeight: 500 }}>
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
            style={{ width: '80px', padding: '8px', border: '1px solid #ddd', borderRadius: '4px' }}
          />
          <button
            onClick={handleGenerate}
            disabled={isGenerating || isAssigning || !sprint}
            style={{
              padding: '10px 20px',
              background: isGenerating ? '#ccc' : '#4CAF50',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: isGenerating ? 'not-allowed' : 'pointer',
              fontWeight: 500,
            }}
          >
            {isGenerating ? 'Generating...' : 'Generate Proposal'}
          </button>
        </section>

        {error && (
          <div
            style={{
              background: '#ffebee',
              color: '#c62828',
              padding: '15px',
              borderRadius: '4px',
              marginBottom: '20px',
            }}
          >
            {error}
          </div>
        )}

        {isGenerating && (
          <div style={{ textAlign: 'center', padding: '40px' }}>
            <div
              style={{
                width: '40px',
                height: '40px',
                border: '4px solid #f3f3f3',
                borderTop: '4px solid #4CAF50',
                borderRadius: '50%',
                animation: 'spin 1s linear infinite',
                margin: '0 auto 20px',
              }}
            />
            <p>AI is selecting the best tickets for your sprint...</p>
            <p style={{ color: '#666', fontSize: '0.9em' }}>
              Analyzing dependencies and last release context
            </p>
          </div>
        )}

        {proposal.length > 0 && !isGenerating && (
          <section style={{ marginBottom: '30px' }}>
            <h2>Proposed Tickets</h2>
            <p style={{ color: '#666', fontSize: '0.9em', marginBottom: '20px', fontStyle: 'italic' }}>
              Branches are selected as a whole. Unchecking any issue removes the entire branch.
            </p>
            <DependencyTree
              nodes={treeNodes}
              selectedIssues={selectedIssues}
              onToggleBranch={handleToggleBranch}
            />
          </section>
        )}

        {isAssigning && assignmentProgress && (
          <section
            style={{
              background: '#f5f5f5',
              padding: '30px',
              borderRadius: '8px',
              textAlign: 'center',
              margin: '20px 0',
            }}
          >
            <h3>Assigning tickets to sprint...</h3>
            <div
              style={{
                width: '100%',
                height: '20px',
                background: '#e0e0e0',
                borderRadius: '10px',
                overflow: 'hidden',
                margin: '20px 0',
              }}
            >
              <div
                style={{
                  height: '100%',
                  background: '#4CAF50',
                  transition: 'width 0.3s ease',
                  width: `${(assignmentProgress.current / assignmentProgress.total) * 100}%`,
                }}
              />
            </div>
            <p>
              {assignmentProgress.current} / {assignmentProgress.total} tickets assigned
            </p>
            {assignmentProgress.branch && (
              <p style={{ color: '#666', fontStyle: 'italic' }}>
                Processing branch: {assignmentProgress.branch}...
              </p>
            )}
          </section>
        )}

        {proposal.length > 0 && !isAssigning && (
          <section
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '20px',
              background: '#f9f9f9',
              borderRadius: '8px',
              marginTop: '30px',
              position: 'sticky',
              bottom: '20px',
            }}
          >
            <div style={{ fontWeight: 500, color: overcommit ? '#ff9800' : 'inherit' }}>
              Selected: {selectedCount} / {targetCount} tickets ({targetPercentage}%)
              {overcommit && (
                <span style={{ fontSize: '0.9em', marginLeft: '8px' }}>(Over target)</span>
              )}
            </div>
            <button
              onClick={handleAssign}
              disabled={selectedIssues.size === 0}
              style={{
                padding: '12px 30px',
                background: selectedIssues.size === 0 ? '#ccc' : '#2196F3',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: selectedIssues.size === 0 ? 'not-allowed' : 'pointer',
                fontWeight: 500,
                fontSize: '1em',
              }}
            >
              Assign to Sprint
            </button>
          </section>
        )}
      </main>
    </div>
  )
}
