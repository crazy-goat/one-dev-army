import type { TreeNode } from '../types/sprint'

interface DependencyTreeProps {
  nodes: TreeNode[]
  selectedIssues: Set<number>
  onToggleBranch: (branchId: string, selected: boolean) => void
}

export default function DependencyTree({
  nodes,
  selectedIssues,
  onToggleBranch,
}: DependencyTreeProps) {
  const renderNode = (node: TreeNode, level: number = 0) => {
    const isSelected = selectedIssues.has(node.issue.number)
    const hasChildren = node.children.length > 0
    const isRoot = level === 0

    return (
      <div
        key={node.issue.number}
        className={`mb-2 ${level > 0 ? 'ml-5' : ''}`}
      >
        <label
          className={`flex items-start gap-3 p-4 rounded-lg cursor-pointer transition-colors ${
            isSelected ? 'bg-blue-500/10 border border-blue-500/30' : 'bg-gray-950 border border-gray-800 hover:border-gray-700'
          } ${isRoot ? 'border-l-4 border-l-blue-500' : ''}`}
        >
          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => onToggleBranch(node.branch, !isSelected)}
            className="mt-1 w-4 h-4 rounded border-gray-600 text-blue-500 focus:ring-blue-500 focus:ring-offset-0 bg-gray-800"
          />
          <span className="text-lg">{getTypeIcon(node.issue.labels)}</span>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-gray-500 text-sm font-medium">#{node.issue.number}</span>
              <span className="text-white font-medium truncate">{node.issue.title}</span>
            </div>
            <div className="flex flex-wrap gap-2 mb-2">
              {node.issue.labels.map((label) => (
                <span
                  key={label}
                  className="px-2 py-0.5 bg-blue-500/10 text-blue-400 text-xs rounded-full"
                >
                  {label}
                </span>
              ))}
              {node.issue.complexity && (
                <span className="px-2 py-0.5 bg-yellow-500/10 text-yellow-400 text-xs rounded-full">
                  Complexity: {node.issue.complexity}
                </span>
              )}
            </div>
            <p className="text-sm text-gray-500 italic">{node.issue.reason}</p>
          </div>
        </label>
        {hasChildren && (
          <div className="mt-2">
            {node.children.map((child) => renderNode(child, level + 1))}
          </div>
        )}
      </div>
    )
  }

  const getTypeIcon = (labels: string[]) => {
    if (labels.includes('type:bug')) return '🐛'
    if (labels.includes('type:feature')) return '✨'
    if (labels.includes('type:docs')) return '📚'
    return '📝'
  }

  return (
    <div className="space-y-2">
      {nodes.map((node) => renderNode(node))}
    </div>
  )
}
