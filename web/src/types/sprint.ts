// Sprint planning types

export interface Sprint {
  number: number
  title: string
  state: 'open' | 'closed'
  start_date: string
  end_date: string
  ticket_count: number
}

export interface IssueCandidate {
  number: number
  title: string
  labels: string[]
  complexity?: number
  priority?: 'high' | 'medium' | 'low'
  type?: 'bug' | 'feature' | 'docs' | 'other'
}

export interface Branch {
  id: string
  name: string
  root_issue: number
  issues: number[]
  total_complexity: number
}

export interface ProposedIssue extends IssueCandidate {
  reason: string
  dependencies: number[]
  branch: string
}

export interface ProposalJob {
  id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  proposal?: ProposedIssue[]
  branches?: Branch[]
  error?: string
}

export interface AssignmentProgress {
  type: 'progress' | 'completed' | 'error'
  current: number
  total: number
  issue?: number
  branch?: string
  error?: string
}

export interface TreeNode {
  issue: ProposedIssue
  children: TreeNode[]
  branch: string
}

export interface LastTagInfo {
  tag: string
  date?: string
  sha?: string
}
