package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CommitInfo represents information about a commit
type CommitInfo struct {
	Hash      string
	Author    string
	Timestamp time.Time
}

// Status represents the git status of a repository
type Status struct {
	IsDirty      bool
	HasUntracked bool
}

// RemoteStatus represents remote tracking information
type RemoteStatus struct {
	Ahead  int
	Behind int
}

// RepoInfo combines branch, commit, and remote info (fetched in one call)
type RepoInfo struct {
	Branch    string
	Commit    *CommitInfo
	RemoteURL string
	Host      string
}

// GetRepoInfo gets branch, commit, and remote info in a single efficient call
// This is much faster than calling GetBranch, GetLastCommit, and GetRemoteURL separately
func GetRepoInfo(repoPath string) (*RepoInfo, error) {
	// Use a single git command with format to get branch, commit, and remote
	// Format: branch|hash|author|timestamp|remoteurl
	cmd := exec.Command("bash", "-c",
		`git rev-parse --abbrev-ref HEAD && git log -1 --format=%H\|%an\|%ct 2>/dev/null && git remote get-url origin 2>/dev/null || true`)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get repo info: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("unexpected git output")
	}

	info := &RepoInfo{}

	// Line 1: branch name
	info.Branch = strings.TrimSpace(lines[0])

	// Line 2: commit info (hash|author|timestamp)
	if len(lines) > 1 && lines[1] != "" {
		parts := strings.Split(lines[1], "|")
		if len(parts) == 3 {
			var timestamp int64
			fmt.Sscanf(parts[2], "%d", &timestamp)
			info.Commit = &CommitInfo{
				Hash:      parts[0],
				Author:    parts[1],
				Timestamp: time.Unix(timestamp, 0),
			}
		}
	}

	// Line 3: remote URL
	if len(lines) > 2 && lines[2] != "" {
		info.RemoteURL = strings.TrimSpace(lines[2])
		info.Host = ExtractHost(info.RemoteURL)
	}

	return info, nil
}

// GetBranch returns the current branch name
func GetBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// GetLastCommit returns information about the last commit
func GetLastCommit(repoPath string) (*CommitInfo, error) {
	// Format: hash|author|timestamp
	cmd := exec.Command("git", "log", "-1", "--format=%H|%an|%ct")
	cmd.Dir = repoPath

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

	// Parse timestamp
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

// GetStatus returns the working tree status
func GetStatus(repoPath string) (*Status, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath

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

		// Check if file is untracked (starts with ??)
		if strings.HasPrefix(line, "??") {
			status.HasUntracked = true
		} else {
			// Any other status means dirty
			status.IsDirty = true
		}
	}

	return status, nil
}

// GetRemoteURL returns the remote URL for the origin remote
func GetRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// No remote is not an error, just return empty string
		return "", nil
	}

	url := strings.TrimSpace(string(output))
	return url, nil
}

// GetRemoteStatus returns ahead/behind counts compared to upstream
func GetRemoteStatus(repoPath string) (*RemoteStatus, error) {
	// First, fetch to get latest remote state
	fetchCmd := exec.Command("git", "fetch", "--quiet")
	fetchCmd.Dir = repoPath
	if err := fetchCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}

	// Get ahead/behind counts
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", "HEAD...@{u}")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// No upstream is not an error, just return zeros
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

// ExtractHost extracts the host from a git URL
func ExtractHost(url string) string {
	if url == "" {
		return ""
	}

	// Handle SSH URLs (git@github.com:user/repo.git)
	if strings.HasPrefix(url, "git@") {
		parts := strings.Split(url, "@")
		if len(parts) > 1 {
			hostParts := strings.Split(parts[1], ":")
			if len(hostParts) > 0 {
				return hostParts[0]
			}
		}
	}

	// Handle HTTPS URLs
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "https://")
		url = strings.TrimPrefix(url, "http://")
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return ""
}

// IsGitRepo checks if a directory is a git repository
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}
