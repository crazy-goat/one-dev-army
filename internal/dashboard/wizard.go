package dashboard

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// WizardType represents the type of wizard being run
type WizardType string

const (
	WizardTypeFeature WizardType = "feature"
	WizardTypeBug     WizardType = "bug"
)

// WizardStep represents the current step in the wizard flow
type WizardStep string

const (
	WizardStepNew       WizardStep = "new"
	WizardStepRefine    WizardStep = "refine"
	WizardStepBreakdown WizardStep = "breakdown"
	WizardStepCreate    WizardStep = "create"
	WizardStepDone      WizardStep = "done"
)

// LLMLogEntry represents a single log entry from LLM interactions
type LLMLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"` // "system", "user", "assistant"
	Message   string    `json:"message"`
}

// WizardTask represents a single task parsed from LLM breakdown
type WizardTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`   // "low", "medium", "high", "critical"
	Complexity  string `json:"complexity"` // "S", "M", "L", "XL"
}

// WizardSession holds the state for a single wizard instance
type WizardSession struct {
	ID                 string        `json:"id"`
	Type               WizardType    `json:"type"`
	CurrentStep        WizardStep    `json:"current_step"`
	IdeaText           string        `json:"idea_text"`
	RefinedDescription string        `json:"refined_description"`
	Tasks              []WizardTask  `json:"tasks"`
	LLMLogs            []LLMLogEntry `json:"llm_logs"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	mu                 sync.RWMutex  `json:"-"`
}

// AddLog adds a new log entry to the session (thread-safe)
func (s *WizardSession) AddLog(role, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LLMLogs = append(s.LLMLogs, LLMLogEntry{
		Timestamp: time.Now(),
		Role:      role,
		Message:   message,
	})
	s.UpdatedAt = time.Now()
}

// SetStep updates the current step (thread-safe)
func (s *WizardSession) SetStep(step WizardStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentStep = step
	s.UpdatedAt = time.Now()
}

// SetRefinedDescription updates the refined description (thread-safe)
func (s *WizardSession) SetRefinedDescription(desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RefinedDescription = desc
	s.UpdatedAt = time.Now()
}

// SetTasks updates the task list (thread-safe)
func (s *WizardSession) SetTasks(tasks []WizardTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tasks = tasks
	s.UpdatedAt = time.Now()
}

// GetLogs returns a copy of the logs (thread-safe)
func (s *WizardSession) GetLogs() []LLMLogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := make([]LLMLogEntry, len(s.LLMLogs))
	copy(logs, s.LLMLogs)
	return logs
}

// WizardSessionStore manages all active wizard sessions in memory
type WizardSessionStore struct {
	sessions map[string]*WizardSession
	mu       sync.RWMutex
}

// NewWizardSessionStore creates a new session store
func NewWizardSessionStore() *WizardSessionStore {
	return &WizardSessionStore{
		sessions: make(map[string]*WizardSession),
	}
}

// Create creates a new wizard session and returns it
func (ws *WizardSessionStore) Create(wizardType string) *WizardSession {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	now := time.Now()
	session := &WizardSession{
		ID:          uuid.New().String(),
		Type:        WizardType(wizardType),
		CurrentStep: WizardStepNew,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ws.sessions[session.ID] = session
	return session
}

// Get retrieves a session by ID
func (ws *WizardSessionStore) Get(id string) (*WizardSession, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	session, ok := ws.sessions[id]
	return session, ok
}

// Delete removes a session by ID
func (ws *WizardSessionStore) Delete(id string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	delete(ws.sessions, id)
}

// CleanupOldSessions removes sessions older than the specified duration
func (ws *WizardSessionStore) CleanupOldSessions(maxAge time.Duration) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, session := range ws.sessions {
		if session.UpdatedAt.Before(cutoff) {
			delete(ws.sessions, id)
		}
	}
}
