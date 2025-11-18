package git

import (
	"fmt"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/wsl"
)

// GetBranchWSL returns the current branch name for a WSL repository
func GetBranchWSL(distro, repoPath string) (string, error) {
	cmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// GetLastCommitWSL returns information about the last commit for a WSL repository
func GetLastCommitWSL(distro, repoPath string) (*CommitInfo, error) {
	cmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "log", "-1", "--format=%H|%an|%ct")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get last commit: %w", err)
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return nil, fmt.Errorf("no commits found")
	}

	parts := strings.Split(line, "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected git log output format")
	}

	var timestamp int64
	if _, err := fmt.Sscanf(parts[2], "%d", &timestamp); err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return &CommitInfo{
		Hash:      parts[0],
		Author:    parts[1],
		Timestamp: time.Unix(timestamp, 0),
	}, nil
}

// GetStatusWSL returns the working tree status for a WSL repository
func GetStatusWSL(distro, repoPath string) (*Status, error) {
	cmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "status", "--porcelain")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	status := &Status{}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "??") {
			status.HasUntracked = true
		} else {
			status.IsDirty = true
		}
	}

	return status, nil
}

// GetRemoteURLWSL returns the remote URL for a WSL repository
func GetRemoteURLWSL(distro, repoPath string) (string, error) {
	cmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "remote", "get-url", "origin")

	output, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	url := strings.TrimSpace(string(output))
	return url, nil
}

// GetRemoteStatusWSL returns ahead/behind counts for a WSL repository
func GetRemoteStatusWSL(distro, repoPath string) (*RemoteStatus, error) {
	// First, fetch
	fetchCmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "fetch", "--quiet")
	if err := fetchCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	// Get ahead/behind counts
	cmd := wsl.ExecInDistro(distro, "git", "-C", repoPath, "rev-list", "--left-right", "--count", "HEAD...@{u}")

	output, err := cmd.Output()
	if err != nil {
		return &RemoteStatus{}, nil
	}

	line := strings.TrimSpace(string(output))
	parts := strings.Fields(line)
	if len(parts) != 2 {
		return &RemoteStatus{}, nil
	}

	var ahead, behind int
	if _, err := fmt.Sscanf(parts[0], "%d", &ahead); err != nil {
		return nil, fmt.Errorf("failed to parse ahead count: %w", err)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &behind); err != nil {
		return nil, fmt.Errorf("failed to parse behind count: %w", err)
	}

	return &RemoteStatus{
		Ahead:  ahead,
		Behind: behind,
	}, nil
}

// IsGitRepoWSL checks if a directory in WSL is a git repository
func IsGitRepoWSL(distro, path string) bool {
	cmd := wsl.ExecInDistro(distro, "git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}
