package wsl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// IsAvailable checks if WSL is available (Windows only)
func IsAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := hiddenCommandContext(ctx, "--list", "--quiet")
	return cmd.Run() == nil
}

// ListDistros returns registered distributions without starting them.
func ListDistros() ([]string, error) {
	if !IsAvailable() {
		return nil, fmt.Errorf("WSL is not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := hiddenCommandContext(ctx, "--list", "--quiet").Output()
	if err != nil {
		return nil, fmt.Errorf("list WSL distributions: %w", err)
	}
	if len(out)%2 != 0 {
		out = out[:len(out)-1]
	}
	words := make([]uint16, 0, len(out)/2)
	for i := 0; i+1 < len(out); i += 2 {
		words = append(words, uint16(out[i])|uint16(out[i+1])<<8)
	}
	text := strings.TrimPrefix(strings.ReplaceAll(string(utf16.Decode(words)), "\x00", ""), "\ufeff")
	var distros []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if line != "" {
			distros = append(distros, line)
		}
	}
	return distros, nil
}

// DistroExists checks if a specific WSL distro exists
func DistroExists(distro string) bool {
	if !IsAvailable() || distro == "" {
		return false
	}

	// wsl --list --quiet uses UTF-16 output on Windows; ask the distro directly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ExecInDistroContext(ctx, distro, "true").Run() == nil
}

// GetDefaultDistro returns the default WSL distro
func GetDefaultDistro() (string, error) {
	if !IsAvailable() {
		return "", fmt.Errorf("WSL not available")
	}

	// Query inside the default distro to avoid decoding wsl --list's UTF-16 output.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := hiddenCommandContext(ctx, "--exec", "printenv", "WSL_DISTRO_NAME").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default WSL distro: %w", err)
	}
	distro := strings.TrimSpace(string(output))
	if distro == "" {
		return "", fmt.Errorf("default WSL distro name is empty")
	}
	return distro, nil
}

// ExecInDistro executes a command in a specific WSL distro
func ExecInDistro(distro string, command string, args ...string) *exec.Cmd {
	return ExecInDistroContext(context.Background(), distro, command, args...)
}

// ExecInDistroContext executes a command in a specific WSL distro with cancellation.
func ExecInDistroContext(ctx context.Context, distro string, command string, args ...string) *exec.Cmd {
	// --exec passes arguments directly without a second shell expansion.
	wslArgs := []string{"-d", distro, "--exec", command}
	wslArgs = append(wslArgs, args...)

	return hiddenCommandContext(ctx, wslArgs...)
}

// TranslatePathToWindows converts a WSL path to the Windows UNC share.
// Example: /home/user/project -> \\wsl$\Ubuntu\home\user\project
func TranslatePathToWindows(distro, wslPath string) string {
	wslPath = strings.TrimPrefix(wslPath, "/")
	return fmt.Sprintf(`\\wsl$\%s\%s`, distro, strings.ReplaceAll(wslPath, "/", `\`))
}

// ValidateRoot validates a WSL root configuration
func ValidateRoot(distro, path string) error {
	if !IsAvailable() {
		return fmt.Errorf("WSL is not available on this system")
	}

	if distro == "" {
		return fmt.Errorf("WSL distro not specified")
	}

	if !DistroExists(distro) {
		return fmt.Errorf("WSL distro '%s' not found", distro)
	}

	// Test if path exists in WSL
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := ExecInDistroContext(ctx, distro, "test", "-d", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("path '%s' does not exist in WSL distro '%s'", path, distro)
	}

	return nil
}

// executable resolves the inbox WSL launcher even when an embedded shell has
// a reduced PATH. SystemRoot is supplied by Windows itself.
func executable() string {
	if runtime.GOOS == "windows" {
		if root := os.Getenv("SystemRoot"); root != "" {
			candidate := filepath.Join(root, "System32", "wsl.exe")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return "wsl.exe"
}
