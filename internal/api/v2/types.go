package v2

// TaskCard represents a task on the board.
type TaskCard struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Worker   string   `json:"worker,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Labels   []string `json:"labels"`
	PRURL    string   `json:"pr_url,omitempty"`
	IsMerged bool     `json:"is_merged"`
}

// TaskInfo represents the currently processing task.
type TaskInfo struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
	Type     string `json:"type,omitempty"`
	Size     string `json:"size,omitempty"`
}

// BoardResponse is the full board state returned by GET /api/v2/board.
type BoardResponse struct {
	SprintName     string     `json:"sprint_name"`
	Paused         bool       `json:"paused"`
	Processing     bool       `json:"processing"`
	CanCloseSprint bool       `json:"can_close_sprint"`
	YoloMode       bool       `json:"yolo_mode"`
	CurrentTicket  *TaskInfo  `json:"current_ticket,omitempty"`
	Blocked        []TaskCard `json:"blocked"`
	Backlog        []TaskCard `json:"backlog"`
	Plan           []TaskCard `json:"plan"`
	Code           []TaskCard `json:"code"`
	AIReview       []TaskCard `json:"ai_review"`
	CheckPipeline  []TaskCard `json:"check_pipeline"`
	Approve        []TaskCard `json:"approve"`
	Merge          []TaskCard `json:"merge"`
	Done           []TaskCard `json:"done"`
	Failed         []TaskCard `json:"failed"`
}

// SprintResponse is returned by GET /api/v2/sprint.
type SprintResponse struct {
	SprintName     string `json:"sprint_name"`
	Paused         bool   `json:"paused"`
	Processing     bool   `json:"processing"`
	CanCloseSprint bool   `json:"can_close_sprint"`
}

// TaskDetailResponse is returned by GET /api/v2/tasks/{id}.
type TaskDetailResponse struct {
	IssueNumber int        `json:"issue_number"`
	IssueTitle  string     `json:"issue_title"`
	Status      string     `json:"status"`
	IsActive    bool       `json:"is_active"`
	Steps       []StepInfo `json:"steps"`
}

// StepInfo represents a single processing step.
type StepInfo struct {
	ID         int64   `json:"id"`
	StepName   string  `json:"step_name"`
	Status     string  `json:"status"`
	Prompt     string  `json:"prompt,omitempty"`
	Response   string  `json:"response,omitempty"`
	ErrorMsg   string  `json:"error_msg,omitempty"`
	StartedAt  *string `json:"started_at,omitempty"`
	FinishedAt *string `json:"finished_at,omitempty"`
}

// WorkerStatusResponse is returned by GET /api/v2/worker-status.
type WorkerStatusResponse struct {
	Active     bool   `json:"active"`
	Paused     bool   `json:"paused"`
	Step       string `json:"step"`
	Elapsed    int    `json:"elapsed"`
	IssueID    int    `json:"issue_id"`
	IssueTitle string `json:"issue_title"`
}

// ActionResponse is returned by POST action endpoints.
type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RateLimitInfo represents a single rate limit category.
type RateLimitInfo struct {
	Name      string  `json:"name"`
	Limit     int     `json:"limit"`
	Remaining int     `json:"remaining"`
	Used      float64 `json:"used_percentage"`
	ResetAt   string  `json:"reset_at"`
}

// RateLimitResponse is returned by GET /api/v2/rate-limit.
type RateLimitResponse struct {
	Core    *RateLimitInfo `json:"core,omitempty"`
	GraphQL *RateLimitInfo `json:"graphql,omitempty"`
	Search  *RateLimitInfo `json:"search,omitempty"`
}

// SettingsResponse is returned by GET /api/v2/settings.
type SettingsResponse struct {
	LLM      LLMSettingsResponse `json:"llm"`
	YoloMode bool                `json:"yolo_mode"`
}

// LLMSettingsResponse contains LLM model configuration.
type LLMSettingsResponse struct {
	Setup         string `json:"setup"`
	Planning      string `json:"planning"`
	Orchestration string `json:"orchestration"`
	Code          string `json:"code"`
	CodeHeavy     string `json:"code_heavy"`
}
