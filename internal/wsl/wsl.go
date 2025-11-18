package wsl

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// IsAvailable checks if WSL is available (Windows only)
func IsAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	cmd := exec.Command("wsl", "--list", "--quiet")
	return cmd.Run() == nil
}

// DistroExists checks if a specific WSL distro exists
func DistroExists(distro string) bool {
	if !IsAvailable() {
		return false
	}

	cmd := exec.Command("wsl", "--list", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	distros := strings.Split(string(output), "\n")
	for _, d := range distros {
		d = strings.TrimSpace(d)
		// Remove null bytes and BOM that Windows might add
		d = strings.Trim(d, "\x00\ufeff")
		if strings.EqualFold(d, distro) {
			return true
		}
	}

	return false
}

// GetDefaultDistro returns the default WSL distro
func GetDefaultDistro() (string, error) {
	if !IsAvailable() {
		return "", fmt.Errorf("WSL not available")
	}

	cmd := exec.Command("wsl", "--list", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get WSL distros: %w", err)
	}

	distros := strings.Split(string(output), "\n")
	if len(distros) == 0 {
		return "", fmt.Errorf("no WSL distros found")
	}

	// First distro is typically the default
	distro := strings.TrimSpace(distros[0])
	distro = strings.Trim(distro, "\x00\ufeff")

	if distro == "" && len(distros) > 1 {
		distro = strings.TrimSpace(distros[1])
		distro = strings.Trim(distro, "\x00\ufeff")
	}

	return distro, nil
}

// ExecInDistro executes a command in a specific WSL distro
func ExecInDistro(distro string, command string, args ...string) *exec.Cmd {
	// Build WSL command: wsl -d <distro> -- <command> <args...>
	wslArgs := []string{"-d", distro, "--", command}
	wslArgs = append(wslArgs, args...)

	return exec.Command("wsl", wslArgs...)
}

// TranslatePathToWindows converts a WSL path to Windows UNC path
// Example: /home/user/project -> \\wsl.localhost\Ubuntu\home\user\project
func TranslatePathToWindows(distro, wslPath string) string {
	// Remove leading slash
	wslPath = strings.TrimPrefix(wslPath, "/")

	// Use \\wsl.localhost\ or \\wsl$\ depending on Windows version
	// \\wsl.localhost\ is newer and more reliable
	return fmt.Sprintf("\\\\wsl.localhost\\%s\\%s", distro, wslPath)
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
	cmd := ExecInDistro(distro, "test", "-d", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("path '%s' does not exist in WSL distro '%s'", path, distro)
	}

	return nil
}
