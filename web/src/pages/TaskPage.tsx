import { useParams, Link } from 'react-router-dom'
import useSWR from 'swr'
import { Layout } from '../components/Layout/Layout'
import { StepList } from '../components/Task/StepList'
import { TaskActions } from '../components/Task/TaskActions'
import { tasksAPI } from '../api/tasks'
import type { TaskDetail } from '../types/task'

export function TaskPage() {
  const { id } = useParams<{ id: string }>()
  const { data: task, error, isLoading, mutate } = useSWR<TaskDetail>(
    id ? `task-${id}` : null,
    () => tasksAPI.getTask(parseInt(id!)),
    { refreshInterval: (currentData) => currentData?.is_active ? 3000 : 0 },
  )

  if (isLoading) {
    return (
      <Layout>
        <div style={{ color: 'var(--muted)', padding: '2rem', textAlign: 'center' }}>
          Loading task...
        </div>
      </Layout>
    )
  }

  if (error) {
    return (
      <Layout>
        <div style={{ color: 'var(--red)', padding: '2rem', textAlign: 'center' }}>
          Error loading task: {error.message}
        </div>
      </Layout>
    )
  }

  if (!task) {
    return (
      <Layout>
        <div style={{ color: 'var(--muted)', padding: '2rem', textAlign: 'center' }}>
          Task not found
        </div>
      </Layout>
    )
  }

  return (
    <Layout>
      <div style={{ maxWidth: '900px', margin: '0 auto' }}>
        <div style={{ marginBottom: '1rem' }}>
          <Link to="/" style={{ color: 'var(--muted)', fontSize: '.85rem' }}>
            &larr; Back to Board
          </Link>
        </div>

        <div style={{ marginBottom: '1.5rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '.75rem', marginBottom: '.5rem' }}>
            <span style={{ color: 'var(--muted)', fontSize: '1.1rem' }}>#{task.issue_number}</span>
            <h1 style={{ fontSize: '1.4rem', margin: 0 }}>{task.issue_title}</h1>
          </div>

          <div style={{ display: 'flex', gap: '.5rem', alignItems: 'center' }}>
            {task.status && (
              <span style={{
                fontSize: '.75rem',
                padding: '.2rem .6rem',
                borderRadius: '4px',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
              }}>
                {task.status}
              </span>
            )}
            {task.is_active && (
              <span style={{
                fontSize: '.75rem',
                padding: '.2rem .6rem',
                borderRadius: '4px',
                background: 'rgba(52,152,219,0.15)',
                border: '1px solid rgba(52,152,219,0.3)',
                color: 'var(--accent)',
              }}>
                Active
              </span>
            )}
          </div>
        </div>

        {/* Task Actions */}
        {task.status && (
          <div style={{ marginBottom: '1.5rem' }}>
            <TaskActions
              issueNumber={task.issue_number}
              status={task.status}
              onActionComplete={() => mutate()}
            />
          </div>
        )}

        {/* Steps */}
        <h2 style={{ fontSize: '1.1rem', color: 'var(--muted)', marginBottom: '.75rem' }}>
          Processing Steps
        </h2>
        <StepList
          steps={task.steps}
          isActive={task.is_active}
          issueNumber={task.issue_number}
        />
      </div>
    </Layout>
  )
}
