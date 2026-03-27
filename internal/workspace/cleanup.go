package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FindOldTempFiles returns files in .oda/tmp/ older than given time
func FindOldTempFiles(rootDir string, olderThan time.Time) ([]string, error) {
	tmpDir := filepath.Join(rootDir, ".oda", "tmp")

	// Check if directory exists
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	var oldFiles []string

	err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Skip files we can't access - continue walking
			return nil //nolint:nilerr // intentional - skip inaccessible files
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is older than the cutoff time
		if info.ModTime().Before(olderThan) {
			oldFiles = append(oldFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking tmp directory: %w", err)
	}

	return oldFiles, nil
}

// RemoveTempFiles deletes the specified temp files
func RemoveTempFiles(files []string, dryRun bool) error {
	if dryRun {
		return nil
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("removing file %s: %w", file, err)
		}
	}

	return nil
}
