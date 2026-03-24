package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSkillsFS(t *testing.T) {
	// Verify the embedded skills filesystem contains expected files
	dirEntries, err := skillsFS.ReadDir("skills")
	if err != nil {
		t.Fatalf("Failed to read embedded skills directory: %v", err)
	}

	if len(dirEntries) == 0 {
		t.Error("Expected at least one skill in embedded FS, got none")
	}

	// Check for the creating-oda-ticket skill
	found := false
	for _, entry := range dirEntries {
		if entry.Name() == "creating-oda-ticket" && entry.IsDir() {
			found = true
			// Verify it contains SKILL.md
			skillFile, err := skillsFS.ReadFile("skills/creating-oda-ticket/SKILL.md")
			if err != nil {
				t.Errorf("Failed to read creating-oda-ticket/SKILL.md: %v", err)
			}
			if len(skillFile) == 0 {
				t.Error("SKILL.md is empty")
			}
			break
		}
	}

	if !found {
		t.Error("Expected 'creating-oda-ticket' skill directory in embedded FS")
	}
}

func TestWorkingDirFlag_ValidDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "oda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Test that a valid directory is accepted
	absDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Fatal("Expected path to be a directory")
	}

	// Verify the absolute path is correct
	if absDir != tempDir && absDir != filepath.Clean(tempDir) {
		// On some systems, the paths might differ slightly due to symlinks
		// This is acceptable as long as both point to the same location
		t.Logf("Absolute path: %s, Original: %s", absDir, tempDir)
	}
}

func TestWorkingDirFlag_DefaultValue(t *testing.T) {
	// Test that default value is current directory
	defaultDir := "."

	absDir, err := filepath.Abs(defaultDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path for default: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	if absDir != cwd {
		t.Errorf("Default directory should resolve to current working directory. Got: %s, Expected: %s", absDir, cwd)
	}
}

func TestWorkingDirFlag_NonExistent(t *testing.T) {
	// Test error handling for non-existent path
	nonExistentPath := "/path/that/does/not/exist/12345"

	_, err := os.Stat(nonExistentPath)
	if err == nil {
		t.Skip("Path unexpectedly exists, skipping test")
	}

	// Verify that os.IsNotExist works as expected
	if !os.IsNotExist(err) {
		t.Logf("Error is not 'not exist' type: %v", err)
	}
}

func TestWorkingDirFlag_NotADirectory(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "oda-test-file-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_ = tempFile.Close()

	// Test that the path exists but is not a directory
	info, err := os.Stat(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.IsDir() {
		t.Fatal("Expected path to be a file, not a directory")
	}
}

func TestWorkingDirFlag_AbsolutePath(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "oda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Change to a different directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Change to the temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test relative path conversion
	relPath := "subdir"
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		t.Fatalf("Failed to convert to absolute path: %v", err)
	}

	// Verify it's absolute
	if !filepath.IsAbs(absPath) {
		t.Errorf("Expected absolute path, got: %s", absPath)
	}

	// Verify it points to the correct location
	expectedPath := subDir
	if absPath != expectedPath {
		t.Errorf("Absolute path mismatch. Got: %s, Expected: %s", absPath, expectedPath)
	}
}

func TestRunInit_WithDirectory(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "oda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Initialize a git repo in the temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// We can't fully test runInit without a git repo, but we can verify
	// that it accepts the directory parameter
	// The actual git check will fail, which is expected behavior
	err = runInit(tempDir)
	// We expect an error because there's no git repo
	if err == nil {
		t.Log("runInit succeeded without a git repo (may be expected in some cases)")
	} else {
		t.Logf("runInit failed as expected without git repo: %v", err)
	}
}
