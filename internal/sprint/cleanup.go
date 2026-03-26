package sprint

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
)

// GitHubClient defines the interface for GitHub operations needed by CleanupManager
type GitHubClient interface {
	GetMostRecentlyClosedSprint() (*github.Milestone, error)
	GetClosedTicketsForSprint(milestoneNumber int) ([]github.Issue, error)
	ListMilestones() ([]github.Milestone, error)
	ListIssuesWithPRStatus(milestone string) ([]github.Issue, error)
}

// CleanupManager orchestrates sprint cleanup operations
type CleanupManager struct {
	repoDir   string
	ghClient  GitHubClient
	branchMgr *git.BranchManager
}

// NewCleanupManager creates a new CleanupManager for the given repo directory
func NewCleanupManager(repoDir string, ghClient GitHubClient, branchMgr *git.BranchManager) *CleanupManager {
	return &CleanupManager{
		repoDir:   repoDir,
		ghClient:  ghClient,
		branchMgr: branchMgr,
	}
}

// BranchArtifact represents a branch that can be cleaned up
type BranchArtifact struct {
	Name        string
	IssueNumber int
	HasUnmerged bool
	HasOpenPR   bool
	CanDelete   bool
}

// WorktreeArtifact represents a worktree that can be cleaned up
type WorktreeArtifact struct {
	Path   string
	Branch string
}

// TempFileArtifact represents a temp file that can be cleaned up
type TempFileArtifact struct {
	Path       string
	ModifiedAt time.Time
}

// CleanupPlan holds all artifacts identified for cleanup
type CleanupPlan struct {
	Sprint    string
	Branches  []BranchArtifact
	Worktrees []WorktreeArtifact
	TempFiles []TempFileArtifact
}

// CleanupResult holds the results of a cleanup operation
type CleanupResult struct {
	DeletedBranches  int
	DeletedWorktrees int
	DeletedTempFiles int
	SkippedBranches  int
	Errors           []error
}

// DetectArtifacts finds all artifacts from a completed sprint
func (m *CleanupManager) DetectArtifacts(sprintTitle string) (*CleanupPlan, error) {
	plan := &CleanupPlan{Sprint: sprintTitle}

	// Get the most recently closed sprint if no title provided
	var milestone *github.Milestone
	var err error

	if sprintTitle == "" {
		milestone, err = m.ghClient.GetMostRecentlyClosedSprint()
		if err != nil {
			return nil, fmt.Errorf("getting most recently closed sprint: %w", err)
		}
		if milestone == nil {
			return nil, errors.New("no closed sprints found")
		}
		plan.Sprint = milestone.Title
	} else {
		// Find milestone by title
		milestones, err := m.ghClient.ListMilestones()
		if err != nil {
			return nil, fmt.Errorf("listing milestones: %w", err)
		}
		for _, m := range milestones {
			if m.Title == sprintTitle {
				milestone = &m
				break
			}
		}
		if milestone == nil {
			return nil, fmt.Errorf("sprint %q not found", sprintTitle)
		}
	}

	// Get closed tickets for this sprint
	tickets, err := m.ghClient.GetClosedTicketsForSprint(milestone.Number)
	if err != nil {
		return nil, fmt.Errorf("getting closed tickets: %w", err)
	}

	log.Printf("[Sprint] Found %d closed/merged tickets in sprint %s", len(tickets), plan.Sprint)

	// Detect branches for each ticket
	for _, ticket := range tickets {
		branch := m.detectBranchForIssue(ticket.Number)
		if branch != "" {
			artifact := m.analyzeBranch(branch, ticket.Number)
			plan.Branches = append(plan.Branches, artifact)
		}
	}

	// Detect worktrees
	worktrees, err := m.detectWorktrees()
	if err != nil {
		log.Printf("[Sprint] Warning: could not detect worktrees: %v", err)
	} else {
		plan.Worktrees = worktrees
	}

	// Detect old temp files
	tempFiles, err := m.detectOldTempFiles(milestone.DueOn)
	if err != nil {
		log.Printf("[Sprint] Warning: could not detect temp files: %v", err)
	} else {
		plan.TempFiles = tempFiles
	}

	return plan, nil
}

// detectBranchForIssue finds a branch associated with an issue number
func (m *CleanupManager) detectBranchForIssue(issueNum int) string {
	prefix := fmt.Sprintf("oda-%d-", issueNum)
	return m.branchMgr.FindBranchByPrefix(prefix)
}

// analyzeBranch checks if a branch can be safely deleted
func (m *CleanupManager) analyzeBranch(branchName string, issueNum int) BranchArtifact {
	artifact := BranchArtifact{
		Name:        branchName,
		IssueNumber: issueNum,
		CanDelete:   true,
	}

	// Check for unmerged changes
	if m.hasUnmergedChanges(branchName) {
		artifact.HasUnmerged = true
		artifact.CanDelete = false
		log.Printf("[Sprint] Branch %s has unmerged changes, skipping", branchName)
	}

	// Check for open PRs
	if m.hasOpenPR(branchName) {
		artifact.HasOpenPR = true
		artifact.CanDelete = false
		log.Printf("[Sprint] Branch %s has open PR, skipping", branchName)
	}

	return artifact
}

// hasUnmergedChanges checks if a branch has commits not merged to default branch
func (m *CleanupManager) hasUnmergedChanges(branch string) bool {
	defaultBranch := m.detectDefaultBranch()

	// Check if there are commits on this branch not in default
	out, err := git.RunInDir(m.repoDir, "git", "log", fmt.Sprintf("%s..%s", defaultBranch, branch), "--oneline")
	if err != nil {
		// If we can't determine, assume it has unmerged changes to be safe
		return true
	}

	return len(strings.TrimSpace(string(out))) > 0
}

// hasOpenPR checks if there's an open PR for this branch
func (m *CleanupManager) hasOpenPR(branch string) bool {
	// Use gh CLI to check for open PRs
	out, err := git.RunInDir(m.repoDir, "gh", "pr", "list", "--head", branch, "--state", "open", "--json", "number")
	if err != nil {
		// If we can't determine, assume no open PR to allow cleanup
		return false
	}

	// If output is not empty "[]", there's an open PR
	return strings.TrimSpace(string(out)) != "[]"
}

// detectDefaultBranch returns the default branch name
func (m *CleanupManager) detectDefaultBranch() string {
	out, err := git.RunInDir(m.repoDir, "git", "rev-parse", "--verify", "main")
	if err == nil && len(out) > 0 {
		return "main"
	}
	return "master"
}

// detectWorktrees finds all worktrees in the repository
func (m *CleanupManager) detectWorktrees() ([]WorktreeArtifact, error) {
	out, err := git.RunInDir(m.repoDir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []WorktreeArtifact
	blocks := strings.SplitSeq(strings.TrimSpace(string(out)), "\n\n")

	for block := range blocks {
		var wtPath, wtBranch string
		lines := strings.SplitSeq(block, "\n")

		for line := range lines {
			if after, ok := strings.CutPrefix(line, "worktree "); ok {
				wtPath = after
			}
			if after, ok := strings.CutPrefix(line, "branch "); ok {
				wtBranch = after
				// Remove refs/heads/ prefix if present
				wtBranch = strings.TrimPrefix(wtBranch, "refs/heads/")
			}
		}

		// Skip the main worktree (repo root)
		if wtPath != "" && wtPath != m.repoDir {
			worktrees = append(worktrees, WorktreeArtifact{
				Path:   wtPath,
				Branch: wtBranch,
			})
		}
	}

	return worktrees, nil
}

// detectOldTempFiles finds temp files older than the given time
func (m *CleanupManager) detectOldTempFiles(olderThan time.Time) ([]TempFileArtifact, error) {
	tmpDir := filepath.Join(m.repoDir, ".oda", "tmp")

	// Check if directory exists
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		return nil, nil
	}

	var oldFiles []TempFileArtifact

	err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files we can't access but continue walking
			return nil //nolint:nilerr // intentional - skip inaccessible files
		}

		if info.IsDir() {
			return nil
		}

		if info.ModTime().Before(olderThan) {
			oldFiles = append(oldFiles, TempFileArtifact{
				Path:       path,
				ModifiedAt: info.ModTime(),
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return oldFiles, nil
}

// Execute performs the cleanup operation
func (m *CleanupManager) Execute(plan *CleanupPlan, dryRun bool) (*CleanupResult, error) {
	result := &CleanupResult{}

	if dryRun {
		log.Printf("[Sprint] DRY RUN - No changes will be made")
	}

	// Clean up branches
	for _, branch := range plan.Branches {
		if !branch.CanDelete {
			result.SkippedBranches++
			continue
		}

		if dryRun {
			log.Printf("[Sprint] Would delete branch: %s", branch.Name)
			result.DeletedBranches++
			continue
		}

		// Delete local branch
		if err := m.branchMgr.RemoveBranch(branch.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("deleting branch %s: %w", branch.Name, err))
			continue
		}

		// Delete remote branch
		if err := m.deleteRemoteBranch(branch.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("deleting remote branch %s: %w", branch.Name, err))
		}

		log.Printf("[Sprint] Deleted branch: %s", branch.Name)
		result.DeletedBranches++
	}

	// Clean up worktrees
	for _, wt := range plan.Worktrees {
		if dryRun {
			log.Printf("[Sprint] Would remove worktree: %s", wt.Path)
			result.DeletedWorktrees++
			continue
		}

		if err := m.removeWorktree(wt.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("removing worktree %s: %w", wt.Path, err))
			continue
		}

		log.Printf("[Sprint] Removed worktree: %s", wt.Path)
		result.DeletedWorktrees++
	}

	// Clean up temp files
	for _, tf := range plan.TempFiles {
		if dryRun {
			log.Printf("[Sprint] Would delete temp file: %s", tf.Path)
			result.DeletedTempFiles++
			continue
		}

		if err := os.Remove(tf.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("deleting temp file %s: %w", tf.Path, err))
			continue
		}

		log.Printf("[Sprint] Deleted temp file: %s", tf.Path)
		result.DeletedTempFiles++
	}

	// Prune git worktree metadata
	if !dryRun && result.DeletedWorktrees > 0 {
		if _, err := git.RunInDir(m.repoDir, "git", "worktree", "prune"); err != nil {
			log.Printf("[Sprint] Warning: git worktree prune failed: %v", err)
		}
	}

	return result, nil
}

// deleteRemoteBranch deletes a branch from origin
func (m *CleanupManager) deleteRemoteBranch(branch string) error {
	_, err := git.RunInDir(m.repoDir, "git", "push", "origin", "--delete", branch)
	return err
}

// removeWorktree removes a git worktree
func (m *CleanupManager) removeWorktree(path string) error {
	_, err := git.RunInDir(m.repoDir, "git", "worktree", "remove", "--force", path)
	return err
}

// PrintPlan displays a summary of the cleanup plan
func PrintPlan(plan *CleanupPlan, w io.Writer) {
	fmt.Fprintf(w, "\nCleanup Plan for Sprint: %s\n", plan.Sprint)
	fmt.Fprintf(w, "================================\n\n")

	// Branches
	fmt.Fprintf(w, "Branches to delete (%d):\n", len(plan.Branches))
	for _, b := range plan.Branches {
		status := "✓"
		if !b.CanDelete {
			if b.HasUnmerged {
				status = "⚠ (unmerged)"
			} else if b.HasOpenPR {
				status = "⚠ (open PR)"
			}
		}
		fmt.Fprintf(w, "  %s %s (issue #%d)\n", status, b.Name, b.IssueNumber)
	}
	if len(plan.Branches) == 0 {
		fmt.Fprintf(w, "  (none)\n")
	}
	fmt.Fprintln(w)

	// Worktrees
	fmt.Fprintf(w, "Worktrees to remove (%d):\n", len(plan.Worktrees))
	for _, wt := range plan.Worktrees {
		fmt.Fprintf(w, "  %s (branch: %s)\n", wt.Path, wt.Branch)
	}
	if len(plan.Worktrees) == 0 {
		fmt.Fprintf(w, "  (none)\n")
	}
	fmt.Fprintln(w)

	// Temp files
	fmt.Fprintf(w, "Temp files to delete (%d):\n", len(plan.TempFiles))
	for _, tf := range plan.TempFiles {
		fmt.Fprintf(w, "  %s (modified: %s)\n", tf.Path, tf.ModifiedAt.Format("2006-01-02"))
	}
	if len(plan.TempFiles) == 0 {
		fmt.Fprintf(w, "  (none)\n")
	}
	fmt.Fprintln(w)
}

// PrintResult displays the results of a cleanup operation
func PrintResult(result *CleanupResult, w io.Writer) {
	fmt.Fprintf(w, "\nCleanup Results:\n")
	fmt.Fprintf(w, "================\n")
	fmt.Fprintf(w, "Branches deleted:  %d\n", result.DeletedBranches)
	fmt.Fprintf(w, "Worktrees removed: %d\n", result.DeletedWorktrees)
	fmt.Fprintf(w, "Temp files deleted: %d\n", result.DeletedTempFiles)
	fmt.Fprintf(w, "Branches skipped:  %d\n", result.SkippedBranches)

	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "\nErrors (%d):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Fprintf(w, "  ! %v\n", err)
		}
	}
	fmt.Fprintln(w)
}
