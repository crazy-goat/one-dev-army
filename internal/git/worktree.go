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

// CreateBranch fetches the latest remote state and creates a new branch from
// origin/<defaultBranch>. This ensures the branch always starts from the
// newest remote HEAD, avoiding merge conflicts caused by a stale local base.
// If the branch already exists locally, it checks it out.
func (m *BranchManager) CreateBranch(branch string) error {
	// Clean up any leftover worktrees from previous runs
	m.cleanupLegacyWorktrees()

	// Fetch latest remote state and determine the start point.
	// When origin is available we branch from origin/<default>;
	// otherwise (e.g. in tests without a remote) we fall back to HEAD.
	startPoint := "HEAD"
	if out, err := exec.Command("git", "-C", m.repoDir, "fetch", "origin").CombinedOutput(); err == nil {
		if remote := m.detectRemoteDefaultBranch(); remote != "" {
			startPoint = "origin/" + remote
		}
	} else {
		log.Printf("[BranchManager] git fetch origin failed (no remote?), branching from HEAD: %s", strings.TrimSpace(string(out)))
	}

	// Create the feature branch directly from the start point — no need
	// to checkout the default branch first.
	cmd := exec.Command("git", "checkout", "-b", branch, startPoint)
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

			// Rebase existing branch onto the latest remote default branch
			// to prevent merge conflicts when the PR is eventually merged.
			if startPoint != "HEAD" {
				log.Printf("[BranchManager] Rebasing existing branch %q onto %s", branch, startPoint)
				rebaseCmd := exec.Command("git", "rebase", startPoint)
				rebaseCmd.Dir = m.repoDir
				if rbOut, rbErr := rebaseCmd.CombinedOutput(); rbErr != nil {
					// Abort the failed rebase so the worktree stays clean
					abortCmd := exec.Command("git", "rebase", "--abort")
					abortCmd.Dir = m.repoDir
					_ = abortCmd.Run()
					log.Printf("[BranchManager] Rebase of %q onto %s failed, continuing without rebase: %s",
						branch, startPoint, strings.TrimSpace(string(rbOut)))
				}
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

// detectDefaultBranch returns "main" or "master" depending on what exists locally.
func (m *BranchManager) detectDefaultBranch() string {
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = m.repoDir
	if err := cmd.Run(); err == nil {
		return "main"
	}
	return "master"
}

// CheckoutDefault switches to the default branch (main or master).
// This is called after a successful merge to prepare the repository
// for the next ticket. Errors are logged but not returned as this
// is a non-critical cleanup operation.
func (m *BranchManager) CheckoutDefault() error {
	defaultBranch := m.detectDefaultBranch()
	cmd := exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", defaultBranch, err, out)
	}
	return nil
}

// detectRemoteDefaultBranch returns the default branch name on origin
// (e.g. "main" or "master") by inspecting origin/HEAD. Returns "" if
// the remote has no HEAD or the remote doesn't exist.
func (m *BranchManager) detectRemoteDefaultBranch() string {
	// git symbolic-ref refs/remotes/origin/HEAD → refs/remotes/origin/master
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		// "refs/remotes/origin/master\n" → "master"
		ref := strings.TrimSpace(string(out))
		if after, ok := strings.CutPrefix(ref, "refs/remotes/origin/"); ok {
			return after
		}
	}

	// Fallback: check which remote tracking branches exist
	for _, name := range []string{"main", "master"} {
		cmd = exec.Command("git", "rev-parse", "--verify", "origin/"+name)
		cmd.Dir = m.repoDir
		if cmd.Run() == nil {
			return name
		}
	}

	return ""
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

// FindBranchByPrefix finds a local branch matching the given prefix.
// Returns the full branch name if found, or empty string if no match.
// This is used to find branches like "oda-{num}-*" when we don't know the slug.
func (m *BranchManager) FindBranchByPrefix(prefix string) string {
	cmd := exec.Command("git", "branch", "--list", "--format=%(refname:short)")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[BranchManager] Error listing branches: %v", err)
		return ""
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		branch := strings.TrimSpace(line)
		if strings.HasPrefix(branch, prefix) {
			return branch
		}
	}

	return ""
}

// HasCommitsDifferentFromMaster checks if the given branch has any commits
// that are not in master. Returns true if there are new commits, false if
// the branch is identical to master.
func (m *BranchManager) HasCommitsDifferentFromMaster(branch string) (bool, error) {
	// Get the default branch (main or master)
	defaultBranch := m.detectDefaultBranch()

	// Get merge base between default branch and the target branch
	cmd := exec.Command("git", "merge-base", defaultBranch, branch)
	cmd.Dir = m.repoDir
	mergeBase, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("getting merge base: %w", err)
	}

	// Get HEAD of the target branch
	cmd = exec.Command("git", "rev-parse", branch)
	cmd.Dir = m.repoDir
	head, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("getting branch HEAD: %w", err)
	}

	// If merge base equals HEAD, branch has no new commits
	mergeBaseStr := strings.TrimSpace(string(mergeBase))
	headStr := strings.TrimSpace(string(head))

	return mergeBaseStr != headStr, nil
}

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
