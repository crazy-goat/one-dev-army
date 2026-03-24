package git

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// BranchManager manages git branches for the single-worker pipeline.
// Replaces WorktreeManager — uses regular git checkout instead of worktrees.
type BranchManager struct {
	repoDir string
}

// NewBranchManager creates a new BranchManager for the given repo directory.
func NewBranchManager(repoDir string) *BranchManager {
	return &BranchManager{repoDir: repoDir}
}

// RepoDir returns the repository root directory.
func (m *BranchManager) RepoDir() string {
	return m.repoDir
}

// CreateBranch creates a new branch and checks it out.
// If the branch already exists, it checks it out. If a workerName-based
// worktree exists from a previous run, it is cleaned up first.
func (m *BranchManager) CreateBranch(branch string) error {
	// Clean up any leftover worktrees from previous runs
	m.cleanupLegacyWorktrees()

	// Try creating a new branch
	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Branch might already exist — try checking it out
		if strings.Contains(string(out), "already exists") {
			cmd = exec.Command("git", "checkout", branch)
			cmd.Dir = m.repoDir
			if out2, err2 := cmd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git checkout %s: %w\n%s", branch, err2, out2)
			}
			return nil
		}
		return fmt.Errorf("git checkout -b %s: %w\n%s", branch, err, out)
	}
	return nil
}

// RemoveBranch switches back to the default branch and deletes the given branch.
// If the branch doesn't exist, it returns nil (no error) and logs a warning.
func (m *BranchManager) RemoveBranch(branch string) error {
	// First check if branch exists
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = m.repoDir
	if err := cmd.Run(); err != nil {
		// Branch doesn't exist - not an error, just log and return
		log.Printf("[BranchManager] Branch %q does not exist, skipping deletion", branch)
		return nil //nolint:nilerr // intentional - branch not existing is not an error
	}

	// Switch to main/master first
	defaultBranch := m.detectDefaultBranch()
	cmd = exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", defaultBranch, err, out)
	}

	// Delete the branch
	cmd = exec.Command("git", "branch", "-D", branch)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %w\n%s", branch, err, out)
	}

	log.Printf("[BranchManager] Deleted branch %q", branch)
	return nil
}

// PushBranch pushes the given branch to origin.
func (m *BranchManager) PushBranch(branch string) error {
	// Try force-with-lease first (safe for existing remote branches)
	cmd := exec.Command("git", "push", "-u", "--force-with-lease", "origin", branch)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// If stale info or no upstream, fall back to regular push
	if strings.Contains(string(out), "stale info") || strings.Contains(string(out), "no upstream") {
		cmd = exec.Command("git", "push", "-u", "origin", branch)
		cmd.Dir = m.repoDir
		out, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git push -u origin %s: %w\n%s", branch, err, out)
		}
		return nil
	}

	return fmt.Errorf("git push -u --force-with-lease origin %s: %w\n%s", branch, err, out)
}

// RunInDir executes a command in the repo directory.
func RunInDir(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running %s in %s: %w\n%s", name, dir, err, out)
	}
	return out, nil
}

// detectDefaultBranch returns "main" or "master" depending on what exists.
func (m *BranchManager) detectDefaultBranch() string {
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = m.repoDir
	if err := cmd.Run(); err == nil {
		return "main"
	}
	return "master"
}

// cleanupLegacyWorktrees removes any leftover git worktrees from previous runs.
func (m *BranchManager) cleanupLegacyWorktrees() {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	// Parse worktree list — remove any non-main worktrees
	blocks := strings.SplitSeq(strings.TrimSpace(string(out)), "\n\n")
	for block := range blocks {
		var wtPath string
		for line := range strings.SplitSeq(block, "\n") {
			if after, ok := strings.CutPrefix(line, "worktree "); ok {
				wtPath = after
			}
		}
		// Skip the main worktree (the repo itself)
		if wtPath != "" && wtPath != m.repoDir {
			rmCmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
			rmCmd.Dir = m.repoDir
			_ = rmCmd.Run()
		}
	}
}

// Legacy aliases for backward compatibility during migration

// WorktreeManager is an alias for BranchManager (legacy compatibility).
type WorktreeManager = BranchManager

// NewWorktreeManager creates a BranchManager. The worktreesDir parameter is ignored.
func NewWorktreeManager(repoDir, _ string) *BranchManager {
	return NewBranchManager(repoDir)
}

// Worktree is kept for struct compatibility but Path always equals repoDir.
type Worktree struct {
	Name   string
	Path   string
	Branch string
}

// RunInWorktree is a legacy alias for RunInDir.
func RunInWorktree(dir, name string, args ...string) ([]byte, error) {
	return RunInDir(dir, name, args...)
}
