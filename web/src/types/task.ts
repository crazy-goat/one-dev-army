export interface TaskStep {
  id: number
  step_name: string
  status: string
  prompt?: string
  response?: string
  error_msg?: string
  started_at?: string
  finished_at?: string
}

export interface TaskDetail {
  issue_number: number
  issue_title: string
  status: string
  is_active: boolean
  steps: TaskStep[]
}
