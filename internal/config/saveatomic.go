package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// SaveAtomic atomically writes the Config to a file using the tmp+rename pattern.
// This ensures that if the write fails, the original file is not corrupted.
//
// The implementation:
// 1. Marshals cfg to indented JSON
// 2. Writes to a temporary file (path + ".tmp") with 0o600 permissions
// 3. Renames the temporary file to the target path (atomic on POSIX systems)
// 4. Cleans up the temporary file if rename fails
//
// Returns an error if marshaling, writing, or renaming fails.
func SaveAtomic(path string, cfg *Config) error {
	// Marshal the config to indented JSON
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write to temporary file with restricted permissions
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Rename is atomic on POSIX systems; track success to clean up on failure
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpPath)
		}
	}()

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	renamed = true
	return nil
}
