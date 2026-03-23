package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultLLMModel is the fallback model used for wizard LLM calls
// when no model is configured via config.yaml
const DefaultLLMModel = "nexos-ai/Kimi K2.5"

// SessionCleanupInterval is how often to check for old sessions
const SessionCleanupInterval = 5 * time.Minute

// SessionMaxAge is how long sessions can live before being cleaned up
const SessionMaxAge = 30 * time.Minute

// MaxSessions is the maximum number of concurrent wizard sessions allowed
const MaxSessions = 1000

// ValidWizardTypes contains the allowed wizard types
var ValidWizardTypes = map[WizardType]bool{
	WizardTypeFeature: true,
	WizardTypeBug:     true,
}

// WizardType represents the type of wizard being run
type WizardType string

const (
	WizardTypeFeature WizardType = "feature"
	WizardTypeBug     WizardType = "bug"
)

// WizardStep represents the current step in the wizard flow
type WizardStep string

const (
	WizardStepNew    WizardStep = "new"
	WizardStepRefine WizardStep = "refine"
	// REMOVED: WizardStepBreakdown WizardStep = "breakdown"
	// REMOVED: WizardStepTitle  WizardStep = "title" (merged into refine step)
	WizardStepCreate WizardStep = "create"
	WizardStepDone   WizardStep = "done"
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

// CreatedIssue tracks a GitHub issue created by the wizard
type CreatedIssue struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	IsEpic  bool   `json:"is_epic"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// WizardSession holds the state for a single wizard instance
type WizardSession struct {
	ID                 string     `json:"id"`
	Type               WizardType `json:"type"`
	CurrentStep        WizardStep `json:"current_step"`
	IdeaText           string     `json:"idea_text"`
	RefinedDescription string     `json:"refined_description"`
	// REMOVED: Tasks              []WizardTask   `json:"tasks"`
	TechnicalPlanning string         `json:"technical_planning"` // NEW FIELD
	GeneratedTitle    string         `json:"generated_title"`    // NEW: LLM-generated title
	CustomTitle       string         `json:"custom_title"`       // NEW: User-edited title
	UseCustomTitle    bool           `json:"use_custom_title"`   // NEW: Whether to use custom title
	Priority          string         `json:"priority"`           // LLM-estimated priority: high, medium, low
	Complexity        string         `json:"complexity"`         // LLM-estimated complexity: S, M, L, XL
	CreatedIssues     []CreatedIssue `json:"created_issues"`
	EpicNumber        int            `json:"epic_number"`
	AddToSprint       bool           `json:"add_to_sprint"`
	SkipBreakdown     bool           `json:"skip_breakdown"` // Keep for backward compatibility
	Language          string         `json:"language"`       // NEW FIELD
	LLMLogs           []LLMLogEntry  `json:"llm_logs"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	mu                sync.RWMutex   `json:"-"`
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

// SetTechnicalPlanning updates the technical planning (thread-safe)
func (s *WizardSession) SetTechnicalPlanning(planning string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TechnicalPlanning = planning
	s.UpdatedAt = time.Now()
}

// SetGeneratedTitle updates the generated title (thread-safe)
func (s *WizardSession) SetGeneratedTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.GeneratedTitle = title
	s.UpdatedAt = time.Now()
}

// SetCustomTitle updates the custom title (thread-safe)
func (s *WizardSession) SetCustomTitle(title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CustomTitle = title
	s.UpdatedAt = time.Now()
}

// SetUseCustomTitle updates the flag for using custom title (thread-safe)
func (s *WizardSession) SetUseCustomTitle(use bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UseCustomTitle = use
	s.UpdatedAt = time.Now()
}

// GetFinalTitle returns the title to use for issue creation (thread-safe)
// Returns custom title if UseCustomTitle is true, otherwise returns generated title
func (s *WizardSession) GetFinalTitle() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.UseCustomTitle && s.CustomTitle != "" {
		return s.CustomTitle
	}
	return s.GeneratedTitle
}

// SetLanguage updates the language preference (thread-safe)
func (s *WizardSession) SetLanguage(lang string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Language = lang
	s.UpdatedAt = time.Now()
}

// SetPriority updates the LLM-estimated priority (thread-safe)
func (s *WizardSession) SetPriority(priority string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Priority = priority
	s.UpdatedAt = time.Now()
}

// SetComplexity updates the LLM-estimated complexity (thread-safe)
func (s *WizardSession) SetComplexity(complexity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Complexity = complexity
	s.UpdatedAt = time.Now()
}

// MigrateOldSession converts an old session format to the new format
// This handles sessions that were in the breakdown step
func (s *WizardSession) MigrateOldSession() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If session was in breakdown step, move it to refine
	if s.CurrentStep == "breakdown" {
		s.CurrentStep = WizardStepRefine
	}

	// If session has RefinedDescription but no TechnicalPlanning, convert it
	if s.RefinedDescription != "" && s.TechnicalPlanning == "" {
		s.TechnicalPlanning = s.RefinedDescription
	}
}

// SetIdeaText updates the idea text (thread-safe)
func (s *WizardSession) SetIdeaText(idea string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IdeaText = idea
	s.UpdatedAt = time.Now()
}

// SetAddToSprint updates the add-to-sprint flag (thread-safe)
func (s *WizardSession) SetAddToSprint(add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AddToSprint = add
	s.UpdatedAt = time.Now()
}

// SetSkipBreakdown updates the skip-breakdown flag (thread-safe)
func (s *WizardSession) SetSkipBreakdown(skip bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SkipBreakdown = skip
	s.UpdatedAt = time.Now()
}

// SetCreatedIssues updates the created issues list (thread-safe)
func (s *WizardSession) SetCreatedIssues(issues []CreatedIssue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CreatedIssues = issues
	s.UpdatedAt = time.Now()
}

// AddCreatedIssue appends a single created issue (thread-safe)
func (s *WizardSession) AddCreatedIssue(issue CreatedIssue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CreatedIssues = append(s.CreatedIssues, issue)
	s.UpdatedAt = time.Now()
}

// SetEpicNumber sets the epic issue number (thread-safe)
func (s *WizardSession) SetEpicNumber(num int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EpicNumber = num
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
	cancel   context.CancelFunc
}

// NewWizardSessionStore creates a new session store with background cleanup
func NewWizardSessionStore() *WizardSessionStore {
	ctx, cancel := context.WithCancel(context.Background())

	store := &WizardSessionStore{
		sessions: make(map[string]*WizardSession),
		cancel:   cancel,
	}

	// Start background cleanup goroutine
	go store.cleanupLoop(ctx)

	return store
}

// Stop stops the background cleanup goroutine
func (ws *WizardSessionStore) Stop() {
	if ws.cancel != nil {
		ws.cancel()
	}
}

// cleanupLoop runs periodically to clean up old sessions
func (ws *WizardSessionStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ws.CleanupOldSessions(SessionMaxAge)
		}
	}
}

// Create creates a new wizard session and returns it
func (ws *WizardSessionStore) Create(wizardType string) (*WizardSession, error) {
	// Validate wizard type
	if !ValidWizardTypes[WizardType(wizardType)] {
		return nil, fmt.Errorf("invalid wizard type: %q (must be 'feature' or 'bug')", wizardType)
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Check session limit to prevent memory exhaustion
	if len(ws.sessions) >= MaxSessions {
		return nil, fmt.Errorf("maximum number of sessions (%d) reached, please try again later", MaxSessions)
	}

	now := time.Now()
	session := &WizardSession{
		ID:          uuid.New().String(),
		Type:        WizardType(wizardType),
		CurrentStep: WizardStepNew,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ws.sessions[session.ID] = session
	return session, nil
}

// Get retrieves a session by ID and migrates it if needed
func (ws *WizardSessionStore) Get(id string) (*WizardSession, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	session, ok := ws.sessions[id]
	if ok {
		session.MigrateOldSession()
	}
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

// Count returns the number of active sessions
func (ws *WizardSessionStore) Count() int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return len(ws.sessions)
}

// jsonCodeBlockRegex matches markdown code blocks with json
var jsonCodeBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")

// parseTaskJSON extracts and parses the JSON task array from LLM response
// Handles both raw JSON and JSON wrapped in markdown code blocks
func parseTaskJSON(text string) []WizardTask {
	// First, try to find JSON in markdown code blocks
	matches := jsonCodeBlockRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		// Extract content from code block
		text = matches[1]
	}

	// Find the JSON array in the response
	start := -1
	end := -1
	depth := 0

	for i, char := range text {
		if char == '[' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if char == ']' {
			depth--
			if depth == 0 && start != -1 {
				end = i
				break
			}
		}
	}

	if start == -1 || end == -1 || end <= start {
		return nil
	}

	jsonStr := text[start : end+1]

	var tasks []WizardTask
	if err := json.Unmarshal([]byte(jsonStr), &tasks); err != nil {
		return nil
	}

	return tasks
}
