package skills

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestDeploy_CreatesFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test filesystem with a skill
	testFS := fstest.MapFS{
		"skill1/SKILL.md": &fstest.MapFile{
			Data: []byte("# Skill 1\n"),
			Mode: 0644,
		},
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Verify file was created
	targetPath := filepath.Join(tempDir, ".opencode", "skills", "skill1", "SKILL.md")
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read deployed file: %v", err)
	}

	if string(content) != "# Skill 1\n" {
		t.Errorf("Content mismatch. Got: %s, Want: # Skill 1\\n", string(content))
	}
}

func TestDeploy_CreatesNestedDirectories(t *testing.T) {
	tempDir := t.TempDir()

	testFS := fstest.MapFS{
		"deep/nested/skill/SKILL.md": &fstest.MapFile{
			Data: []byte("nested content"),
			Mode: 0644,
		},
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	targetPath := filepath.Join(tempDir, ".opencode", "skills", "deep", "nested", "skill", "SKILL.md")
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("Nested file not created: %v", err)
	}
}

func TestDeploy_OverwritesExisting(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-create the target directory with old content
	targetDir := filepath.Join(tempDir, ".opencode", "skills", "skill1")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}

	oldFile := filepath.Join(targetDir, "SKILL.md")
	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("Failed to write old file: %v", err)
	}

	// Deploy new content
	testFS := fstest.MapFS{
		"skill1/SKILL.md": &fstest.MapFile{
			Data: []byte("new content"),
			Mode: 0644,
		},
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Verify content was overwritten
	content, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "new content" {
		t.Errorf("File was not overwritten. Got: %s, Want: new content", string(content))
	}
}

func TestDeploy_Idempotent(t *testing.T) {
	tempDir := t.TempDir()

	testFS := fstest.MapFS{
		"skill1/SKILL.md": &fstest.MapFile{
			Data: []byte("content"),
			Mode: 0644,
		},
	}

	// Deploy twice
	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("First deploy failed: %v", err)
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Second deploy failed: %v", err)
	}

	// Verify file still exists and has correct content
	targetPath := filepath.Join(tempDir, ".opencode", "skills", "skill1", "SKILL.md")
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read file after second deploy: %v", err)
	}

	if string(content) != "content" {
		t.Errorf("Content changed after second deploy. Got: %s", string(content))
	}
}

func TestDeploy_MultipleSkills(t *testing.T) {
	tempDir := t.TempDir()

	testFS := fstest.MapFS{
		"skill1/SKILL.md": &fstest.MapFile{
			Data: []byte("skill 1 content"),
			Mode: 0644,
		},
		"skill2/SKILL.md": &fstest.MapFile{
			Data: []byte("skill 2 content"),
			Mode: 0644,
		},
		"skill2/extra.txt": &fstest.MapFile{
			Data: []byte("extra file"),
			Mode: 0644,
		},
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Verify all files were created
	files := []string{
		".opencode/skills/skill1/SKILL.md",
		".opencode/skills/skill2/SKILL.md",
		".opencode/skills/skill2/extra.txt",
	}

	for _, file := range files {
		path := filepath.Join(tempDir, file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("File not created: %s", file)
		}
	}
}

func TestDeploy_EmptyFS(t *testing.T) {
	tempDir := t.TempDir()

	// Empty filesystem
	testFS := fstest.MapFS{}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy with empty FS should not fail: %v", err)
	}

	// Verify .opencode/skills directory was not created (nothing to deploy)
	targetDir := filepath.Join(tempDir, ".opencode", "skills")
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Error("Target directory should not exist for empty FS")
	}
}

func TestDeploy_FilePermissions(t *testing.T) {
	tempDir := t.TempDir()

	testFS := fstest.MapFS{
		"skill1/SKILL.md": &fstest.MapFile{
			Data: []byte("content"),
			Mode: 0644,
		},
	}

	if err := Deploy(tempDir, testFS); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	// Check file permissions
	targetPath := filepath.Join(tempDir, ".opencode", "skills", "skill1", "SKILL.md")
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// File should be readable by all (0644)
	mode := info.Mode().Perm()
	if mode != 0644 {
		t.Errorf("File permissions incorrect. Got: %o, Want: 0644", mode)
	}
}
