import { fetchAPI, postAPI } from './client'

export interface WorkerStatus {
  active: boolean
  paused: boolean
  step: string
  elapsed: number
  issue_id: number
  issue_title: string
}

export const settingsAPI = {
  getWorkerStatus: () => fetchAPI<WorkerStatus>('/worker-status'),
  toggleYolo: () => postAPI<{ success: boolean; yolo_mode: boolean }>('/yolo/toggle'),
}
