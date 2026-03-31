//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsInPath reports whether dir is in the colon-separated PATH.
func IsInPath(dir string) bool {
	dir = filepath.Clean(dir)
	for _, entry := range strings.Split(os.Getenv("PATH"), ":") {
		if filepath.Clean(entry) == dir {
			return true
		}
	}
	return false
}

// PathWarningMessage returns instructions for adding dir to PATH on Unix.
// Returns an empty string if dir is already in PATH.
func PathWarningMessage(dir string) string {
	if IsInPath(dir) {
		return ""
	}

	shell := filepath.Base(os.Getenv("SHELL"))
	var rcLine, rcFile string
	switch shell {
	case "bash":
		rcFile = "~/.bashrc"
		rcLine = fmt.Sprintf(`echo 'export PATH="%s:$PATH"' >> %s && source %s`, dir, rcFile, rcFile)
	case "zsh":
		rcFile = "~/.zshrc"
		rcLine = fmt.Sprintf(`echo 'export PATH="%s:$PATH"' >> %s && source %s`, dir, rcFile, rcFile)
	case "fish":
		rcLine = fmt.Sprintf("fish_add_path %s", dir)
	default:
		rcLine = fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
		return fmt.Sprintf(
			"warning: %s is not in your PATH\n\n"+
				"  Add it to your shell's rc file (e.g. ~/.bashrc or ~/.profile):\n\n"+
				"    %s\n",
			dir, rcLine,
		)
	}

	return fmt.Sprintf(
		"warning: %s is not in your PATH\n\n"+
			"  Add it by running:\n\n"+
			"    %s\n",
		dir, rcLine,
	)
}
