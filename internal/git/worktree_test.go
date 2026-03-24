package git_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/git"
)

// setupRepo creates a bare "origin" repo and a local clone so that
// git fetch origin / git reset --hard origin/master work in tests.
func setupRepo(t *testing.T) string {
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

	// Create an initial commit and push to origin
	runGit(cloneDir, "commit", "--allow-empty", "-m", "init")
	runGit(cloneDir, "push", "origin", "master")

	return cloneDir
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %v\n%s", err, out)
	}
	return string(out[:len(out)-1]) // trim newline
}

func TestCreateAndRemoveBranch(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	if err := mgr.CreateBranch("feature-1"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if got := currentBranch(t, repoDir); got != "feature-1" {
		t.Errorf("current branch = %q, want %q", got, "feature-1")
	}

	if err := mgr.RemoveBranch("feature-1"); err != nil {
		t.Fatalf("RemoveBranch: %v", err)
	}

	// Should be back on default branch
	got := currentBranch(t, repoDir)
	if got != "master" && got != "main" {
		t.Errorf("current branch = %q, want master or main", got)
	}
}

func TestCreateBranchAlreadyExists(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	if err := mgr.CreateBranch("branch-a"); err != nil {
		t.Fatalf("first CreateBranch: %v", err)
	}

	// Switch back to default
	defaultBranch := currentBranch(t, repoDir)
	_ = defaultBranch
	cmd := exec.Command("git", "checkout", "master")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// try main
		cmd2 := exec.Command("git", "checkout", "main")
		cmd2.Dir = repoDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			t.Fatalf("checkout default: %v\n%s\n%s", err, out, out2)
		}
	}

	// Creating same branch again should succeed (checkout existing)
	if err := mgr.CreateBranch("branch-a"); err != nil {
		t.Fatalf("second CreateBranch: %v", err)
	}

	if got := currentBranch(t, repoDir); got != "branch-a" {
		t.Errorf("current branch = %q, want %q", got, "branch-a")
	}
}

func TestRunInDir(t *testing.T) {
	repoDir := setupRepo(t)

	out, err := git.RunInDir(repoDir, "git", "status")
	if err != nil {
		t.Fatalf("RunInDir: %v", err)
	}

	if len(out) == 0 {
		t.Error("expected non-empty output from git status")
	}
}

func TestRepoDir(t *testing.T) {
	mgr := git.NewBranchManager("/some/path")
	if mgr.RepoDir() != "/some/path" {
		t.Errorf("RepoDir() = %q, want %q", mgr.RepoDir(), "/some/path")
	}
}

// TestLegacyAliases verifies backward-compatible aliases work
func TestLegacyAliases(t *testing.T) {
	repoDir := setupRepo(t)

	// NewWorktreeManager should return a BranchManager
	mgr := git.NewWorktreeManager(repoDir, "/ignored")
	if mgr.RepoDir() != repoDir {
		t.Errorf("RepoDir() = %q, want %q", mgr.RepoDir(), repoDir)
	}

	// RunInWorktree should work as alias for RunInDir
	out, err := git.RunInWorktree(repoDir, "git", "status")
	if err != nil {
		t.Fatalf("RunInWorktree: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty output")
	}
}

// TestCreateBranchFromLatestOrigin verifies that CreateBranch bases the new
// branch on the latest origin/master, even when the local repo is behind.
func TestCreateBranchFromLatestOrigin(t *testing.T) {
	repoDir := setupRepo(t)

	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	}

	// Record the current local master commit
	localHead := func() string {
		t.Helper()
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("rev-parse HEAD: %v\n%s", err, out)
		}
		return string(out[:len(out)-1])
	}

	oldCommit := localHead()

	// Simulate another developer pushing a new commit to origin.
	// We do this by cloning origin into a second working copy, committing,
	// and pushing — so origin advances but our local repo doesn't know yet.
	originURL := func() string {
		t.Helper()
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("get-url origin: %v\n%s", err, out)
		}
		return string(out[:len(out)-1])
	}()

	otherClone := t.TempDir()
	for _, args := range [][]string{
		{"clone", originURL, "."},
		{"commit", "--allow-empty", "-m", "remote advance"},
		{"push", "origin", "master"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = otherClone
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("other clone git %v: %v\n%s", args, err, out)
		}
	}

	// Now CreateBranch in our original repo — it should fetch and base
	// the new branch on the advanced origin/master, not the stale local one.
	mgr := git.NewBranchManager(repoDir)
	if err := mgr.CreateBranch("feature-fresh"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if got := currentBranch(t, repoDir); got != "feature-fresh" {
		t.Errorf("current branch = %q, want %q", got, "feature-fresh")
	}

	newCommit := localHead()
	if newCommit == oldCommit {
		t.Errorf("branch was created from stale local master (%s); expected latest origin commit", oldCommit)
	}
}

// TestCreateBranchExistingRebasesOntoOrigin verifies that when a branch already
// exists and origin has advanced, CreateBranch rebases the branch onto the latest
// origin/master to prevent merge conflicts.
func TestCreateBranchExistingRebasesOntoOrigin(t *testing.T) {
	repoDir := setupRepo(t)

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

	gitOutput := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	mgr := git.NewBranchManager(repoDir)

	// Create a feature branch with a commit
	if err := mgr.CreateBranch("feature-rebase"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Add a file and commit on the feature branch
	if err := os.WriteFile(repoDir+"/feature.txt", []byte("feature work"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runGit(repoDir, "add", "feature.txt")
	runGit(repoDir, "commit", "-m", "feature commit")

	// Switch back to master
	runGit(repoDir, "checkout", "master")

	// Simulate origin advancing: push a new commit via a second clone
	originURL := gitOutput(repoDir, "remote", "get-url", "origin")
	otherClone := t.TempDir()
	for _, args := range [][]string{
		{"clone", originURL, "."},
		{"commit", "--allow-empty", "-m", "origin advanced"},
		{"push", "origin", "master"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = otherClone
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("other clone git %v: %v\n%s", args, err, out)
		}
	}

	// Now CreateBranch again — should checkout existing branch AND rebase onto origin/master
	if err := mgr.CreateBranch("feature-rebase"); err != nil {
		t.Fatalf("second CreateBranch: %v", err)
	}

	if got := currentBranch(t, repoDir); got != "feature-rebase" {
		t.Errorf("current branch = %q, want %q", got, "feature-rebase")
	}

	// Verify the feature commit is still present (rebase preserved it)
	logOutput := gitOutput(repoDir, "log", "--oneline")
	if !strings.Contains(logOutput, "feature commit") {
		t.Errorf("feature commit lost after rebase; log:\n%s", logOutput)
	}

	// Verify origin's "origin advanced" commit is in the history (rebase applied it)
	if !strings.Contains(logOutput, "origin advanced") {
		t.Errorf("origin advance commit not in history after rebase; log:\n%s", logOutput)
	}
}

// TestRemoveBranchNonExistent verifies that RemoveBranch handles non-existent branches gracefully
func TestRemoveBranchNonExistent(t *testing.T) {
	repoDir := setupRepo(t)
	mgr := git.NewBranchManager(repoDir)

	// Try to remove a branch that doesn't exist - should not error
	if err := mgr.RemoveBranch("non-existent-branch"); err != nil {
		t.Errorf("RemoveBranch(non-existent) = %v, want nil", err)
	}

	// Verify we're still on the default branch
	got := currentBranch(t, repoDir)
	if got != "master" && got != "main" {
		t.Errorf("current branch = %q, want master or main", got)
	}
}
