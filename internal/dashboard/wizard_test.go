package dashboard

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWizardSessionStore_CreateAndRetrieve(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	session, err := store.Create("feature")
	if err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}
	if session.ID == "" {
		t.Error("expected session ID to be generated")
	}
	if session.Type != "feature" {
		t.Errorf("expected type 'feature', got %q", session.Type)
	}
	if session.CurrentStep != "new" {
		t.Errorf("expected step 'new', got %q", session.CurrentStep)
	}

	// Test retrieval
	retrieved, ok := store.Get(session.ID)
	if !ok {
		t.Error("expected to retrieve session")
	}
	if retrieved.ID != session.ID {
		t.Error("retrieved session ID mismatch")
	}
}

func TestWizardSessionStore_Create_InvalidType(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	_, err := store.Create("invalid")
	if err == nil {
		t.Error("expected error for invalid wizard type")
	}
	if !strings.Contains(err.Error(), "invalid wizard type") {
		t.Errorf("expected error message to contain 'invalid wizard type', got: %v", err)
	}
}

func TestWizardSessionStore_Update(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	session, _ := store.Create("bug")

	session.CurrentStep = "refine"
	session.IdeaText = "Fix login bug"
	session.RefinedDescription = "The login form doesn't validate email format"

	updated, ok := store.Get(session.ID)
	if !ok {
		t.Fatal("expected to retrieve updated session")
	}
	if updated.CurrentStep != "refine" {
		t.Errorf("expected step 'refine', got %q", updated.CurrentStep)
	}
	if updated.IdeaText != "Fix login bug" {
		t.Errorf("expected idea 'Fix login bug', got %q", updated.IdeaText)
	}
}

func TestWizardSessionStore_Delete(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	session, _ := store.Create("feature")

	store.Delete(session.ID)

	_, ok := store.Get(session.ID)
	if ok {
		t.Error("expected session to be deleted")
	}
}

func TestWizardSessionStore_CleanupOldSessions(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	// Create a session
	session, _ := store.Create("feature")

	// Manually set UpdatedAt to be old
	session.UpdatedAt = time.Now().Add(-31 * time.Minute)

	// Run cleanup with 30 minute max age
	store.CleanupOldSessions(30 * time.Minute)

	// Session should be deleted
	_, ok := store.Get(session.ID)
	if ok {
		t.Error("expected old session to be cleaned up")
	}
}

func TestWizardSessionStore_BackgroundCleanup(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	// Create a session and make it old
	session, _ := store.Create("feature")
	session.UpdatedAt = time.Now().Add(-31 * time.Minute)

	// Wait for background cleanup (cleanup runs every 5 minutes in production,
	// but we'll manually trigger it for testing)
	store.CleanupOldSessions(30 * time.Minute)

	// Verify session was cleaned up
	if store.Count() != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", store.Count())
	}
}

func TestWizardSession_AddLog(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	session.AddLog("system", "Starting refinement")
	session.AddLog("user", "Create a user profile page")

	if len(session.LLMLogs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(session.LLMLogs))
	}
	if session.LLMLogs[0].Role != "system" {
		t.Errorf("expected first log role 'system', got %q", session.LLMLogs[0].Role)
	}
}

func TestWizardSession_GetLogs(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	session.AddLog("system", "Starting")
	session.AddLog("user", "Test")

	// Get logs should return a copy
	logs1 := session.GetLogs()
	logs2 := session.GetLogs()

	// Modify logs1
	if len(logs1) > 0 {
		logs1[0].Message = "Modified"
	}

	// logs2 should be unchanged
	if logs2[0].Message != "Starting" {
		t.Error("GetLogs should return a copy, not a reference")
	}
}

func TestWizardSession_SetStep(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	session.SetStep(WizardStepRefine)
	if session.CurrentStep != WizardStepRefine {
		t.Errorf("expected step 'refine', got %q", session.CurrentStep)
	}
}

func TestWizardSession_SetRefinedDescription(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	session.SetRefinedDescription("Refined description")
	if session.RefinedDescription != "Refined description" {
		t.Errorf("expected refined description, got %q", session.RefinedDescription)
	}
}

func TestWizardSession_SetTasks(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	tasks := []WizardTask{
		{Title: "Task 1", Description: "Desc 1", Priority: "high", Complexity: "M"},
	}

	session.SetTasks(tasks)
	if len(session.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(session.Tasks))
	}
}

func TestParseTaskJSON_RawJSON(t *testing.T) {
	input := `[{"title": "Task 1", "description": "Description 1", "priority": "high", "complexity": "M"}]`

	tasks := parseTaskJSON(input)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Task 1" {
		t.Errorf("expected title 'Task 1', got %q", tasks[0].Title)
	}
}

func TestParseTaskJSON_MarkdownCodeBlock(t *testing.T) {
	input := "```json\n[{\"title\": \"Task 1\", \"description\": \"Description 1\", \"priority\": \"high\", \"complexity\": \"M\"}]\n```"

	tasks := parseTaskJSON(input)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Task 1" {
		t.Errorf("expected title 'Task 1', got %q", tasks[0].Title)
	}
}

func TestParseTaskJSON_MarkdownNoLanguage(t *testing.T) {
	input := "```\n[{\"title\": \"Task 1\", \"description\": \"Description 1\", \"priority\": \"high\", \"complexity\": \"M\"}]\n```"

	tasks := parseTaskJSON(input)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestParseTaskJSON_InvalidJSON(t *testing.T) {
	input := "not valid json"

	tasks := parseTaskJSON(input)
	if tasks != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestParseTaskJSON_EmptyString(t *testing.T) {
	tasks := parseTaskJSON("")
	if tasks != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseTaskJSON_NestedArrays(t *testing.T) {
	// Test that it correctly finds the outer array
	input := `[{"title": "Task 1", "description": "Has [brackets] inside", "priority": "high", "complexity": "M"}]`

	tasks := parseTaskJSON(input)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Task 1" {
		t.Errorf("expected title 'Task 1', got %q", tasks[0].Title)
	}
}

func TestParseTaskJSON_MultipleTasks(t *testing.T) {
	input := `[
		{"title": "Task 1", "description": "Desc 1", "priority": "high", "complexity": "M"},
		{"title": "Task 2", "description": "Desc 2", "priority": "medium", "complexity": "S"},
		{"title": "Task 3", "description": "Desc 3", "priority": "low", "complexity": "L"}
	]`

	tasks := parseTaskJSON(input)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "Task 1" || tasks[1].Title != "Task 2" || tasks[2].Title != "Task 3" {
		t.Error("task titles don't match")
	}
}

func TestBuildRefinementPrompt_Feature(t *testing.T) {
	codebaseContext := "Project uses Go with standard layout"
	prompt := BuildRefinementPrompt(WizardTypeFeature, "Create a login page", codebaseContext)
	if !strings.Contains(prompt, "feature description") {
		t.Error("expected prompt to mention 'feature description'")
	}
	if !strings.Contains(prompt, "Create a login page") {
		t.Error("expected prompt to contain the original idea")
	}
	if !strings.Contains(prompt, codebaseContext) {
		t.Error("expected prompt to contain codebase context")
	}
	if !strings.Contains(prompt, "existing codebase patterns") {
		t.Error("expected prompt to instruct analyzing codebase patterns")
	}
	if !strings.Contains(prompt, "ONLY output is a markdown issue body") {
		t.Error("expected prompt to enforce markdown-only output")
	}
	if !strings.Contains(prompt, "Do NOT start with") {
		t.Error("expected prompt to forbid conversational preamble")
	}
}

func TestBuildRefinementPrompt_Bug(t *testing.T) {
	codebaseContext := "Project uses Go with standard layout"
	prompt := BuildRefinementPrompt(WizardTypeBug, "Login is broken", codebaseContext)
	if !strings.Contains(prompt, "bug report") {
		t.Error("expected prompt to mention 'bug report'")
	}
	if !strings.Contains(prompt, "Steps to reproduce") {
		t.Error("expected prompt to ask for steps to reproduce")
	}
	if !strings.Contains(prompt, codebaseContext) {
		t.Error("expected prompt to contain codebase context")
	}
	if !strings.Contains(prompt, "ONLY output is a markdown issue body") {
		t.Error("expected prompt to enforce markdown-only output")
	}
	if !strings.Contains(prompt, "Do NOT start with") {
		t.Error("expected prompt to forbid conversational preamble")
	}
}

func TestBuildRefinementPrompt_EmptyCodebaseContext(t *testing.T) {
	prompt := BuildRefinementPrompt(WizardTypeFeature, "Create a login page", "")
	if !strings.Contains(prompt, "No codebase context provided") {
		t.Error("expected prompt to handle empty codebase context gracefully")
	}
}

func TestBuildBreakdownPrompt(t *testing.T) {
	prompt := BuildBreakdownPrompt(WizardTypeFeature, "A feature description")
	if !strings.Contains(prompt, "JSON array") {
		t.Error("expected prompt to mention JSON array")
	}
	if !strings.Contains(prompt, "title") {
		t.Error("expected prompt to mention title field")
	}
	if !strings.Contains(prompt, "A feature description") {
		t.Error("expected prompt to contain the description")
	}
	if !strings.Contains(prompt, "DO NOT include implementation details") {
		t.Error("expected prompt to explicitly forbid implementation details")
	}
	if !strings.Contains(prompt, "Acceptance Criteria:") {
		t.Error("expected prompt to require acceptance criteria")
	}
	if !strings.Contains(prompt, "WHAT needs to be done") {
		t.Error("expected prompt to focus on WHAT not HOW")
	}
}

func TestBuildBreakdownPrompt_BugType(t *testing.T) {
	prompt := BuildBreakdownPrompt(WizardTypeBug, "A bug description")
	if !strings.Contains(prompt, "Bug fix description") {
		t.Error("expected prompt to use 'Bug fix' label for bug type")
	}
	if !strings.Contains(prompt, "DO NOT include implementation details") {
		t.Error("expected prompt to explicitly forbid implementation details")
	}
}

func TestBuildBreakdownPrompt_ForbidsImplementationDetails(t *testing.T) {
	prompt := BuildBreakdownPrompt(WizardTypeFeature, "Some feature")

	// Check for explicit prohibition
	if !strings.Contains(prompt, "DO NOT include implementation details") {
		t.Error("expected prompt to explicitly forbid implementation details")
	}
	if !strings.Contains(prompt, "code snippets") {
		t.Error("expected prompt to forbid code snippets")
	}
	if !strings.Contains(prompt, "specific technical solutions") {
		t.Error("expected prompt to forbid specific technical solutions")
	}

	// Check for focus on WHAT not HOW
	if !strings.Contains(prompt, "WHAT needs to be done") {
		t.Error("expected prompt to focus on WHAT")
	}
	if !strings.Contains(prompt, "NOT HOW to do it") {
		t.Error("expected prompt to explicitly say NOT HOW")
	}
}

func TestBuildBreakdownPrompt_RequiresAcceptanceCriteria(t *testing.T) {
	prompt := BuildBreakdownPrompt(WizardTypeFeature, "Some feature")

	if !strings.Contains(prompt, "Acceptance Criteria:") {
		t.Error("expected prompt to require acceptance criteria section")
	}
	if !strings.Contains(prompt, "MUST include clear acceptance criteria") {
		t.Error("expected prompt to require acceptance criteria in description")
	}
	if !strings.Contains(prompt, "what \"done\" looks like") {
		t.Error("expected prompt to define what done looks like")
	}
}

func TestBuildBreakdownPrompt_ValidJSONStructure(t *testing.T) {
	prompt := BuildBreakdownPrompt(WizardTypeFeature, "Some feature")

	// Check for JSON format requirements
	if !strings.Contains(prompt, "Return ONLY a JSON array") {
		t.Error("expected prompt to require JSON array output")
	}
	if !strings.Contains(prompt, `"title"`) {
		t.Error("expected prompt to specify title field in JSON")
	}
	if !strings.Contains(prompt, `"description"`) {
		t.Error("expected prompt to specify description field in JSON")
	}
	if !strings.Contains(prompt, `"priority"`) {
		t.Error("expected prompt to specify priority field in JSON")
	}
	if !strings.Contains(prompt, `"complexity"`) {
		t.Error("expected prompt to specify complexity field in JSON")
	}
	if !strings.Contains(prompt, "No markdown, no explanation, just the JSON array") {
		t.Error("expected prompt to forbid markdown and explanations")
	}
}

func TestGetCodebaseContext(t *testing.T) {
	context := GetCodebaseContext()
	if context == "" {
		t.Error("expected non-empty codebase context")
	}
	if !strings.Contains(context, "Project Structure") {
		t.Error("expected context to mention project structure")
	}
}

func TestWizardSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewWizardSessionStore()
	defer store.Stop()

	// Create multiple sessions concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Create("feature")
		}()
	}
	wg.Wait()

	if store.Count() != 100 {
		t.Errorf("expected 100 sessions, got %d", store.Count())
	}

	// Access sessions concurrently
	var ids []string
	for id := range store.sessions {
		ids = append(ids, id)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx < len(ids) {
				store.Get(ids[idx])
			}
		}(i)
	}
	wg.Wait()
}

func TestWizardSession_ConcurrentLogAccess(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session.AddLog("system", "Log message")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.GetLogs()
		}()
	}

	wg.Wait()

	if len(session.LLMLogs) != 100 {
		t.Errorf("expected 100 logs, got %d", len(session.LLMLogs))
	}
}

func TestValidWizardTypes(t *testing.T) {
	if !ValidWizardTypes[WizardTypeFeature] {
		t.Error("WizardTypeFeature should be valid")
	}
	if !ValidWizardTypes[WizardTypeBug] {
		t.Error("WizardTypeBug should be valid")
	}
	if ValidWizardTypes["invalid"] {
		t.Error("'invalid' should not be a valid wizard type")
	}
}

func TestDefaultLLMModel(t *testing.T) {
	if DefaultLLMModel != "nexos-ai/Kimi K2.5" {
		t.Errorf("expected DefaultLLMModel to be 'nexos-ai/Kimi K2.5', got %q", DefaultLLMModel)
	}
}

func TestSessionConstants(t *testing.T) {
	if SessionCleanupInterval != 5*time.Minute {
		t.Errorf("expected SessionCleanupInterval to be 5 minutes, got %v", SessionCleanupInterval)
	}
	if SessionMaxAge != 30*time.Minute {
		t.Errorf("expected SessionMaxAge to be 30 minutes, got %v", SessionMaxAge)
	}
}

func TestWizardSession_SetAddToSprint(t *testing.T) {
	session := &WizardSession{
		ID:   "test-id",
		Type: "feature",
	}

	session.SetAddToSprint(true)
	if !session.AddToSprint {
		t.Errorf("expected AddToSprint to be true, got %v", session.AddToSprint)
	}

	session.SetAddToSprint(false)
	if session.AddToSprint {
		t.Errorf("expected AddToSprint to be false, got %v", session.AddToSprint)
	}
}

func TestWizardStepConstants(t *testing.T) {
	// Verify breakdown step is removed
	steps := []WizardStep{
		WizardStepNew,
		WizardStepRefine,
		// WizardStepBreakdown should NOT exist
		WizardStepCreate,
		WizardStepDone,
	}

	// Should have exactly 4 steps (not 5)
	if len(steps) != 4 {
		t.Errorf("Expected 4 steps, got %d", len(steps))
	}
}

func TestWizardSession_TechnicalPlanning(t *testing.T) {
	// Verify TechnicalPlanning field works
	session := &WizardSession{
		ID:   "test-id",
		Type: WizardTypeFeature,
	}

	planning := "## Architecture Overview\n\nTest planning content"
	session.SetTechnicalPlanning(planning)

	if session.TechnicalPlanning != planning {
		t.Errorf("expected TechnicalPlanning to be set correctly")
	}
}
