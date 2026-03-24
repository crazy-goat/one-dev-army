# Embed OpenCode Skills Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Embed the `skills/creating-oda-ticket/SKILL.md` file into the ODA binary and deploy it to `.opencode/skills/creating-oda-ticket/` in the working directory before opencode server starts.

**Architecture:** Create a new `internal/skills` package that uses `//go:embed` to bundle skill files into the binary. A single exported function `Deploy(dir string) error` copies all embedded skills to `<dir>/.opencode/skills/`. This function is called in `main.go`'s `runServe()` before the opencode server starts (before preflight checks that contact opencode). The design is extensible — adding new skills only requires dropping files into `skills/` and updating the embed directive.

**Tech Stack:** Go `embed.FS`, `os.MkdirAll`, `os.WriteFile`, standard library only.

---

### Task 1: Create `internal/skills` package with embedded skill files

**Files:**
- Create: `internal/skills/skills.go`

**Step 1: Write the failing test**

Create `internal/skills/skills_test.go`:

```go
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeploy_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()

	if err := Deploy(dir); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	// Verify the skill file was created
	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("skill file not found: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected file, got directory")
	}
	if info.Size() == 0 {
		t.Fatal("skill file is empty")
	}

	// Verify content is non-trivial (contains the skill name)
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading skill file: %v", err)
	}
	if len(content) < 100 {
		t.Errorf("skill file suspiciously small: %d bytes", len(content))
	}
}

func TestDeploy_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()

	// Create a stale file
	skillDir := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleContent := []byte("old content")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), staleContent, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Deploy(dir); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	// Verify the file was overwritten with the embedded content
	content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == string(staleContent) {
		t.Error("skill file was not overwritten")
	}
}

func TestDeploy_DirectoryAlreadyExists(t *testing.T) {
	dir := t.TempDir()

	// Pre-create the directory structure
	skillDir := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Deploy should succeed even if directory exists
	if err := Deploy(dir); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	// Verify the file was created
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("skill file not found: %v", err)
	}
}

func TestDeploy_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	if err := Deploy(dir); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatal(err)
	}

	// File should be readable (0644)
	perm := info.Mode().Perm()
	if perm != 0o644 {
		t.Errorf("file permissions = %o, want %o", perm, 0o644)
	}
}

func TestDeploy_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Deploy twice — should not error
	if err := Deploy(dir); err != nil {
		t.Fatalf("first Deploy() error: %v", err)
	}
	if err := Deploy(dir); err != nil {
		t.Fatalf("second Deploy() error: %v", err)
	}

	// Verify file still exists and is valid
	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 100 {
		t.Errorf("skill file suspiciously small after double deploy: %d bytes", len(content))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/skills/ -v`
Expected: FAIL — package does not exist yet

**Step 3: Write the implementation**

Create `internal/skills/skills.go`:

```go
// Package skills embeds opencode skill files and deploys them to the working directory.
// Skills are deployed to <dir>/.opencode/skills/ before opencode starts,
// making them available to all opencode sessions.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// skillFiles embeds the entire skills directory from the repository root.
// The path is relative to this Go source file, so we use ../../skills
// to reach the top-level skills/ directory.
//
//go:embed all:data
var skillFiles embed.FS

// Deploy copies all embedded skill files to <dir>/.opencode/skills/.
// It creates directories as needed and overwrites existing files.
// This must be called before opencode server starts.
func Deploy(dir string) error {
	targetBase := filepath.Join(dir, ".opencode", "skills")

	return fs.WalkDir(skillFiles, "data", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking embedded skills: %w", err)
		}

		// Strip the "data" prefix to get the relative path
		relPath, err := filepath.Rel("data", path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}

		// Skip the root "data" directory itself
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(targetBase, relPath)

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("creating skill directory %s: %w", relPath, err)
			}
			return nil
		}

		content, err := skillFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded file %s: %w", path, err)
		}

		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return fmt.Errorf("writing skill file %s: %w", relPath, err)
		}

		return nil
	})
}
```

**Important:** The `//go:embed` directive cannot reference paths outside the package directory using `../../`. Instead, we need a `data/` subdirectory inside `internal/skills/` that contains the skill files. We have two options:

- **Option A (symlink):** Create a symlink `internal/skills/data` → `../../skills` — but `go:embed` does **not** follow symlinks.
- **Option B (copy at build time):** Use a build script — adds complexity.
- **Option C (embed from root, pass to package):** Embed in `main.go` and pass the `embed.FS` to the skills package.

**We'll use Option C** — embed in a thin package at the root level and pass the FS to the deploy function. Actually, the cleanest Go approach is:

**Revised approach:** Place the embed directive in a package that lives alongside the skill files. Since `skills/` is at the repo root, we create `internal/skills/skills.go` that receives the `embed.FS` as a parameter. The `//go:embed` directive lives in `main.go` (which is at the repo root and CAN reference `skills/*`).

**Step 3 (revised): Write the implementation**

Create `internal/skills/skills.go`:

```go
// Package skills deploys opencode skill files to the working directory.
// Skills are deployed to <dir>/.opencode/skills/ before opencode starts,
// making them available to all opencode sessions.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Deploy copies all files from the embedded filesystem to <dir>/.opencode/skills/.
// The embedded FS should have skill directories at its root (e.g., "creating-oda-ticket/SKILL.md").
// It creates directories as needed and overwrites existing files.
// This must be called before opencode server starts.
func Deploy(dir string, skillFS embed.FS) error {
	targetBase := filepath.Join(dir, ".opencode", "skills")

	return fs.WalkDir(skillFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking embedded skills: %w", err)
		}

		// Skip the root directory itself
		if path == "." {
			return nil
		}

		targetPath := filepath.Join(targetBase, path)

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("creating skill directory %s: %w", path, err)
			}
			return nil
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", path, err)
		}

		content, err := skillFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded file %s: %w", path, err)
		}

		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return fmt.Errorf("writing skill file %s: %w", path, err)
		}

		return nil
	})
}
```

Update the test to pass an `embed.FS` — but since tests can't use `//go:embed` with dynamic content, we'll use `testing/fstest.MapFS` to simulate:

Update `internal/skills/skills_test.go`:

```go
package skills

import (
	"embed"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// testSkillFS creates a mock filesystem for testing.
// We can't use embed.FS in tests directly, so we test with the
// fs.FS interface by changing Deploy to accept fs.FS instead of embed.FS.
```

**Wait — `embed.FS` implements `fs.FS`.** We should change the function signature to accept `fs.FS` so tests can use `fstest.MapFS`. This is better design anyway.

**Step 3 (final): Write the implementation**

Create `internal/skills/skills.go`:

```go
// Package skills deploys opencode skill files to the working directory.
// Skills are deployed to <dir>/.opencode/skills/ before opencode starts,
// making them available to all opencode sessions.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Deploy copies all files from the given filesystem to <dir>/.opencode/skills/.
// The FS should have skill directories at its root (e.g., "creating-oda-ticket/SKILL.md").
// It creates directories as needed and overwrites existing files.
// This must be called before opencode server starts.
func Deploy(dir string, skillFS fs.FS) error {
	targetBase := filepath.Join(dir, ".opencode", "skills")

	return fs.WalkDir(skillFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking embedded skills: %w", err)
		}

		if path == "." {
			return nil
		}

		targetPath := filepath.Join(targetBase, path)

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("creating skill directory %s: %w", path, err)
			}
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", path, err)
		}

		content, err := fs.ReadFile(skillFS, path)
		if err != nil {
			return fmt.Errorf("reading embedded file %s: %w", path, err)
		}

		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return fmt.Errorf("writing skill file %s: %w", path, err)
		}

		return nil
	})
}
```

Update `internal/skills/skills_test.go` to use `fstest.MapFS`:

```go
package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var testSkillContent = `---
name: creating-oda-ticket
description: Test skill for creating ODA tickets
---

# Creating an ODA Ticket

This is a test skill file with enough content to pass the size check.
It simulates the real SKILL.md that would be embedded in the binary.
The content here is intentionally longer than 100 bytes to satisfy tests.
`

func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"creating-oda-ticket/SKILL.md": &fstest.MapFile{
			Data: []byte(testSkillContent),
		},
	}
}

func TestDeploy_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()

	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatalf("skill file not found: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected file, got directory")
	}
	if info.Size() == 0 {
		t.Fatal("skill file is empty")
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading skill file: %v", err)
	}
	if !strings.Contains(string(content), "creating-oda-ticket") {
		t.Error("skill file does not contain expected skill name")
	}
}

func TestDeploy_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleContent := []byte("old content")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), staleContent, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == string(staleContent) {
		t.Error("skill file was not overwritten")
	}
}

func TestDeploy_DirectoryAlreadyExists(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("skill file not found: %v", err)
	}
}

func TestDeploy_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	info, err := os.Stat(skillPath)
	if err != nil {
		t.Fatal(err)
	}

	perm := info.Mode().Perm()
	if perm != 0o644 {
		t.Errorf("file permissions = %o, want %o", perm, 0o644)
	}
}

func TestDeploy_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("first Deploy() error: %v", err)
	}
	if err := Deploy(dir, newTestFS()); err != nil {
		t.Fatalf("second Deploy() error: %v", err)
	}

	skillPath := filepath.Join(dir, ".opencode", "skills", "creating-oda-ticket", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "creating-oda-ticket") {
		t.Error("skill file content invalid after double deploy")
	}
}

func TestDeploy_MultipleSkills(t *testing.T) {
	dir := t.TempDir()

	multiFS := fstest.MapFS{
		"skill-one/SKILL.md": &fstest.MapFile{Data: []byte("# Skill One")},
		"skill-two/SKILL.md": &fstest.MapFile{Data: []byte("# Skill Two")},
	}

	if err := Deploy(dir, multiFS); err != nil {
		t.Fatalf("Deploy() error: %v", err)
	}

	for _, name := range []string{"skill-one", "skill-two"} {
		path := filepath.Join(dir, ".opencode", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("skill %s not found: %v", name, err)
		}
	}
}

func TestDeploy_EmptyFS(t *testing.T) {
	dir := t.TempDir()

	emptyFS := fstest.MapFS{}

	if err := Deploy(dir, emptyFS); err != nil {
		t.Fatalf("Deploy() with empty FS error: %v", err)
	}

	// .opencode/skills should not be created for empty FS
	// (WalkDir skips "." and there are no entries)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/skills/ -v -race`
Expected: PASS — all tests green

**Step 5: Commit**

```bash
git add internal/skills/skills.go internal/skills/skills_test.go
git commit -m "feat: add internal/skills package for deploying embedded opencode skills"
```

---

### Task 2: Add `//go:embed` directive in `main.go` and call `skills.Deploy()`

**Files:**
- Modify: `main.go` (add embed import, embed directive, Deploy call)

**Step 1: Write the failing test**

Add to `main_test.go`:

```go
func TestSkillsEmbedFS_ContainsSkillFile(t *testing.T) {
	// Verify the embedded FS contains the expected skill file
	content, err := fs.ReadFile(skillFiles, "creating-oda-ticket/SKILL.md")
	if err != nil {
		t.Fatalf("embedded skill file not found: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("embedded skill file is empty")
	}
	if !strings.Contains(string(content), "creating-oda-ticket") {
		t.Error("embedded skill file does not contain expected content")
	}
}
```

This requires adding `"io/fs"` and `"strings"` to the imports in `main_test.go`.

**Step 2: Run test to verify it fails**

Run: `go test -run TestSkillsEmbedFS -v`
Expected: FAIL — `skillFiles` not defined

**Step 3: Write the implementation**

Modify `main.go`:

1. Add `"embed"` and `"io/fs"` to imports (embed must be imported for the `//go:embed` directive to work — use blank import `_ "embed"` is not needed since we reference `embed.FS`).

2. Add the embed directive before the `main()` function:

```go
//go:embed skills/*
var skillFiles embed.FS
```

3. In `runServe()`, add the skills deployment call **before** the preflight checks (before line `fmt.Println("Running preflight checks...")`). We need to sub-FS to strip the `skills/` prefix:

```go
	// Deploy embedded opencode skills before starting opencode
	fmt.Println("Deploying opencode skills...")
	skillsSub, err := fs.Sub(skillFiles, "skills")
	if err != nil {
		return fmt.Errorf("accessing embedded skills: %w", err)
	}
	if err := skills.Deploy(dir, skillsSub); err != nil {
		return fmt.Errorf("deploying opencode skills: %w", err)
	}
	fmt.Println("  ✓ skills deployed to .opencode/skills/")
```

4. Add `"github.com/crazy-goat/one-dev-army/internal/skills"` to imports.

The full diff for `main.go`:

- Add to imports: `"embed"`, `"io/fs"`, `"github.com/crazy-goat/one-dev-army/internal/skills"`
- Add after imports, before `func main()`:
  ```go
  //go:embed skills/*
  var skillFiles embed.FS
  ```
- Add at the beginning of `runServe()`, before `const opencodeURL`:
  ```go
  // Deploy embedded opencode skills before starting opencode
  fmt.Println("Deploying opencode skills...")
  skillsSub, err := fs.Sub(skillFiles, "skills")
  if err != nil {
      return fmt.Errorf("accessing embedded skills: %w", err)
  }
  if err := skills.Deploy(dir, skillsSub); err != nil {
      return fmt.Errorf("deploying opencode skills: %w", err)
  }
  fmt.Println("  ✓ skills deployed to .opencode/skills/")
  ```

**Step 4: Run tests to verify they pass**

Run: `go test -run TestSkillsEmbedFS -v`
Expected: PASS

Run: `go test ./... -race`
Expected: PASS — all tests green

**Step 5: Verify build**

Run: `go build -o oda .`
Expected: Binary builds successfully with embedded skills

**Step 6: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: embed skills/ and deploy to .opencode/skills/ on startup"
```

---

### Task 3: Update documentation

**Files:**
- Modify: `docs/structure.md` (add `skills/` to top-level layout and `internal/skills/` package description)

**Step 1: Add `skills/` to the top-level layout section**

In the top-level layout code block, add:

```
skills/                     # Opencode skill files (embedded into binary at build time)
  creating-oda-ticket/      # Skill for creating ODA tickets via CLI
```

**Step 2: Add `internal/skills/` package description**

After the `### `setup/`` section, add:

```markdown
### `skills/` — Embedded Opencode Skills

**Files:** `skills.go`

- **`skills.go`** — `Deploy(dir string, skillFS fs.FS) error` copies embedded skill files to `<dir>/.opencode/skills/`. Called during startup before opencode server starts. Uses `fs.WalkDir` to iterate the embedded filesystem and writes each file with `0644` permissions. Creates directories as needed. Idempotent — overwrites existing files on every run to ensure skills stay up-to-date with the binary version.
```

**Step 3: Commit**

```bash
git add docs/structure.md
git commit -m "docs: add skills package to repository structure documentation"
```

---

### Task 4: Run full verification

**Step 1: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors

**Step 2: Run all tests**

Run: `go test -race ./...`
Expected: All tests pass

**Step 3: Build binary**

Run: `go build -o oda .`
Expected: Builds successfully

**Step 4: Verify embedded content**

Run: `./oda --help`
Expected: Shows usage without errors (skills are embedded but not deployed since we didn't run serve)

**Step 5: Commit (if any fixes needed)**

Only if linter or tests required changes.

---

## Summary of Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/skills/skills.go` | Create | `Deploy()` function — walks embedded FS, writes to `.opencode/skills/` |
| `internal/skills/skills_test.go` | Create | 7 tests covering create, overwrite, idempotent, permissions, multi-skill, empty FS |
| `main.go` | Modify | Add `//go:embed skills/*`, call `skills.Deploy()` before opencode starts |
| `main_test.go` | Modify | Add test verifying embedded FS contains skill file |
| `docs/structure.md` | Modify | Document `skills/` directory and `internal/skills/` package |

## Key Design Decisions

1. **`fs.FS` interface instead of `embed.FS` concrete type** — allows testing with `fstest.MapFS` without needing real embedded files in tests.

2. **Embed in `main.go`, sub-FS to strip prefix** — Go's `//go:embed` can only reference files in or below the package directory. Since `skills/` is at the repo root and `internal/skills/` is nested, we embed in `main.go` and use `fs.Sub()` to strip the `skills/` prefix before passing to `Deploy()`.

3. **Always overwrite** — skills are deployed on every startup to ensure they match the binary version. No version checking or caching — simplicity over optimization for a handful of small files.

4. **Deploy before opencode starts** — skills must be on disk before opencode reads them. The deploy call is placed before preflight checks in `runServe()`.
