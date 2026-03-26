import { fetchAPI, postAPI } from './client'
import type { SprintStatus, ActionResponse } from '../types/board'

export const sprintAPI = {
  getSprint: () => fetchAPI<SprintStatus>('/sprint'),
  action: (action: 'start' | 'pause' | 'close') =>
    postAPI<ActionResponse>(`/sprint/${action}`),
}
