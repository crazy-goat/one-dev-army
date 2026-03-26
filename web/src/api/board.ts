import { fetchAPI } from './client'
import type { BoardData } from '../types/board'

export const boardAPI = {
  getBoard: () => fetchAPI<BoardData>('/board'),
}
