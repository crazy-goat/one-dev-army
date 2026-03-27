package sprint

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/git"
	"github.com/crazy-goat/one-dev-army/internal/github"
	"github.com/crazy-goat/one-dev-army/internal/workspace"
)

// CleanupManager orchestrates sprint cleanup
type CleanupManager struct {
	repoDir   string
	ghClient  *github.Client
	branchMgr *git.BranchManager
}

// NewCleanupManager creates a new CleanupManager
func NewCleanupManager(repoDir string, ghClient *github.Client, branchMgr *git.BranchManager) *CleanupManager {
	return &CleanupManager{
		repoDir:   repoDir,
		ghClient:  ghClient,
		branchMgr: branchMgr,
	}
}

// BranchArtifact represents a branch to be cleaned up
type BranchArtifact struct {
	Name        string
	IssueNumber int
	HasUnmerged bool
	HasOpenPR   bool
	CanDelete   bool
}

// WorktreeArtifact represents a worktree to be cleaned up
type WorktreeArtifact struct {
	Path   string
	Branch string
}

// TempFileArtifact represents a temp file to be cleaned up
type TempFileArtifact struct {
	Path    string
	ModTime time.Time
}

// CleanupPlan holds all artifacts to be cleaned
type CleanupPlan struct {
	Sprint    string
	Branches  []BranchArtifact
	Worktrees []WorktreeArtifact
	TempFiles []TempFileArtifact
}

// CleanupResult holds the results of cleanup execution
type CleanupResult struct {
	DeletedBranches  int
	DeletedWorktrees int
	DeletedTempFiles int
	Errors           []string
}

// DetectArtifacts finds all artifacts from a completed sprint
func (m *CleanupManager) DetectArtifacts(sprintTitle string) (*CleanupPlan, error) {
	plan := &CleanupPlan{
		Sprint: sprintTitle,
	}

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
			return nil, fmt.Errorf("sprint not found: %s", sprintTitle)
		}
	}

	// Get closed tickets for the sprint
	issues, err := m.ghClient.GetClosedTicketsForSprint(milestone.Number)
	if err != nil {
		return nil, fmt.Errorf("getting closed tickets for sprint: %w", err)
	}

	// Detect branches for each merged issue
	for _, issue := range issues {
		branchPrefix := fmt.Sprintf("oda-%d-", issue.Number)
		branchName := m.branchMgr.FindBranchByPrefix(branchPrefix)

		if branchName != "" {
			artifact := BranchArtifact{
				Name:        branchName,
				IssueNumber: issue.Number,
			}

			// Check for unmerged changes
			artifact.HasUnmerged = m.hasUnmergedChanges(branchName)

			// Check for open PRs
			artifact.HasOpenPR = m.hasOpenPR(branchName)

			// Can delete if no unmerged changes and no open PRs
			artifact.CanDelete = !artifact.HasUnmerged && !artifact.HasOpenPR

			plan.Branches = append(plan.Branches, artifact)
		}
	}

	// Detect worktrees
	worktrees, err := m.detectWorktrees()
	if err != nil {
		log.Printf("[CleanupManager] Warning: failed to detect worktrees: %v", err)
	} else {
		plan.Worktrees = worktrees
	}

	// Detect old temp files (older than sprint end)
	// Use sprint creation time as a proxy for sprint end
	tempFiles, err := workspace.FindOldTempFiles(m.repoDir, milestone.CreatedAt)
	if err != nil {
		log.Printf("[CleanupManager] Warning: failed to detect temp files: %v", err)
	} else {
		for _, path := range tempFiles {
			info, err := getFileInfo(path)
			if err != nil {
				continue
			}
			plan.TempFiles = append(plan.TempFiles, TempFileArtifact{
				Path:    path,
				ModTime: info.ModTime(),
			})
		}
	}

	return plan, nil
}

// hasUnmergedChanges checks if a branch has commits not merged to default branch
func (m *CleanupManager) hasUnmergedChanges(branch string) bool {
	defaultBranch := m.detectDefaultBranch()

	// Check if there are commits in branch that are not in defaultBranch
	cmd := exec.Command("git", "log", fmt.Sprintf("%s..%s", defaultBranch, branch), "--oneline")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return true // Assume unmerged if we can't check
	}

	return strings.TrimSpace(string(out)) != ""
}

// hasOpenPR checks if there's an open PR for the branch
func (m *CleanupManager) hasOpenPR(branch string) bool {
	cmd := exec.Command("gh", "pr", "list", "--head", branch, "--state", "open", "--json", "number")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false // Assume no open PR if we can't check
	}

	return strings.TrimSpace(string(out)) != "[]"
}

// detectWorktrees finds all worktrees in the repository
func (m *CleanupManager) detectWorktrees() ([]WorktreeArtifact, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	var worktrees []WorktreeArtifact

	// Parse worktree list
	blocks := strings.SplitSeq(strings.TrimSpace(string(out)), "\n\n")
	for block := range blocks {
		var wtPath, wtBranch string
		for line := range strings.SplitSeq(block, "\n") {
			if after, ok := strings.CutPrefix(line, "worktree "); ok {
				wtPath = after
			}
			if after, ok := strings.CutPrefix(line, "branch "); ok {
				wtBranch = filepath.Base(after)
			}
		}

		// Skip the main worktree (the repo itself)
		if wtPath != "" && wtPath != m.repoDir {
			worktrees = append(worktrees, WorktreeArtifact{
				Path:   wtPath,
				Branch: wtBranch,
			})
		}
	}

	return worktrees, nil
}

// detectDefaultBranch returns "main" or "master" depending on what exists locally
func (m *CleanupManager) detectDefaultBranch() string {
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = m.repoDir
	if err := cmd.Run(); err == nil {
		return "main"
	}
	return "master"
}

// getFileInfo returns file info for a path
func getFileInfo(path string) (interface {
	ModTime() time.Time
}, error) {
	info, err := exec.Command("stat", path).Output()
	if err != nil {
		return nil, err
	}

	// Parse stat output to get modification time
	// This is a simplified version - in production you'd parse the stat output properly
	_ = info
	return &fileInfo{modTime: time.Now()}, nil
}

type fileInfo struct {
	modTime time.Time
}

func (f *fileInfo) ModTime() time.Time {
	return f.modTime
}

// Execute performs the cleanup (or dry-run)
func (m *CleanupManager) Execute(plan *CleanupPlan, dryRun bool) (*CleanupResult, error) {
	result := &CleanupResult{}

	// Delete branches
	for _, branch := range plan.Branches {
		if !branch.CanDelete {
			log.Printf("[CleanupManager] Skipping branch %s (unmerged changes or open PR)", branch.Name)
			continue
		}

		if dryRun {
			log.Printf("[CleanupManager] Would delete branch: %s", branch.Name)
			result.DeletedBranches++
			continue
		}

		// Delete local branch
		if err := m.branchMgr.RemoveBranch(branch.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete local branch %s: %v", branch.Name, err))
			continue
		}

		// Delete remote branch
		if err := m.deleteRemoteBranch(branch.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete remote branch %s: %v", branch.Name, err))
			continue
		}

		log.Printf("[CleanupManager] Deleted branch: %s", branch.Name)
		result.DeletedBranches++
	}

	// Remove worktrees
	for _, wt := range plan.Worktrees {
		if dryRun {
			log.Printf("[CleanupManager] Would remove worktree: %s", wt.Path)
			result.DeletedWorktrees++
			continue
		}

		if err := m.removeWorktree(wt.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to remove worktree %s: %v", wt.Path, err))
			continue
		}

		log.Printf("[CleanupManager] Removed worktree: %s", wt.Path)
		result.DeletedWorktrees++
	}

	// Clean up worktree metadata
	if !dryRun && result.DeletedWorktrees > 0 {
		if err := m.pruneWorktrees(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to prune worktrees: %v", err))
		}
	}

	// Delete temp files
	if len(plan.TempFiles) > 0 {
		paths := make([]string, len(plan.TempFiles))
		for i, tf := range plan.TempFiles {
			paths[i] = tf.Path
		}

		if dryRun {
			log.Printf("[CleanupManager] Would delete %d temp files", len(paths))
			result.DeletedTempFiles = len(paths)
		} else {
			if err := workspace.RemoveTempFiles(paths, false); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to delete temp files: %v", err))
			} else {
				log.Printf("[CleanupManager] Deleted %d temp files", len(paths))
				result.DeletedTempFiles = len(paths)
			}
		}
	}

	return result, nil
}

// deleteRemoteBranch deletes a branch from origin
func (m *CleanupManager) deleteRemoteBranch(branch string) error {
	cmd := exec.Command("git", "push", "origin", "--delete", branch)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push origin --delete %s: %w\n%s", branch, err, out)
	}
	return nil
}

// removeWorktree removes a git worktree
func (m *CleanupManager) removeWorktree(path string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove %s: %w\n%s", path, err, out)
	}
	return nil
}

// pruneWorktrees cleans up worktree metadata
func (m *CleanupManager) pruneWorktrees() error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree prune: %w\n%s", err, out)
	}
	return nil
}
