package sprint_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/sprint"
)

// mockGitHubClient is a mock implementation of the sprint.GitHubClient interface
type mockGitHubClient struct {
	milestones []github.Milestone
	issues     []github.Issue
}

func (m *mockGitHubClient) GetMostRecentlyClosedSprint() (*github.Milestone, error) {
	for i := range m.milestones {
		if m.milestones[i].State == "closed" {
			return &m.milestones[i], nil
		}
	}
	return nil, nil
}

func (m *mockGitHubClient) GetClosedTicketsForSprint(_ int) ([]github.Issue, error) {
	var closed []github.Issue
	for _, issue := range m.issues {
		if issue.State == "closed" && issue.PRMerged {
			closed = append(closed, issue)
		}
	}
	return closed, nil
}

func (m *mockGitHubClient) ListMilestones() ([]github.Milestone, error) {
	return m.milestones, nil
}

func (m *mockGitHubClient) ListIssuesWithPRStatus(_ string) ([]github.Issue, error) {
	return m.issues, nil
}

// setupTestRepo creates a test git repository
func setupTestRepo(t *testing.T) string {
	t.Helper()

	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	}

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create a bare repo to act as "origin"
	bareDir := t.TempDir()
	runGit(bareDir, "init", "--bare")

	// Clone it to get a working repo with origin configured
	cloneDir := t.TempDir()
	runGit(cloneDir, "clone", bareDir, ".")

	// Configure git identity
	runGit(cloneDir, "config", "user.name", "test")
	runGit(cloneDir, "config", "user.email", "test@test.com")

	// Create an initial commit and push to origin
	runGit(cloneDir, "commit", "--allow-empty", "-m", "init")
	runGit(cloneDir, "push", "origin", "master")

	return cloneDir
}

func TestNewCleanupManager(t *testing.T) {
	repoDir := setupTestRepo(t)
	ghClient := &mockGitHubClient{}
	branchMgr := git.NewBranchManager(repoDir)

	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	if mgr == nil {
		t.Fatal("NewCleanupManager returned nil")
	}
}

func TestDetectArtifacts_NoClosedSprints(t *testing.T) {
	repoDir := setupTestRepo(t)
	ghClient := &mockGitHubClient{
		milestones: []github.Milestone{
			{Title: "Sprint 1", Number: 1, State: "open"},
		},
	}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	_, err := mgr.DetectArtifacts("")
	if err == nil {
		t.Error("Expected error when no closed sprints exist")
	}
}

func TestDetectArtifacts_WithClosedSprint(t *testing.T) {
	repoDir := setupTestRepo(t)
	ghClient := &mockGitHubClient{
		milestones: []github.Milestone{
			{Title: "Sprint 1", Number: 1, State: "closed", DueOn: time.Now().Add(-24 * time.Hour)},
		},
		issues: []github.Issue{
			{Number: 123, Title: "Test Issue", State: "closed", PRMerged: true},
		},
	}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	plan, err := mgr.DetectArtifacts("")
	if err != nil {
		t.Fatalf("DetectArtifacts failed: %v", err)
	}

	if plan.Sprint != "Sprint 1" {
		t.Errorf("Expected sprint 'Sprint 1', got %q", plan.Sprint)
	}
}

func TestDetectArtifacts_WithBranches(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Create a branch for issue 123
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create a branch with the oda-123- prefix
	runGit("checkout", "-b", "oda-123-fix-bug")
	runGit("commit", "--allow-empty", "-m", "fix bug")
	runGit("checkout", "master")

	ghClient := &mockGitHubClient{
		milestones: []github.Milestone{
			{Title: "Sprint 1", Number: 1, State: "closed", DueOn: time.Now().Add(-24 * time.Hour)},
		},
		issues: []github.Issue{
			{Number: 123, Title: "Fix bug", State: "closed", PRMerged: true},
		},
	}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	plan, err := mgr.DetectArtifacts("")
	if err != nil {
		t.Fatalf("DetectArtifacts failed: %v", err)
	}

	if len(plan.Branches) != 1 {
		t.Errorf("Expected 1 branch, got %d", len(plan.Branches))
	}

	if len(plan.Branches) > 0 && plan.Branches[0].Name != "oda-123-fix-bug" {
		t.Errorf("Expected branch 'oda-123-fix-bug', got %q", plan.Branches[0].Name)
	}
}

func TestExecute_DryRun(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Create a branch
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("checkout", "-b", "oda-456-test")
	runGit("checkout", "master")

	ghClient := &mockGitHubClient{}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	plan := &sprint.CleanupPlan{
		Sprint: "Test Sprint",
		Branches: []sprint.BranchArtifact{
			{Name: "oda-456-test", IssueNumber: 456, CanDelete: true},
		},
	}

	result, err := mgr.Execute(plan, true) // dry-run = true
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.DeletedBranches != 1 {
		t.Errorf("Expected 1 branch in dry-run count, got %d", result.DeletedBranches)
	}

	// Verify branch still exists (dry-run shouldn't delete)
	cmd := exec.Command("git", "branch", "--list", "oda-456-test")
	cmd.Dir = repoDir
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "oda-456-test") {
		t.Error("Branch was deleted in dry-run mode - should still exist")
	}
}

func TestExecute_ActualDelete(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Create a branch
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("checkout", "-b", "oda-789-test")
	runGit("checkout", "master")

	ghClient := &mockGitHubClient{}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	plan := &sprint.CleanupPlan{
		Sprint: "Test Sprint",
		Branches: []sprint.BranchArtifact{
			{Name: "oda-789-test", IssueNumber: 789, CanDelete: true},
		},
	}

	result, err := mgr.Execute(plan, false) // dry-run = false
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.DeletedBranches != 1 {
		t.Errorf("Expected 1 deleted branch, got %d", result.DeletedBranches)
	}

	// Verify branch was deleted
	cmd := exec.Command("git", "branch", "--list", "oda-789-test")
	cmd.Dir = repoDir
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "oda-789-test") {
		t.Error("Branch still exists after deletion")
	}
}

func TestExecute_SkipsUnmergedBranches(t *testing.T) {
	repoDir := setupTestRepo(t)

	ghClient := &mockGitHubClient{}
	branchMgr := git.NewBranchManager(repoDir)
	mgr := sprint.NewCleanupManager(repoDir, ghClient, branchMgr)

	plan := &sprint.CleanupPlan{
		Sprint: "Test Sprint",
		Branches: []sprint.BranchArtifact{
			{Name: "oda-999-test", IssueNumber: 999, CanDelete: false, HasUnmerged: true},
		},
	}

	result, err := mgr.Execute(plan, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.SkippedBranches != 1 {
		t.Errorf("Expected 1 skipped branch, got %d", result.SkippedBranches)
	}

	if result.DeletedBranches != 0 {
		t.Errorf("Expected 0 deleted branches, got %d", result.DeletedBranches)
	}
}

func TestDetectOldTempFiles(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Create .oda/tmp directory with old and new files
	tmpDir := filepath.Join(repoDir, ".oda", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create tmp dir: %v", err)
	}

	// Create an old file
	oldFile := filepath.Join(tmpDir, "old-file.txt")
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}

	// Set modification time to 30 days ago
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Failed to set file time: %v", err)
	}

	// Create a new file
	newFile := filepath.Join(tmpDir, "new-file.txt")
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Manually test temp file detection
	cutoffTime := time.Now().Add(-7 * 24 * time.Hour)
	var oldFiles []string
	err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // intentional - skip inaccessible files
		}
		if info.ModTime().Before(cutoffTime) {
			oldFiles = append(oldFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	if len(oldFiles) != 1 {
		t.Errorf("Expected 1 old file, got %d", len(oldFiles))
	}

	if len(oldFiles) > 0 && !strings.Contains(oldFiles[0], "old-file.txt") {
		t.Errorf("Expected old-file.txt, got %s", oldFiles[0])
	}
}

func TestPrintPlan(t *testing.T) {
	plan := &sprint.CleanupPlan{
		Sprint: "Test Sprint",
		Branches: []sprint.BranchArtifact{
			{Name: "oda-123-fix", IssueNumber: 123, CanDelete: true},
			{Name: "oda-456-feat", IssueNumber: 456, CanDelete: false, HasOpenPR: true},
		},
		Worktrees: []sprint.WorktreeArtifact{
			{Path: "/tmp/wt1", Branch: "oda-123-fix"},
		},
		TempFiles: []sprint.TempFileArtifact{
			{Path: "/tmp/old.txt", ModifiedAt: time.Now().Add(-30 * 24 * time.Hour)},
		},
	}

	var buf bytes.Buffer
	sprint.PrintPlan(plan, &buf)

	output := buf.String()
	if !strings.Contains(output, "Test Sprint") {
		t.Error("PrintPlan output should contain sprint name")
	}
	if !strings.Contains(output, "oda-123-fix") {
		t.Error("PrintPlan output should contain branch name")
	}
}

func TestPrintResult(t *testing.T) {
	result := &sprint.CleanupResult{
		DeletedBranches:  2,
		DeletedWorktrees: 1,
		DeletedTempFiles: 3,
		SkippedBranches:  1,
		Errors:           []error{},
	}

	var buf bytes.Buffer
	sprint.PrintResult(result, &buf)

	output := buf.String()
	if !strings.Contains(output, "2") {
		t.Error("PrintResult output should contain deleted branch count")
	}
	if !strings.Contains(output, "1") {
		t.Error("PrintResult output should contain worktree count")
	}
}
