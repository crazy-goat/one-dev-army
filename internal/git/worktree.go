package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Name   string
	Path   string
	Branch string
}

type WorktreeManager struct {
	repoDir      string
	worktreesDir string
}

func NewWorktreeManager(repoDir, worktreesDir string) *WorktreeManager {
	return &WorktreeManager{
		repoDir:      repoDir,
		worktreesDir: worktreesDir,
	}
}

func (m *WorktreeManager) Create(workerName, branch string) (*Worktree, error) {
	wtPath := filepath.Join(m.worktreesDir, workerName)

	if _, err := os.Stat(wtPath); err == nil {
		if err := m.Remove(workerName); err != nil {
			return nil, fmt.Errorf("removing existing worktree %s: %w", workerName, err)
		}
	}

	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git worktree add: %w\n%s", err, out)
	}

	return &Worktree{
		Name:   workerName,
		Path:   wtPath,
		Branch: branch,
	}, nil
}

func (m *WorktreeManager) Remove(workerName string) error {
	wtPath := filepath.Join(m.worktreesDir, workerName)

	worktrees, err := m.List()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	var branch string
	for _, wt := range worktrees {
		if wt.Path == wtPath {
			branch = wt.Branch
			break
		}
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}

	if branch != "" {
		cmd = exec.Command("git", "branch", "-D", branch)
		cmd.Dir = m.repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git branch -D %s: %w\n%s", branch, err, out)
		}
	}

	return nil
}

func (m *WorktreeManager) List() ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w\n%s", err, out)
	}

	return parsePorcelain(string(out)), nil
}

func parsePorcelain(output string) []Worktree {
	var worktrees []Worktree
	blocks := strings.Split(strings.TrimSpace(output), "\n\n")

	for _, block := range blocks {
		if block == "" {
			continue
		}

		var wt Worktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimPrefix(line, "worktree ")
				wt.Name = filepath.Base(wt.Path)
			case strings.HasPrefix(line, "branch "):
				ref := strings.TrimPrefix(line, "branch ")
				wt.Branch = strings.TrimPrefix(ref, "refs/heads/")
			}
		}

		if wt.Path != "" {
			worktrees = append(worktrees, wt)
		}
	}

	return worktrees
}

func RunInWorktree(wtPath, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = wtPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("running %s in %s: %w\n%s", name, wtPath, err, out)
	}
	return out, nil
}
