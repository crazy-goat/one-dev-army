package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestCreateAndRemoveWorktree(t *testing.T) {
	repoDir := setupRepo(t)
	wtDir := filepath.Join(repoDir, "worktrees")
	mgr := git.NewWorktreeManager(repoDir, wtDir)

	wt, err := mgr.Create("worker-1", "branch-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if wt.Name != "worker-1" {
		t.Errorf("Name = %q, want %q", wt.Name, "worker-1")
	}
	if wt.Branch != "branch-1" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "branch-1")
	}

	expectedPath := filepath.Join(wtDir, "worker-1")
	if wt.Path != expectedPath {
		t.Errorf("Path = %q, want %q", wt.Path, expectedPath)
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree directory should exist: %v", err)
	}

	if err := mgr.Remove("worker-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree directory should be removed, got err: %v", err)
	}
}

func TestCreateWorktreeAlreadyExists(t *testing.T) {
	repoDir := setupRepo(t)
	wtDir := filepath.Join(repoDir, "worktrees")
	mgr := git.NewWorktreeManager(repoDir, wtDir)

	_, err := mgr.Create("worker-1", "branch-a")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	wt, err := mgr.Create("worker-1", "branch-b")
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}

	if wt.Branch != "branch-b" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "branch-b")
	}

	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree directory should exist after re-create: %v", err)
	}
}

func TestList(t *testing.T) {
	repoDir := setupRepo(t)
	wtDir := filepath.Join(repoDir, "worktrees")
	mgr := git.NewWorktreeManager(repoDir, wtDir)

	for i, name := range []string{"worker-1", "worker-2"} {
		branch := "branch-" + name
		if _, err := mgr.Create(name, branch); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	worktrees, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// main worktree + 2 created = 3
	if len(worktrees) != 3 {
		t.Fatalf("len(worktrees) = %d, want 3", len(worktrees))
	}
}
