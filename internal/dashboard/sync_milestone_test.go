package dashboard

import (
	"errors"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

// mockGitHubClientWithMilestone extends mockGitHubClient to support milestone switching
type mockGitHubClientWithMilestone struct {
	mockGitHubClient
	milestones     []*github.Milestone
	milestoneIndex int
	milestoneErr   error
}

func (m *mockGitHubClientWithMilestone) GetOldestOpenMilestone() (*github.Milestone, error) {
	if m.milestoneErr != nil {
		return nil, m.milestoneErr
	}
	if m.milestoneIndex >= len(m.milestones) {
		return nil, nil
	}
	milestone := m.milestones[m.milestoneIndex]
	return milestone, nil
}

func (m *mockGitHubClientWithMilestone) setNextMilestone(index int) {
	m.milestoneIndex = index
}

func TestSyncService_AutoDetectsNewMilestone(t *testing.T) {
	milestoneA := &github.Milestone{Number: 1, Title: "Sprint 1"}
	milestoneB := &github.Milestone{Number: 2, Title: "Sprint 2"}

	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{milestoneA, milestoneB},
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// First sync - should detect and set Sprint 1
	gh.setNextMilestone(0)
	service.syncNow()

	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1' after first sync, got %q", got)
	}

	// Second sync - should auto-detect and switch to Sprint 2
	gh.setNextMilestone(1)
	service.syncNow()

	if got := service.GetActiveMilestone(); got != "Sprint 2" {
		t.Errorf("Expected milestone 'Sprint 2' after second sync, got %q", got)
	}
}

func TestSyncService_HandlesNoMilestone(t *testing.T) {
	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	// Sync with no milestones available
	gh.setNextMilestone(0)
	service.syncNow()

	// Should clear the active milestone
	if got := service.GetActiveMilestone(); got != "" {
		t.Errorf("Expected empty milestone when none available, got %q", got)
	}

	// Should not cache any issues
	if len(store.cachedIssues) != 0 {
		t.Errorf("Expected 0 cached issues when no milestone, got %d", len(store.cachedIssues))
	}
}

func TestSyncService_HandlesMilestoneClosedExternally(t *testing.T) {
	milestoneA := &github.Milestone{Number: 1, Title: "Sprint 1"}
	milestoneB := &github.Milestone{Number: 2, Title: "Sprint 2"}

	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{milestoneA, milestoneB},
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// Start with Sprint 1
	service.SetActiveMilestone("Sprint 1")

	// Simulate Sprint 1 being closed externally - now Sprint 2 is the oldest open
	gh.setNextMilestone(1)
	service.syncNow()

	// Should automatically switch to Sprint 2
	if got := service.GetActiveMilestone(); got != "Sprint 2" {
		t.Errorf("Expected milestone 'Sprint 2' after external close, got %q", got)
	}
}

func TestSyncService_SameMilestoneNoChange(t *testing.T) {
	milestoneA := &github.Milestone{Number: 1, Title: "Sprint 1"}

	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{milestoneA, milestoneA},
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// Set Sprint 1 as active
	service.SetActiveMilestone("Sprint 1")

	// Sync - milestone hasn't changed
	gh.setNextMilestone(0)
	service.syncNow()

	// Should still be Sprint 1
	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1' to remain unchanged, got %q", got)
	}

	// Should have cached issues
	if len(store.cachedIssues) != 1 {
		t.Errorf("Expected 1 cached issue, got %d", len(store.cachedIssues))
	}
}

func TestSyncService_SameTitleDifferentNumber(t *testing.T) {
	// Simulate milestone recreation with same title but different number
	milestoneA := &github.Milestone{Number: 1, Title: "Sprint 1"}
	milestoneB := &github.Milestone{Number: 2, Title: "Sprint 1"}

	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{milestoneA, milestoneB},
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// Start with original Sprint 1 (number 1)
	service.SetActiveMilestone("Sprint 1")

	// Simulate milestone recreation - same title, different number
	gh.setNextMilestone(1)
	service.syncNow()

	// Title is the same, so no change detected (this is expected behavior)
	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1' (same title), got %q", got)
	}
}

func TestSyncService_MilestoneErrorHandling(t *testing.T) {
	gh := &mockGitHubClientWithMilestone{
		milestoneErr: errors.New("github api error"),
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)
	service.SetActiveMilestone("Sprint 1")

	// Sync with error in milestone detection
	service.syncNow()

	// Should continue with existing milestone
	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1' to remain after error, got %q", got)
	}

	// Should still sync issues from existing milestone
	if len(store.cachedIssues) != 1 {
		t.Errorf("Expected 1 cached issue despite milestone error, got %d", len(store.cachedIssues))
	}
}

func TestSyncService_NoMilestoneThenNewMilestoneAppears(t *testing.T) {
	milestoneA := &github.Milestone{Number: 1, Title: "Sprint 1"}

	gh := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{nil, milestoneA},
	}
	gh.issues = []github.Issue{
		{Number: 1, Title: "Issue 1", State: "open"},
	}

	store := &mockStore{}
	service := NewSyncService(gh, store, nil, nil)

	// First sync - no milestones available
	gh.setNextMilestone(0)
	service.syncNow()

	if got := service.GetActiveMilestone(); got != "" {
		t.Errorf("Expected empty milestone initially, got %q", got)
	}

	// Clear store for second sync
	store.clearCachedIssues()

	// Second sync - new milestone appears
	gh.setNextMilestone(1)
	service.syncNow()

	// Should detect and set the new milestone
	if got := service.GetActiveMilestone(); got != "Sprint 1" {
		t.Errorf("Expected milestone 'Sprint 1' after new milestone appears, got %q", got)
	}

	// Should sync issues from the new milestone
	if len(store.cachedIssues) != 1 {
		t.Errorf("Expected 1 cached issue from new milestone, got %d", len(store.cachedIssues))
	}
}
