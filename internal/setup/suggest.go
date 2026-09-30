package setup

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Geogboe/rog/internal/config"
)

type RootSuggestion struct {
	Root     config.Root
	Count    int
	MaxDepth int
	Selected bool
	Existing bool
}

func Suggestions(candidates []Candidate, existing []config.Root) []RootSuggestion {
	byKey := map[string]*RootSuggestion{}
	for _, old := range existing {
		windows := rootIsWindows(old.WSL, old.Windows)
		key := suggestionKey(old.Path, rootSource(old.WSL, old.WSLDistro, old.Windows), windows)
		byKey[key] = &RootSuggestion{Root: old, Selected: true, Existing: true}
	}
	for _, c := range candidates {
		windows := candidateIsWindows(c)
		root := suggestedPath(c.Path, windows)
		key := suggestionKey(root, rootSource(c.WSL, c.Distro, c.Windows), windows)
		item := byKey[key]
		if item == nil {
			item = &RootSuggestion{Root: config.Root{Name: basePath(root, windows), Path: root, MaxDepth: 1, WSL: c.WSL, WSLDistro: c.Distro, Windows: c.Windows}}
			byKey[key] = item
		}
		rel, err := relativePath(root, c.Path, windows)
		if err == nil && rel != "." {
			depth := pathDepthFor(rel, windows)
			if depth > item.MaxDepth {
				item.MaxDepth = depth
			}
			if depth > item.Root.MaxDepth {
				item.Root.MaxDepth = depth
			}
		}
		item.Count++
	}
	out := make([]RootSuggestion, 0, len(byKey))
	for _, s := range byKey {
		if s.Root.MaxDepth == 0 {
			s.Root.MaxDepth = 4
		}
		out = append(out, *s)
	}
	// Preserve existing choices, then add the broadest new roots first. A
	// nested suggestion is selected only when it expands coverage beyond the
	// roots already selected; otherwise it is visible but not redundant.
	var selected []config.Root
	for _, suggestion := range out {
		if suggestion.Existing {
			selected = append(selected, suggestion.Root)
		}
	}
	order := make([]int, 0, len(out))
	for i := range out {
		if !out[i].Existing {
			order = append(order, i)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := out[order[i]].Root, out[order[j]].Root
		if len(a.Path) != len(b.Path) {
			return len(a.Path) < len(b.Path)
		}
		return strings.ToLower(a.Path) < strings.ToLower(b.Path)
	})
	for _, i := range order {
		before := CountCoveredCandidates(candidates, selected, nil)
		trial := append(append([]config.Root(nil), selected...), out[i].Root)
		out[i].Selected = CountCoveredCandidates(candidates, trial, nil) > before
		if out[i].Selected {
			selected = trial
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Root.Name != out[j].Root.Name {
			return out[i].Root.Name < out[j].Root.Name
		}
		return strings.ToLower(out[i].Root.Path) < strings.ToLower(out[j].Root.Path)
	})
	used := map[string]int{}
	for i := range out {
		base := out[i].Root.Name
		used[base]++
		if used[base] > 1 {
			out[i].Root.Name = base + "-" + strconv.Itoa(used[base])
		}
	}
	return out
}

// CountCoveredCandidates returns how many validated discoveries would be
// reached by roots at their current depths and exclusions. Each repository is
// counted once even when roots overlap.
func CountCoveredCandidates(candidates []Candidate, roots []config.Root, globalExcludes []string) int {
	covered := 0
	for _, candidate := range candidates {
		for _, root := range roots {
			if !sameSource(candidate, root) {
				continue
			}
			windows := rootIsWindows(root.WSL, root.Windows)
			rel, err := relativePath(root.Path, candidate.Path, windows)
			if err != nil {
				continue
			}
			depth := pathDepthFor(rel, windows)
			if depth > root.MaxDepth || excludedCandidatePath(rel, append(append([]string{}, globalExcludes...), root.Exclude...), windows) {
				continue
			}
			covered++
			break
		}
	}
	return covered
}

// NestedWithin reports the closest selected-root candidate containing root.
// It lets the setup UI make overlapping suggestions visible before apply.
func NestedWithin(root config.Root, suggestions []RootSuggestion) string {
	windows := rootIsWindows(root.WSL, root.Windows)
	closest := ""
	closestLen := -1
	for _, other := range suggestions {
		if other.Root.Path == root.Path || !sameRootSource(root, other.Root) || rootIsWindows(other.Root.WSL, other.Root.Windows) != windows {
			continue
		}
		rel, err := relativePath(other.Root.Path, root.Path, windows)
		if err != nil || rel == "." {
			continue
		}
		if len(other.Root.Path) > closestLen {
			closest = other.Root.Name
			closestLen = len(other.Root.Path)
		}
	}
	return closest
}

func sameSource(candidate Candidate, root config.Root) bool {
	if candidate.WSL {
		return root.WSL && strings.EqualFold(candidate.Distro, root.WSLDistro)
	}
	if candidate.Windows || runtime.GOOS == "windows" && !candidate.WSL {
		return root.Windows && !root.WSL
	}
	return !root.Windows && !root.WSL
}

func sameRootSource(a, b config.Root) bool {
	if a.WSL || b.WSL {
		return a.WSL && b.WSL && strings.EqualFold(a.WSLDistro, b.WSLDistro)
	}
	return a.Windows == b.Windows
}

func excludedCandidatePath(rel string, patterns []string, windows bool) bool {
	if rel == "." || rel == "" {
		return false
	}
	rel = cleanPath(rel, windows)
	sep := "/"
	if windows {
		rel = strings.ReplaceAll(rel, `\`, "/")
	}
	parts := strings.Split(rel, sep)
	for i, part := range parts {
		relPart := strings.Join(parts[:i+1], sep)
		for _, pattern := range patterns {
			pattern = strings.ReplaceAll(pattern, `\`, "/")
			if pattern == part {
				return true
			}
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
			if matched, _ := filepath.Match(pattern, relPart); matched {
				return true
			}
			if strings.HasPrefix(pattern, "**/") {
				suffix := pattern[3:]
				if strings.HasSuffix(relPart, suffix) || part == suffix {
					return true
				}
			}
		}
	}
	return false
}

func suggestedPath(repo string, windows bool) string {
	dir := cleanPath(repo, windows)
	for {
		switch strings.ToLower(basePath(dir, windows)) {
		case "projects", "github", "repos", "repositories", "workspace", "workspaces", "src", "code", "dev", "development":
			return dir
		}
		parent := dirPath(dir, windows)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirPath(repo, windows)
}
func suggestionKey(value, source string, windows bool) string {
	clean := cleanPath(value, windows)
	if windows {
		clean = strings.ToLower(clean)
	}
	return strings.ToLower(source + "|" + clean)
}
func rootSource(wsl bool, distro string, windows bool) string {
	if wsl {
		return "wsl:" + distro
	}
	if windows {
		return "windows"
	}
	return "native"
}
func candidateIsWindows(c Candidate) bool  { return c.Windows || (!c.WSL && runtime.GOOS == "windows") }
func rootIsWindows(wsl, windows bool) bool { return windows || (!wsl && runtime.GOOS == "windows") }
func cleanPath(value string, windows bool) string {
	if windows {
		value = path.Clean(strings.ReplaceAll(value, `\`, "/"))
		return strings.ReplaceAll(value, "/", `\`)
	}
	return path.Clean(value)
}
func basePath(value string, windows bool) string {
	if windows {
		return path.Base(strings.ReplaceAll(cleanPath(value, true), `\`, "/"))
	}
	return path.Base(cleanPath(value, false))
}
func dirPath(value string, windows bool) string {
	if windows {
		return strings.ReplaceAll(path.Dir(strings.ReplaceAll(cleanPath(value, true), `\`, "/")), "/", `\`)
	}
	return path.Dir(cleanPath(value, false))
}
func relativePath(root, value string, windows bool) (string, error) {
	if !windows {
		root = path.Clean(root)
		value = path.Clean(value)
		if root == value {
			return ".", nil
		}
		prefix := strings.TrimRight(root, "/") + "/"
		if !strings.HasPrefix(value, prefix) {
			return "", fmt.Errorf("path is outside root")
		}
		return strings.TrimPrefix(value, prefix), nil
	}
	root = cleanPath(root, true)
	value = cleanPath(value, true)
	if strings.EqualFold(root, value) {
		return ".", nil
	}
	prefix := strings.TrimRight(root, `\`) + `\`
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", fmt.Errorf("path is outside root")
	}
	return value[len(prefix):], nil
}
func pathDepthFor(value string, windows bool) int {
	if value == "." || value == "" {
		return 0
	}
	sep := "/"
	if windows {
		sep = `\`
	}
	return len(strings.Split(cleanPath(value, windows), sep))
}
