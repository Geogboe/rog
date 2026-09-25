package wsl

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// IsAvailable checks if WSL is available (Windows only)
func IsAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl", "--list", "--quiet")
	return cmd.Run() == nil
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
	output, err := exec.CommandContext(ctx, "wsl", "--exec", "printenv", "WSL_DISTRO_NAME").Output()
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

	return exec.CommandContext(ctx, "wsl", wslArgs...)
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
