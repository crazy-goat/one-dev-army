import { fetchAPI, postAPI } from './client'
import type { TaskDetail } from '../types/task'
import type { ActionResponse } from '../types/board'

export const tasksAPI = {
  getTask: (id: number) => fetchAPI<TaskDetail>(`/tasks/${id}`),
  action: (id: number, action: string, body?: Record<string, string>) =>
    postAPI<ActionResponse>(`/tasks/${id}/actions/${action}`, body),
}
