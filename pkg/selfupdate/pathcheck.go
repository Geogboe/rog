package selfupdate

// IsInPath reports whether dir is present in the process's PATH environment variable.
// Platform-specific implementations are in pathcheck_unix.go and pathcheck_windows.go.

// PathWarningMessage returns a human-readable warning and instructions for adding dir
// to the user's PATH permanently. Returns an empty string if dir is already in PATH.
// Platform-specific implementations handle shell detection and OS-specific instructions.
