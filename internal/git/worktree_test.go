package git_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/crazy-goat/one-dev-army/internal/git"
)

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	}

	for _, args := range [][]string{
		{"init"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return dir
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
