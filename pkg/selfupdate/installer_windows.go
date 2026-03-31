//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// atomicReplace replaces destPath with srcPath on Windows.
//
// Windows does not allow overwriting a running executable directly.
// The strategy is:
//  1. Rename destPath → destPath+".old"  (renaming a running exe IS allowed on Windows)
//  2. Rename srcPath  → destPath         (move new binary into place)
//  3. Best-effort remove of destPath+".old" (may fail if still locked; that's acceptable)
//
// The .old file is harmless if left behind; it will be cleaned up on the next
// successful update or can be deleted manually.
func atomicReplace(srcPath, destPath string) error {
	oldPath := destPath + ".old"

	// Remove any pre-existing .old file from a previous update.
	_ = os.Remove(oldPath)

	// Step 1: rename running exe out of the way.
	if err := os.Rename(destPath, oldPath); err != nil {
		return fmt.Errorf("rename current executable: %w", err)
	}

	// Step 2: move new binary into place.
	if err := os.Rename(srcPath, destPath); err != nil {
		// Attempt rollback so the user isn't left without a binary.
		_ = os.Rename(oldPath, destPath)
		return fmt.Errorf("move new binary into place: %w", err)
	}

	// Step 3: best-effort cleanup of the old file.
	_ = os.Remove(oldPath)

	return nil
}
