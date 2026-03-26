export interface TaskCard {
  id: number
  title: string
  status: string
  worker?: string
  assignee?: string
  labels: string[]
  pr_url?: string
  is_merged: boolean
}

export interface TaskInfo {
  number: number
  title: string
  status: string
  priority?: string
  type?: string
  size?: string
}

export interface BoardData {
  sprint_name: string
  paused: boolean
  processing: boolean
  can_close_sprint: boolean
  yolo_mode: boolean
  current_ticket?: TaskInfo
  blocked: TaskCard[]
  backlog: TaskCard[]
  plan: TaskCard[]
  code: TaskCard[]
  ai_review: TaskCard[]
  check_pipeline: TaskCard[]
  approve: TaskCard[]
  merge: TaskCard[]
  done: TaskCard[]
  failed: TaskCard[]
}

export interface SprintStatus {
  sprint_name: string
  paused: boolean
  processing: boolean
  can_close_sprint: boolean
}

export interface ActionResponse {
  success: boolean
  message?: string
  error?: string
}
