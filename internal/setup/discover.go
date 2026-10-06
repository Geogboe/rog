package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Geogboe/rog/internal/scanner/discovery"
)

type SearchRoot struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	WSL     bool   `json:"wsl,omitempty"`
	Distro  string `json:"wsl_distro,omitempty"`
	Windows bool   `json:"windows,omitempty"`
}

type Candidate struct {
	Path         string   `json:"path"`
	Root         string   `json:"root"`
	Relative     string   `json:"relative"`
	Depth        int      `json:"depth"`
	Email        string   `json:"email,omitempty"`
	AuthorEmails []string `json:"author_emails,omitempty"`
	Valid        bool     `json:"valid"`
	Reason       string   `json:"reason,omitempty"`
	WSL          bool     `json:"wsl,omitempty"`
	Distro       string   `json:"wsl_distro,omitempty"`
	Windows      bool     `json:"windows,omitempty"`
}

type DiscoveryResult struct {
	Candidates          []Candidate `json:"candidates"`
	Rejected            int         `json:"rejected"`
	Overlaps            int         `json:"overlaps"`
	ExcludedDirectories int         `json:"excluded_directories"`
	Warnings            []string    `json:"warnings,omitempty"`
}

// Discover locates .git marker parents using filesystem traversal only.
// Locations are pending Git validation by the subsequent full scan.
func Discover(ctx context.Context, roots []SearchRoot, excludes []string, progress func(root, name string, done, total int)) DiscoveryResult {
	var result DiscoveryResult
	type found struct {
		root SearchRoot
		path string
	}
	var foundPaths []found
	var pathsMu sync.Mutex
	var excluded atomic.Int64
	var visitedDirs atomic.Int64
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			return result
		}
		info, err := os.Stat(root.Path)
		if err != nil || !info.IsDir() {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: location unavailable", root.Name))
			continue
		}
		err = discovery.WalkWithProgress(ctx, root.Path, 64, func(name, full string) bool {
			rel, _ := filepath.Rel(root.Path, full)
			skip := excludedName(name, excludes) || excludedCandidatePath(rel, excludes, runtime.GOOS == "windows") || (filepath.Clean(root.Path) == string(filepath.Separator) && excludedSystemPath(full))
			if skip {
				excluded.Add(1)
			}
			return skip
		}, func(dir string, _ int) {
			if progress != nil {
				count := visitedDirs.Add(1)
				progress(root.Name, fmt.Sprintf("Walking %s · %d directories", filepath.Base(dir), count), -1, 0)
			}
		}, func(path string) error {
			pathsMu.Lock()
			foundPaths = append(foundPaths, found{root, path})
			pathsMu.Unlock()
			return nil
		})
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", root.Name, safeWalkError(err)))
		}
	}
	result.ExcludedDirectories = int(excluded.Load())
	// A marker seen through overlapping roots is recorded once, assigned to the
	// narrowest root, and counted as an overlap for the coverage preview.
	byPath := map[string]found{}
	for _, f := range foundPaths {
		key := canonicalPath(f.path)
		if old, ok := byPath[key]; ok {
			result.Overlaps++
			if len(f.root.Path) > len(old.root.Path) {
				byPath[key] = f
			}
		} else {
			byPath[key] = f
		}
	}
	foundPaths = foundPaths[:0]
	for _, f := range byPath {
		foundPaths = append(foundPaths, f)
	}
	sort.Slice(foundPaths, func(i, j int) bool { return foundPaths[i].path < foundPaths[j].path })
	for _, f := range foundPaths {
		if ctx.Err() != nil {
			break
		}
		c := Candidate{Path: filepath.Clean(f.path), Root: f.root.Name, WSL: f.root.WSL, Distro: f.root.Distro, Windows: f.root.Windows}
		if rel, err := filepath.Rel(f.root.Path, c.Path); err == nil && rel != "." {
			c.Relative = rel
			c.Depth = len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
		}
		result.Candidates = append(result.Candidates, c)
	}
	if ctx.Err() != nil {
		result.Warnings = append(result.Warnings, "discovery cancelled; coverage is partial")
	}
	return result
}

func canonicalPath(value string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(value))
	}
	return filepath.Clean(value)
}

func excludedName(name string, extras []string) bool {
	defaults := RecommendedExcludes()
	defaults = append(defaults, "AppData", "Windows", "Program Files", "Program Files (x86)", "$Recycle.Bin", "System Volume Information")
	for _, item := range append(defaults, extras...) {
		if strings.EqualFold(name, item) {
			return true
		}
	}
	return false
}

// RecommendedExcludes returns directories setup skips during whole-filesystem
// discovery. The wizard displays these as editable defaults and saves the
// selected values to global_excludes.
func RecommendedExcludes() []string {
	return []string{".git", "node_modules", "vendor", "target", "build", "dist", "__pycache__", ".venv", "venv", ".cache", ".npm", ".cargo", ".gradle", ".m2", "site-packages", "dist-packages", "**/go/pkg/mod"}
}

func excludedSystemPath(path string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	clean := filepath.Clean(path)
	for _, prefix := range []string{"/proc", "/sys", "/dev", "/run", "/snap", "/var/lib", "/var/cache", "/mnt", "/media", "/tmp", "/lost+found"} {
		if clean == prefix || strings.HasPrefix(clean, prefix+string(filepath.Separator)) {
			return true
		}
	}
	for _, mount := range nonLocalMountPoints() {
		if clean == mount || strings.HasPrefix(clean, mount+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

var mountsOnce sync.Once
var remoteMounts []string

func nonLocalMountPoints() []string {
	mountsOnce.Do(func() {
		data, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			separator := -1
			for i, v := range fields {
				if v == "-" {
					separator = i
					break
				}
			}
			if separator < 6 || separator+1 >= len(fields) {
				continue
			}
			mount := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\134", "\\").Replace(fields[4])
			fsType := fields[separator+1]
			switch fsType {
			case "nfs", "nfs4", "cifs", "smb3", "9p", "drvfs", "sshfs", "fuse.sshfs", "fuse.rclone", "davfs", "fuse.davfs":
				remoteMounts = append(remoteMounts, filepath.Clean(mount))
			}
		}
	})
	return remoteMounts
}

func safeWalkError(err error) string {
	text := err.Error()
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}
