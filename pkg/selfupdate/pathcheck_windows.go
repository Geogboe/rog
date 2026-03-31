//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsInPath reports whether dir is in the semicolon-separated PATH.
// On Windows, path comparison is case-insensitive.
func IsInPath(dir string) bool {
	dir = filepath.Clean(strings.ToLower(dir))
	for _, entry := range strings.Split(os.Getenv("PATH"), ";") {
		if filepath.Clean(strings.ToLower(entry)) == dir {
			return true
		}
	}
	return false
}

// PathWarningMessage returns instructions for adding dir to PATH on Windows.
// Returns an empty string if dir is already in PATH.
func PathWarningMessage(dir string) string {
	if IsInPath(dir) {
		return ""
	}

	return fmt.Sprintf(
		"warning: %s is not in your PATH\n\n"+
			"  Add it permanently by running (then open a new terminal):\n\n"+
			"    [Environment]::SetEnvironmentVariable('Path', \"%s;\" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')\n",
		dir, dir,
	)
}
