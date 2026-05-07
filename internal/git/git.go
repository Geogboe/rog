package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// GetRepoInfo gets branch, commit, and remote info using direct filesystem reads
// for maximum speed. Falls back to git subprocesses if filesystem reads fail.
func GetRepoInfo(repoPath string) (*RepoInfo, error) {
	info := &RepoInfo{}

	// Fast path: read everything directly from the .git directory
	if gitDir, err := resolveGitDir(repoPath); err == nil {
		if branch, err := readBranchFromGitDir(gitDir); err == nil {
			info.Branch = branch
		}
		if info.Branch != "" {
			if commit, err := readLastCommitFromGitDir(gitDir, info.Branch); err == nil {
				info.Commit = commit
			}
		}
		if url := readRemoteURLFromGitDir(gitDir); url != "" {
			info.RemoteURL = url
			info.Host = ExtractHost(url)
		}
	}

	// Subprocess fallback for branch (required field)
	if info.Branch == "" {
		branch, err := GetBranch(repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get repo info: %w", err)
		}
		info.Branch = branch
	}

	// Subprocess fallback for last commit (optional)
	if info.Commit == nil {
		if commit, err := GetLastCommit(repoPath); err == nil {
			info.Commit = commit
		}
	}

	// Subprocess fallback for remote URL (optional)
	if info.RemoteURL == "" {
		if url, err := GetRemoteURL(repoPath); err == nil {
			info.RemoteURL = url
			info.Host = ExtractHost(url)
		}
	}

	return info, nil
}

// resolveGitDir resolves the actual git directory for a repo path.
// Handles regular repos (.git is a directory) and worktrees/submodules
// (.git is a file containing "gitdir: /path/to/actual/gitdir").
func resolveGitDir(repoPath string) (string, error) {
	gitPath := filepath.Join(repoPath, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", fmt.Errorf("no .git found: %w", err)
	}
	if info.IsDir() {
		return gitPath, nil
	}
	// .git is a file (worktree or submodule checkout)
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		return "", fmt.Errorf("unexpected .git file content")
	}
	gitDir := strings.TrimPrefix(content, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoPath, gitDir)
	}
	return filepath.Clean(gitDir), nil
}

// readBranchFromGitDir reads the current branch from HEAD without spawning a subprocess.
func readBranchFromGitDir(gitDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(data))
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/"), nil
	}
	// Detached HEAD: content is the raw commit hash
	return "HEAD", nil
}

// readRemoteURLFromGitDir reads the remote origin URL from the git config
// without spawning a subprocess.
func readRemoteURLFromGitDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	inRemoteOrigin := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == `[remote "origin"]` {
			inRemoteOrigin = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inRemoteOrigin = false
			continue
		}
		if inRemoteOrigin && strings.HasPrefix(trimmed, "url") {
			if parts := strings.SplitN(trimmed, "=", 2); len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// readLastCommitFromGitDir reads the last commit from the reflog without
// spawning a subprocess. It tries the branch-specific reflog first, then
// falls back to the HEAD reflog.
func readLastCommitFromGitDir(gitDir, branch string) (*CommitInfo, error) {
	if branch != "" && branch != "HEAD" {
		reflogPath := filepath.Join(gitDir, "logs", "refs", "heads", branch)
		if line, err := readLastLine(reflogPath); err == nil {
			return parseReflogLine(line)
		}
	}
	if line, err := readLastLine(filepath.Join(gitDir, "logs", "HEAD")); err == nil {
		return parseReflogLine(line)
	}
	return nil, fmt.Errorf("no reflog available")
}

// readLastLine reads the last non-empty line from a file by seeking near the end.
func readLastLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()
	if size == 0 {
		return "", fmt.Errorf("empty file")
	}

	// A typical reflog line is ~150-300 bytes; 1024 accommodates long author
	// names and branch names. If the last line somehow exceeds this, parsing
	// will fail and the caller falls back to the subprocess.
	const chunkSize = int64(1024)
	readSize := chunkSize
	if size < readSize {
		readSize = size
	}

	buf := make([]byte, readSize)
	if _, err = f.ReadAt(buf, size-readSize); err != nil {
		return "", err
	}

	content := string(buf)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("no content found in reflog")
}

// parseReflogLine parses a git reflog entry.
// Format: <oldhash> <newhash> Author Name <email> <timestamp> <tz>\t<action>
func parseReflogLine(line string) (*CommitInfo, error) {
	// Discard the action part after the tab
	metadata := strings.SplitN(line, "\t", 2)[0]
	fields := strings.Fields(metadata)
	// Minimum: oldhash newhash author_token <email> timestamp tz (6 fields).
	// Author can span multiple tokens before the <email> field.
	if len(fields) < 6 {
		return nil, fmt.Errorf("invalid reflog line: too few fields")
	}

	hash := fields[1] // newhash = current commit hash

	// Locate the email token (wrapped in angle brackets)
	emailIdx := -1
	for i := 2; i < len(fields); i++ {
		if strings.HasPrefix(fields[i], "<") {
			emailIdx = i
			break
		}
	}
	// emailIdx must be at least 3: fields[0]=oldhash, fields[1]=newhash,
	// fields[2]=first author token, fields[3+]=email or more author tokens.
	if emailIdx < 3 {
		return nil, fmt.Errorf("invalid reflog line: no email found")
	}

	author := strings.Join(fields[2:emailIdx], " ")

	if emailIdx+1 >= len(fields) {
		return nil, fmt.Errorf("invalid reflog line: no timestamp")
	}

	var ts int64
	if _, err := fmt.Sscanf(fields[emailIdx+1], "%d", &ts); err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	return &CommitInfo{
		Hash:      hash,
		Author:    author,
		Timestamp: time.Unix(ts, 0),
	}, nil
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
