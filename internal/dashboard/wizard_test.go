package dashboard

import (
	"testing"
)

func TestWizardSessionStore_CreateAndRetrieve(t *testing.T) {
	store := NewWizardSessionStore()

	session := store.Create("feature")
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

func TestWizardSessionStore_Update(t *testing.T) {
	store := NewWizardSessionStore()
	session := store.Create("bug")

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
	session := store.Create("feature")

	store.Delete(session.ID)

	_, ok := store.Get(session.ID)
	if ok {
		t.Error("expected session to be deleted")
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
