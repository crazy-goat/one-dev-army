// =============================================================================
// TypeScript types matching Go API v2 response types
// All field names use snake_case to match JSON serialization from the Go backend
// =============================================================================

// Board
export interface Board {
  sprint_name: string
  paused: boolean
  processing: boolean
  can_close_sprint: boolean
  can_plan_sprint: boolean
  yolo_mode: boolean
  total_tickets: number
  worker_count: number
  opencode_port: number
  current_ticket?: CurrentTicket
  columns: Record<string, Card[]>
}

export interface CurrentTicket {
  number: number
  title: string
  status: string
  priority?: string
  type?: string
  size?: string
}

export interface Card {
  id: number
  title: string
  status: string
  worker?: string
  assignee?: string
  labels: string[]
  pr_url?: string
  is_merged: boolean
}

// Issue Detail
export interface IssueDetail {
  issue_number: number
  issue_title: string
  is_active: boolean
  status?: string
  steps: TaskStep[]
}

export interface TaskStep {
  id: number
  issue_number: number
  step_name: string
  status: string
  prompt?: string
  response?: string
  error_msg?: string
  session_id?: string
  plan_attachment_url?: string
  llm_model?: string
  started_at?: string
  finished_at?: string
}

// Sprint
export interface SprintStatus {
  paused: boolean
  processing: boolean
  current_issue?: number
  current_status?: string
}

export interface SprintClosePreview {
  tag_name: string
  current_version: string
  new_version: string
  bump_type: string
  release_title: string
  release_body: string
  llm_generated: boolean
  closed_issues: ClosedIssue[]
}

export interface ClosedIssue {
  number: number
  title: string
}

export interface SprintCloseResult {
  success: boolean
  tag_name: string
  release_title: string
  release_url: string
  milestone_title: string
  new_sprint_title?: string
  warning?: string
}

// Workers
export interface WorkersResponse {
  workers: WorkerInfo[]
  paused: boolean
  active: boolean
}

export interface WorkerInfo {
  id: string
  status: string
  task_id?: number
  task_title?: string
  stage?: string
  elapsed_ms?: number
}

// Settings
export interface Settings {
  config: LLMConfig
  yolo_mode: boolean
  sprint_auto_start: boolean
  available_models: ProviderModel[]
}

export interface LLMConfig {
  Setup: ModelConfig
  Planning: ModelConfig
  Orchestration: ModelConfig
  Code: ModelConfig
  CodeHeavy: ModelConfig
}

export interface ModelConfig {
  Model: string
}

export interface ProviderModel {
  id: string
  name: string
  provider_id: string
}

// Wizard
export interface WizardSession {
  id: string
  type: string
  current_step: string
  idea_text?: string
  refined_description?: string
  technical_planning?: string
  generated_title?: string
  custom_title?: string
  use_custom_title: boolean
  priority?: string
  complexity?: string
  created_issues?: CreatedIssue[]
  add_to_sprint: boolean
  language?: string
}

export interface CreatedIssue {
  number: number
  title: string
  url: string
  is_epic: boolean
  success: boolean
  error?: string
}

export interface LLMLogEntry {
  timestamp: string
  role: string
  message: string
}

// Rate Limit
export interface APILimit {
  name: string
  limit: number
  remaining: number
  reset: number
  updated_at: string
}

export interface RateLimit {
  core: APILimit | null
  graphql: APILimit | null
  search: APILimit | null
  updated_at: string
  error?: string
}

// WebSocket Messages
export type WSMessageType =
  | 'issue_update'
  | 'sync_complete'
  | 'worker_update'
  | 'can_close_sprint'
  | 'log_stream'
  | 'ping'
  | 'pong'

export interface WSMessage {
  type: WSMessageType
  payload?: unknown
}

export interface IssueUpdatePayload {
  number: number
  title: string
  state: string
  column: string
  is_merged: boolean
}

export interface SyncCompletePayload {
  count: number
}

export interface WorkerUpdatePayload {
  worker_id: string
  status: string
  task_id: number
  task_title: string
  stage: string
  elapsed_seconds: number
}

export interface SprintClosablePayload {
  can_close: boolean
}

export interface LogStreamPayload {
  issue_number: number
  step: string
  timestamp: string
  message: string
  level: string
  file: string
}

// API Error
export interface ApiError {
  error: string
}

// Success response
export interface SuccessResponse {
  success: boolean
  message?: string
  error?: string
}
