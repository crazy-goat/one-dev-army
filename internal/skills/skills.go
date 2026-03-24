// Package skills provides deployment of embedded opencode skills to the working directory.
// Skills are embedded at build time and deployed to .opencode/skills/ on startup.
package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Deploy walks the provided fs.FS and writes all files to <dir>/.opencode/skills/.
// It is idempotent - always overwrites existing files.
// The skillFS should be the embedded skills directory (e.g., skills/ containing skill subdirectories).
func Deploy(dir string, skillFS fs.FS) error {
	targetDir := filepath.Join(dir, ".opencode", "skills")

	// Walk the embedded filesystem
	return fs.WalkDir(skillFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking skillFS at %s: %w", path, err)
		}

		// Skip the root directory itself
		if path == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, path)

		if d.IsDir() {
			// Create directory with appropriate permissions
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", targetPath, err)
			}
			return nil
		}

		// It's a file - copy it
		srcFile, err := skillFS.Open(path)
		if err != nil {
			return fmt.Errorf("opening embedded file %s: %w", path, err)
		}
		defer func() { _ = srcFile.Close() }()

		// Ensure parent directory exists
		parentDir := filepath.Dir(targetPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("creating parent directory %s: %w", parentDir, err)
		}

		// Create/overwrite the target file
		dstFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", targetPath, err)
		}
		defer func() { _ = dstFile.Close() }()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("copying file %s: %w", path, err)
		}

		return nil
	})
}
