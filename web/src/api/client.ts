import type {
  Board,
  IssueDetail,
  TaskStep,
  SprintStatus,
  WorkerInfo,
  Settings,
  WizardSession,
  LLMLogEntry,
  RateLimit,
  SprintClosePreview,
  SprintCloseResult,
  SuccessResponse,
  CreatedIssue,
} from './types'

const BASE = '/api/v2'

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText })) as { error: string }
    throw new ApiError(res.status, body.error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: 'PUT', body: JSON.stringify(body) })
}

function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

export const api = {
  // Board & Issues
  getBoard: () => request<Board>('/board'),
  getIssue: (id: number) => request<IssueDetail>(`/issues/${id}`),
  getIssueSteps: (id: number) =>
    request<{ issue_number: number; steps: TaskStep[] }>(`/issues/${id}/steps`).then(
      (res) => res.steps,
    ),

  // Sprint
  getSprintStatus: () => request<SprintStatus>('/sprint/status'),
  startSprint: () => post<SuccessResponse>('/sprint/start'),
  pauseSprint: () => post<SuccessResponse>('/sprint/pause'),
  planSprint: () => post<SuccessResponse>('/sprint/plan'),
  previewSprintClose: (bumpType: string) =>
    post<SprintClosePreview>('/sprint/close/preview', { bump_type: bumpType }),
  confirmSprintClose: (data: {
    bump_type: string
    release_title: string
    release_body: string
  }) => post<SprintCloseResult>('/sprint/close/confirm', data),

  // Ticket Actions
  approveIssue: (id: number) =>
    post<SuccessResponse>(`/issues/${id}/approve`),
  rejectIssue: (id: number) => post<SuccessResponse>(`/issues/${id}/reject`),
  retryIssue: (id: number) => post<SuccessResponse>(`/issues/${id}/retry`),
  retryFreshIssue: (id: number) =>
    post<SuccessResponse>(`/issues/${id}/retry-fresh`),
  approveMergeIssue: (id: number) =>
    post<SuccessResponse>(`/issues/${id}/approve-merge`),
  declineIssue: (id: number, reason: string) =>
    post<SuccessResponse>(`/issues/${id}/decline`, { reason }),
  blockIssue: (id: number) => post<SuccessResponse>(`/issues/${id}/block`),
  unblockIssue: (id: number) =>
    post<SuccessResponse>(`/issues/${id}/unblock`),
  processIssue: (id: number) =>
    post<SuccessResponse>(`/issues/${id}/process`),

  // Workers
  getWorkers: () =>
    request<{ workers: WorkerInfo[]; paused: boolean; active: boolean }>('/workers'),
  toggleWorkers: () => post<SuccessResponse>('/workers/toggle'),

  // Settings
  getSettings: () => request<Settings>('/settings'),
  saveSettings: (config: unknown) =>
    put<SuccessResponse>('/settings', config),
  toggleYolo: () => post<{ yolo_mode: boolean }>('/settings/yolo'),

  // Sync
  triggerSync: () => post<SuccessResponse>('/sync'),

  // Rate Limit
  getRateLimit: () => request<RateLimit>('/rate-limit'),

  // Wizard
  createWizardSession: (type: string) =>
    post<WizardSession>('/wizard/sessions', { type }),
  getWizardSession: (id: string) =>
    request<WizardSession>(`/wizard/sessions/${id}`),
  deleteWizardSession: (id: string) =>
    del<SuccessResponse>(`/wizard/sessions/${id}`),
  refineWizardSession: (id: string, data: { idea: string; language?: string }) =>
    post<WizardSession>(`/wizard/sessions/${id}/refine`, data),
  createWizardIssue: (
    id: string,
    data: { title?: string; add_to_sprint?: boolean },
  ) =>
    post<{ success: boolean; issue: CreatedIssue }>(
      `/wizard/sessions/${id}/create`,
      data,
    ),
  getWizardLogs: (id: string) =>
    request<LLMLogEntry[]>(`/wizard/sessions/${id}/logs`),
}
