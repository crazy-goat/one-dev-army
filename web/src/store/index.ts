import { create } from 'zustand'
import type { BoardData, TaskCard } from '../types/board'

interface WorkerStatus {
  active: boolean
  paused: boolean
  step: string
  issue_id: number
  issue_title: string
}

interface Store {
  // Board state
  board: BoardData | null
  isLoading: boolean
  error: string | null

  // Worker state
  wsConnected: boolean
  workerStatus: WorkerStatus | null

  // Actions
  setBoard: (board: BoardData) => void
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  updateTaskInColumn: (taskId: number, updates: Partial<TaskCard>) => void
  setWsConnected: (connected: boolean) => void
  setWorkerStatus: (status: WorkerStatus | null) => void
}

export const useStore = create<Store>((set) => ({
  board: null,
  isLoading: false,
  error: null,
  wsConnected: false,
  workerStatus: null,

  setBoard: (board) => set({ board }),
  setLoading: (isLoading) => set({ isLoading }),
  setError: (error) => set({ error }),
  updateTaskInColumn: (_taskId, _updates) =>
    set((state) => {
      // For now, just trigger a re-render. Full board refresh via SWR is simpler.
      return { board: state.board ? { ...state.board } : null }
    }),
  setWsConnected: (wsConnected) => set({ wsConnected }),
  setWorkerStatus: (workerStatus) => set({ workerStatus }),
}))
