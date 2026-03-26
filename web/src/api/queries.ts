import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './client'

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export function useBoard() {
  return useQuery({
    queryKey: ['board'],
    queryFn: api.getBoard,
    refetchInterval: 30_000,
  })
}

export function useIssue(id: number, refetchInterval?: number) {
  return useQuery({
    queryKey: ['issue', id],
    queryFn: () => api.getIssue(id),
    enabled: id > 0,
    refetchInterval,
  })
}

export function useIssueSteps(id: number, refetchInterval?: number) {
  return useQuery({
    queryKey: ['issue-steps', id],
    queryFn: () => api.getIssueSteps(id),
    enabled: id > 0,
    refetchInterval,
  })
}

export function useSprintStatus() {
  return useQuery({
    queryKey: ['sprint'],
    queryFn: api.getSprintStatus,
    refetchInterval: 10_000,
  })
}

export function useWorkers() {
  return useQuery({
    queryKey: ['workers'],
    queryFn: api.getWorkers,
    refetchInterval: 5_000,
    select: (data) => data,
  })
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: api.getSettings,
  })
}

export function useRateLimit() {
  return useQuery({
    queryKey: ['rate-limit'],
    queryFn: api.getRateLimit,
    refetchInterval: 30_000,
  })
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export function useStartSprint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.startSprint,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sprint'] })
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function usePauseSprint() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.pauseSprint,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['sprint'] })
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useApproveIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.approveIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useRejectIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.rejectIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useRetryIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.retryIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useRetryFreshIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.retryFreshIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useApproveMergeIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.approveMergeIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useDeclineIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      api.declineIssue(id, reason),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useBlockIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.blockIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useUnblockIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.unblockIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useProcessIssue() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.processIssue,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useToggleWorkers() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.toggleWorkers,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['workers'] })
    },
  })
}

export function useSaveSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.saveSettings,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['settings'] })
    },
  })
}

export function useToggleYolo() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.toggleYolo,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['settings'] })
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}

export function useTriggerSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.triggerSync,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['board'] })
    },
  })
}
