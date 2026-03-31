package selfupdate

// atomicReplace atomically replaces destPath with srcPath.
// Platform-specific implementations are in installer_unix.go and installer_windows.go.
// The caller is responsible for ensuring srcPath is a valid executable.
