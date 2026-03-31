//go:build !windows

package selfupdate

import "os"

// atomicReplace atomically replaces destPath with srcPath using os.Rename.
// On Unix/macOS, os.Rename is atomic on the same filesystem, and the running
// binary's inode remains valid until the process exits (Unix inode semantics).
func atomicReplace(srcPath, destPath string) error {
	return os.Rename(srcPath, destPath)
}
