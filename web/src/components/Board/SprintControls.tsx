import useSWR from 'swr'
import { sprintAPI } from '../../api/sprint'

export function SprintControls() {
  const { data: sprint, mutate } = useSWR('sprint', sprintAPI.getSprint, {
    refreshInterval: 5000,
  })

  const handleAction = async (action: 'start' | 'pause' | 'close') => {
    await sprintAPI.action(action)
    mutate()
  }

  if (!sprint) return null

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: '.75rem',
      flexShrink: 0,
    }}>
      {sprint.sprint_name && (
        <span style={{ fontWeight: 600, fontSize: '1rem' }}>
          {sprint.sprint_name}
        </span>
      )}

      <div style={{ display: 'flex', gap: '.5rem' }}>
        {sprint.paused ? (
          <button onClick={() => handleAction('start')} className="btn btn-success">
            Start Sprint
          </button>
        ) : (
          <button onClick={() => handleAction('pause')} className="btn btn-danger">
            Pause Sprint
          </button>
        )}

        {sprint.can_close_sprint && (
          <button onClick={() => handleAction('close')} className="btn btn-success">
            Close Sprint
          </button>
        )}
      </div>
    </div>
  )
}
