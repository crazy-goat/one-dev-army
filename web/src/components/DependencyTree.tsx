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
        className={`tree-node level-${level}`}
        style={{ marginLeft: level * 20 }}
      >
        <label
          className={`tree-node-label ${isSelected ? 'selected' : ''}`}
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            gap: '12px',
            padding: '15px',
            cursor: 'pointer',
            backgroundColor: isSelected ? '#e3f2fd' : 'transparent',
            borderLeft: isRoot ? '4px solid #2196F3' : `${3 - level}px solid #90caf9`,
            borderRadius: '8px',
            marginBottom: '8px',
          }}
        >
          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => onToggleBranch(node.branch, !isSelected)}
            style={{ marginTop: '4px' }}
          />
          <span style={{ fontSize: '1.2em' }}>{getTypeIcon(node.issue.labels)}</span>
          <div style={{ flex: 1 }}>
            <div>
              <span style={{ color: '#666', fontWeight: 500, marginRight: '8px' }}>
                #{node.issue.number}
              </span>
              <span style={{ fontWeight: 500 }}>{node.issue.title}</span>
            </div>
            <div style={{ display: 'flex', gap: '8px', marginTop: '8px', flexWrap: 'wrap' }}>
              {node.issue.labels.map((label) => (
                <span
                  key={label}
                  style={{
                    background: '#e3f2fd',
                    color: '#1976d2',
                    padding: '2px 8px',
                    borderRadius: '12px',
                    fontSize: '0.85em',
                  }}
                >
                  {label}
                </span>
              ))}
              {node.issue.complexity && (
                <span
                  style={{
                    background: '#fff3e0',
                    color: '#f57c00',
                    padding: '2px 8px',
                    borderRadius: '12px',
                    fontSize: '0.85em',
                  }}
                >
                  Complexity: {node.issue.complexity}
                </span>
              )}
            </div>
            <p
              style={{
                margin: '8px 0 0 0',
                color: '#666',
                fontSize: '0.9em',
                fontStyle: 'italic',
              }}
            >
              {node.issue.reason}
            </p>
          </div>
        </label>
        {hasChildren && (
          <div style={{ marginTop: '8px' }}>
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
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      {nodes.map((node) => renderNode(node))}
    </div>
  )
}
