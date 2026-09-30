// Package report gathers dated Git evidence from indexed repositories.
package report

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Geogboe/rog/internal/config"
	gitpkg "github.com/Geogboe/rog/internal/git"
	"github.com/Geogboe/rog/internal/index"
)

const SchemaVersion = 1

type Document struct {
	SchemaVersion       int       `json:"schema_version"`
	GeneratedAt         time.Time `json:"generated_at"`
	IndexUpdatedAt      time.Time `json:"index_updated_at"`
	Since               time.Time `json:"since"`
	Until               time.Time `json:"until"`
	Timezone            string    `json:"timezone"`
	ConfiguredRoots     []string  `json:"configured_roots"`
	Projects            []Project `json:"projects"`
	Warnings            []string  `json:"warnings,omitempty"`
	Complete            bool      `json:"complete"`
	CollectionMS        int64     `json:"collection_ms"`
	AISummary           string    `json:"ai_summary,omitempty"`
	Coverage            Coverage  `json:"coverage"`
	CoverageExplanation string    `json:"coverage_explanation,omitempty"`
}

type Coverage struct {
	IndexedRepositories        int `json:"indexed_repositories"`
	GroupedProjects            int `json:"grouped_projects"`
	ProjectsWithCommits        int `json:"projects_with_commits"`
	ProjectsWithCurrentChanges int `json:"projects_with_current_changes"`
	UnavailableProjects        int `json:"unavailable_projects"`
}

// ExplainCoverage describes why a project is or is not represented in a report.
func ExplainCoverage(query string, cfg *config.Config, repos []*index.Repo, d Document) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return "Enter a project name or path."
	}
	var matched *index.Repo
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		if strings.EqualFold(repo.Name, q) || strings.EqualFold(repo.AbsPath, q) || strings.EqualFold(repo.WindowsPath, q) {
			matched = repo
			break
		}
		if strings.Contains(strings.ToLower(repo.AbsPath), strings.ToLower(q)) {
			matched = repo
		}
	}
	if matched != nil {
		var project *Project
		for i := range d.Projects {
			for _, p := range d.Projects[i].Paths {
				if sameReportPath(p, matched.AbsPath) || sameReportPath(p, matched.WindowsPath) {
					project = &d.Projects[i]
					break
				}
			}
			if project != nil {
				break
			}
		}
		if project == nil {
			return fmt.Sprintf("%s is indexed, but its report evidence is unavailable. Check warnings and worker availability.", matched.Name)
		}
		if len(project.Commits) > 0 || project.Metrics.CurrentChangedPaths > 0 {
			activeRank := 0
			for i := range d.Projects {
				candidate := d.Projects[i]
				if candidate.Metrics.Commits == 0 && candidate.Metrics.CurrentChangedPaths == 0 {
					continue
				}
				activeRank++
				if &d.Projects[i] == project && activeRank > 50 {
					return fmt.Sprintf("%s is included in the report evidence but omitted from the Dashboard, which displays the top 50 active projects. JSON includes all projects.", matched.Name)
				}
			}
		}
		if len(project.Commits) > 0 {
			return fmt.Sprintf("%s is included with %d matching authored commits in the selected date range.", matched.Name, len(project.Commits))
		}
		if project.Metrics.CurrentChangedPaths > 0 {
			return fmt.Sprintf("%s has %d current working-tree paths, but no authored commits matched the selected range and author identities. Current edits are undated.", matched.Name, project.Metrics.CurrentChangedPaths)
		}
		if len(project.Warnings) > 0 {
			return fmt.Sprintf("%s was found, but history or working-tree evidence is incomplete: %s", matched.Name, strings.Join(project.Warnings, "; "))
		}
		return fmt.Sprintf("%s is indexed but has no authored commits matching this date range and author email. Check --since, --until, --author-email, and the Git author email in rog config.", matched.Name)
	}
	for _, root := range cfg.Roots {
		searchRoot := root.Path
		searchQuery := q
		if runtime.GOOS == "linux" && root.Windows {
			searchRoot = windowsToWSL(root.Path)
			if windowsDriveAbsolute(q) {
				searchQuery = windowsToWSL(q)
			}
		}
		if runtime.GOOS == "windows" && root.WSL {
			searchRoot = `\\wsl$\` + root.WSLDistro + strings.ReplaceAll(root.Path, "/", `\`)
			if strings.HasPrefix(q, "/") {
				searchQuery = `\\wsl$\` + root.WSLDistro + strings.ReplaceAll(q, "/", `\`)
			}
		}
		if pathWithin(searchRoot, searchQuery) {
			rel, err := filepath.Rel(searchRoot, searchQuery)
			if err == nil && root.MaxDepth > 0 && pathDepth(rel) > root.MaxDepth {
				return fmt.Sprintf("%s is under Configured Root %s but beyond max_depth %d. Increase that depth in rog setup, then run rog scan.", q, root.Name, root.MaxDepth)
			}
			parts := strings.FieldsFunc(filepath.Clean(rel), func(r rune) bool { return r == '/' || r == '\\' })
			exclusions := append(append([]string(nil), cfg.GlobalExcludes...), root.Exclude...)
			for _, part := range parts {
				for _, excluded := range exclusions {
					if strings.EqualFold(part, excluded) {
						return fmt.Sprintf("%s is beneath excluded folder %s in Configured Root %s.", q, excluded, root.Name)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(searchQuery, ".git")); err == nil {
				return fmt.Sprintf("Git marker found at %s under Configured Root %s, but it is not indexed. Run rog scan; if discovery still rejects it, rog setup will show the marker validation result.", q, root.Name)
			}
			return fmt.Sprintf("%s is under Configured Root %s but is not indexed. It may not contain a valid Git repository marker, or the last scan may be stale; run rog scan.", q, root.Name)
		}
	}
	if filepath.IsAbs(q) || windowsDriveAbsolute(q) || strings.HasPrefix(q, `\\`) {
		return fmt.Sprintf("%s is outside the current Configured Roots. Add its parent as a root in rog setup, then run rog scan.", q)
	}
	return fmt.Sprintf("No indexed project matches %q. Run rog scan to refresh Configured Roots, then use rog report --explain with the project name or path.", q)
}

func windowsDriveAbsolute(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func sameReportPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
func pathWithin(root, candidate string) bool {
	if runtime.GOOS == "linux" && len(root) > 2 && root[1] == ':' && root[2] == '\\' {
		root = windowsToWSL(root)
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(root, "/") {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func windowsToWSL(path string) string {
	drive := strings.ToLower(path[:1])
	rest := strings.ReplaceAll(path[2:], `\`, `/`)
	return "/mnt/" + drive + rest
}
func pathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
}

func (d Document) WeeklyProjects() []Project {
	var projects []Project
	for _, p := range d.Projects {
		if p.Metrics.Commits > 0 {
			projects = append(projects, p)
		}
	}
	return projects
}
func (d Document) CurrentProjects() []Project {
	var projects []Project
	for _, p := range d.Projects {
		if p.Metrics.CurrentChangedPaths > 0 {
			projects = append(projects, p)
		}
	}
	return projects
}
func (d Document) DashboardProjects() []Project {
	var projects []Project
	for _, p := range d.Projects {
		if p.Metrics.Commits > 0 || p.Metrics.CurrentChangedPaths > 0 {
			projects = append(projects, p)
			if len(projects) == 50 {
				break
			}
		}
	}
	return projects
}
func (d Document) ActiveCount() int {
	n := 0
	for _, p := range d.Projects {
		if p.Metrics.Commits > 0 || p.Metrics.CurrentChangedPaths > 0 {
			n++
		}
	}
	return n
}

type Project struct {
	Name      string     `json:"name"`
	Roots     []string   `json:"roots"`
	Paths     []string   `json:"paths"`
	Language  string     `json:"language,omitempty"`
	Identity  string     `json:"identity,omitempty"`
	Commits   []Commit   `json:"commits"`
	Worktrees []Worktree `json:"worktrees"`
	Metrics   Metrics    `json:"metrics"`
	Warnings  []string   `json:"warnings,omitempty"`
	// SourcePath is the filesystem owner's path, used only for bounded AI reads.
	SourcePath string   `json:"-"`
	AIPatches  []string `json:"-"`
}

type Commit struct {
	Hash          string    `json:"hash"`
	Subject       string    `json:"subject"`
	AuthorEmail   string    `json:"author_email"`
	AuthorTime    time.Time `json:"author_time"`
	CommitTime    time.Time `json:"commit_time"`
	Merge         bool      `json:"merge"`
	ChangedPaths  []string  `json:"changed_paths,omitempty"`
	Additions     int       `json:"additions"`
	Deletions     int       `json:"deletions"`
	BinaryChanges int       `json:"binary_changes,omitempty"`
}

type Worktree struct {
	Path         string   `json:"path"`
	ChangedPaths []string `json:"changed_paths,omitempty"`
}

type Metrics struct {
	Commits             int       `json:"commits"`
	MergeCommits        int       `json:"merge_commits"`
	ActiveDays          int       `json:"active_days"`
	ChangedPaths        int       `json:"changed_paths"`
	Additions           int       `json:"additions"`
	Deletions           int       `json:"deletions"`
	BinaryChanges       int       `json:"binary_changes"`
	FirstActivity       time.Time `json:"first_activity,omitempty"`
	LastActivity        time.Time `json:"last_activity,omitempty"`
	CurrentChangedPaths int       `json:"current_changed_paths"`
}

type Options struct {
	Since, Until   time.Time
	AuthorEmails   []string
	OnGrouping     func(done, total int)
	OnProgress     func(done, total int, name string)
	IncludePatches bool
}

func CurrentWeek(now time.Time) (time.Time, time.Time) {
	local := now.In(time.Local)
	days := (int(local.Weekday()) + 6) % 7
	start := time.Date(local.Year(), local.Month(), local.Day()-days, 0, 0, 0, 0, time.Local)
	return start, now
}

func SortProjects(projects []Project) {
	sort.SliceStable(projects, func(i, j int) bool {
		a, b := projects[i], projects[j]
		if a.Metrics.ActiveDays != b.Metrics.ActiveDays {
			return a.Metrics.ActiveDays > b.Metrics.ActiveDays
		}
		if a.Metrics.Commits != b.Metrics.Commits {
			return a.Metrics.Commits > b.Metrics.Commits
		}
		if a.Metrics.ChangedPaths != b.Metrics.ChangedPaths {
			return a.Metrics.ChangedPaths > b.Metrics.ChangedPaths
		}
		return a.Name < b.Name
	})
}

// CollectLocal reads each common Git repository once and each working tree once.
// A partial result is returned with warnings rather than hiding readable work.
func CollectLocal(ctx context.Context, repos []*index.Repo, opts Options) ([]Project, []string) {
	type group struct {
		key   string
		repos []*index.Repo
	}
	groups := map[string]*group{}
	var warnings []string
	var groupMu sync.Mutex
	grouped := 0
	preflight := make(chan *index.Repo)
	var preflightWG sync.WaitGroup
	for range min(8, len(repos)) {
		preflightWG.Add(1)
		go func() {
			defer preflightWG.Done()
			for repo := range preflight {
				if repo == nil {
					continue
				}
				common, err := readCommonGitDir(repo.AbsPath)
				if err != nil {
					out, gitErr := git(ctx, repo.AbsPath, 3*time.Second, 4096, "rev-parse", "--git-common-dir")
					if gitErr == nil {
						common = strings.TrimSpace(string(out))
						err = nil
					} else {
						err = gitErr
					}
				}
				groupMu.Lock()
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s: Git repository unavailable: %v", repo.AbsPath, err))
				} else {
					if !filepath.IsAbs(common) {
						common = filepath.Join(repo.AbsPath, common)
					}
					key := filepath.Clean(common)
					if groups[key] == nil {
						groups[key] = &group{key: key}
					}
					groups[key].repos = append(groups[key].repos, repo)
				}
				grouped++
				if opts.OnGrouping != nil {
					opts.OnGrouping(grouped, len(repos))
				}
				groupMu.Unlock()
			}
		}()
	}
	for _, repo := range repos {
		if ctx.Err() != nil {
			break
		}
		preflight <- repo
	}
	close(preflight)
	preflightWG.Wait()
	ordered := make([]*group, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].key < ordered[j].key })
	projects := make([]Project, len(ordered))
	var mu sync.Mutex
	doneCount := 0
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(8, len(ordered))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				project := collectGroup(ctx, ordered[i].repos, opts)
				projects[i] = project
				mu.Lock()
				doneCount++
				if opts.OnProgress != nil {
					opts.OnProgress(doneCount, len(ordered), project.Name)
				}
				mu.Unlock()
			}
		}()
	}
	for i := range ordered {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	result := projects[:0]
	for _, p := range projects {
		if p.Name != "" {
			result = append(result, p)
		}
	}
	return result, warnings
}

// readCommonGitDir resolves standard .git directories and linked worktree
// files without spawning Git. Unusual layouts fall back to rev-parse above.
func readCommonGitDir(repoPath string) (string, error) {
	marker := filepath.Join(repoPath, ".git")
	info, err := os.Stat(marker)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if _, err := os.Stat(filepath.Join(marker, "HEAD")); err != nil {
			return "", err
		}
		return filepath.Abs(marker)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("unsupported .git marker")
	}
	content, err := readSmallGitFile(marker)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(content, "gitdir:") {
		return "", fmt.Errorf("invalid .git file")
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(content, "gitdir:"))
	if gitDir == "" {
		return "", fmt.Errorf("empty gitdir in .git file")
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoPath, gitDir)
	}
	gitDir, err = filepath.Abs(gitDir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		return "", err
	}
	common, err := readSmallGitFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", err
	}
	if common == "" {
		return "", fmt.Errorf("empty worktree commondir")
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return filepath.Abs(common)
}

func readSmallGitFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", fmt.Errorf("Git marker exceeds 4096 bytes")
	}
	return strings.TrimSpace(string(data)), nil
}

func collectGroup(ctx context.Context, repos []*index.Repo, opts Options) Project {
	first := repos[0]
	p := Project{Name: first.Name, Language: first.PrimaryLanguage, SourcePath: first.AbsPath, Commits: []Commit{}, Worktrees: []Worktree{}}
	rootSet := map[string]bool{}
	refs := []string{"--branches", "HEAD"}
	for i, repo := range repos {
		p.Paths = append(p.Paths, repo.AbsPath)
		rootSet[repo.Root] = true
		if i > 0 {
			if head, err := git(ctx, repo.AbsPath, 3*time.Second, 128, "rev-parse", "HEAD"); err == nil {
				refs = append(refs, strings.TrimSpace(string(head)))
			}
		}
		changes, err := currentChanges(ctx, repo.AbsPath)
		if err != nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s: current changes unavailable: %v", repo.AbsPath, err))
		}
		p.Worktrees = append(p.Worktrees, Worktree{Path: repo.AbsPath, ChangedPaths: changes})
		p.Metrics.CurrentChangedPaths += len(changes)
	}
	for root := range rootSet {
		p.Roots = append(p.Roots, root)
	}
	sort.Strings(p.Roots)
	sort.Strings(p.Paths)
	emails := opts.AuthorEmails
	if len(emails) == 0 {
		for _, repo := range repos {
			out, err := git(ctx, repo.AbsPath, 3*time.Second, 1024, "config", "--get", "user.email")
			if err == nil && strings.TrimSpace(string(out)) != "" {
				emails = append(emails, strings.TrimSpace(string(out)))
			}
		}
	}
	if len(emails) == 0 {
		p.Warnings = append(p.Warnings, "Git author email is not configured; use --author-email to include commits")
	}
	p.Identity = strings.Join(emails, ", ")
	if shallow, err := git(ctx, first.AbsPath, 3*time.Second, 64, "rev-parse", "--is-shallow-repository"); err == nil && strings.TrimSpace(string(shallow)) == "true" {
		p.Warnings = append(p.Warnings, "shallow history: older commits may be unavailable")
	}
	if len(emails) == 0 {
		return p
	}
	// Git's date range filters use committer time. Read bounded reachable
	// history, then apply the requested range to author time below.
	baseArgs := []string{"log", "--no-show-signature", "--max-count=1001", "--format=%H%x1f%ae%x1f%aI%x1f%cI%x1f%P%x1f%s%x1e"}
	out, err := git(ctx, first.AbsPath, 12*time.Second, 2<<20, append(append([]string(nil), baseArgs...), refs...)...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("history unavailable: %v", err))
			return p
		}
		if _, headErr := git(ctx, first.AbsPath, 3*time.Second, 128, "rev-parse", "--verify", "HEAD"); headErr != nil {
			return p
		}
		p.Warnings = append(p.Warnings, "some local branch history is unavailable; reporting checked-out HEAD only")
		out, err = git(ctx, first.AbsPath, 12*time.Second, 2<<20, append(baseArgs, "HEAD")...)
		if err != nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("history unavailable: %v", err))
			return p
		}
	}
	seen := map[string]bool{}
	for _, record := range bytes.Split(out, []byte{0x1e}) {
		fields := strings.SplitN(strings.TrimSpace(string(record)), "\x1f", 6)
		if len(fields) != 6 || seen[fields[0]] {
			continue
		}
		if len(seen) >= 1000 {
			p.Warnings = append(p.Warnings, "history truncated at 1000 reachable commits; older authored work may be omitted")
			break
		}
		seen[fields[0]] = true
		authorAt, e1 := time.Parse(time.RFC3339, fields[2])
		commitAt, e2 := time.Parse(time.RFC3339, fields[3])
		if e1 != nil || e2 != nil || authorAt.Before(opts.Since) || !authorAt.Before(opts.Until) {
			continue
		}
		matched := false
		for _, email := range emails {
			if strings.EqualFold(strings.TrimSpace(fields[1]), strings.TrimSpace(email)) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		commit := Commit{Hash: fields[0], AuthorEmail: fields[1], AuthorTime: authorAt, CommitTime: commitAt, Subject: clean(fields[5]), Merge: len(strings.Fields(fields[4])) > 1}
		p.Commits = append(p.Commits, commit)
	}
	p.Warnings = append(p.Warnings, fillCommitStats(ctx, first.AbsPath, p.Commits)...)
	sort.Slice(p.Commits, func(i, j int) bool { return p.Commits[i].AuthorTime.After(p.Commits[j].AuthorTime) })
	p.Metrics = metrics(p)
	if opts.IncludePatches {
		p.AIPatches = PatchExcerpts(ctx, p, 6000)
	}
	return p
}

func metrics(p Project) Metrics {
	m := p.Metrics
	m.Commits = len(p.Commits)
	days := map[string]bool{}
	paths := map[string]bool{}
	for _, c := range p.Commits {
		if c.Merge {
			m.MergeCommits++
		}
		days[c.AuthorTime.In(time.Local).Format("2006-01-02")] = true
		if m.FirstActivity.IsZero() || c.AuthorTime.Before(m.FirstActivity) {
			m.FirstActivity = c.AuthorTime
		}
		if c.AuthorTime.After(m.LastActivity) {
			m.LastActivity = c.AuthorTime
		}
		for _, path := range c.ChangedPaths {
			paths[path] = true
		}
		m.Additions += c.Additions
		m.Deletions += c.Deletions
		m.BinaryChanges += c.BinaryChanges
	}
	m.ActiveDays = len(days)
	m.ChangedPaths = len(paths)
	return m
}

func parseNumstat(out []byte, commit *Commit) {
	for _, line := range strings.Split(string(out), "\x00") {
		parseNumstatLine(strings.TrimPrefix(line, "\n"), commit)
	}
}

func parseNumstatLine(line string, commit *Commit) bool {
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) != 3 {
		return false
	}
	commit.ChangedPaths = append(commit.ChangedPaths, clean(parts[2]))
	if parts[0] == "-" || parts[1] == "-" {
		commit.BinaryChanges++
		return true
	}
	a, e1 := strconv.Atoi(parts[0])
	d, e2 := strconv.Atoi(parts[1])
	if e1 == nil && e2 == nil {
		commit.Additions += a
		commit.Deletions += d
	}
	return e1 == nil && e2 == nil
}

// fillCommitStats reads selected non-merge diffs in bounded batches. A failed
// batch is split so one large or corrupt commit cannot hide its neighbors.
func fillCommitStats(ctx context.Context, dir string, commits []Commit) []string {
	var selected []int
	for i := range commits {
		if !commits[i].Merge {
			selected = append(selected, i)
		}
	}
	var warnings []string
	for start := 0; start < len(selected); start += 16 {
		end := min(start+16, len(selected))
		warnings = append(warnings, fillCommitStatsBatch(ctx, dir, commits, selected[start:end])...)
	}
	return warnings
}

func fillCommitStatsBatch(ctx context.Context, dir string, commits []Commit, selected []int) []string {
	if len(selected) == 0 {
		return nil
	}
	var input strings.Builder
	hashes := make([]string, 0, len(selected))
	for _, i := range selected {
		hashes = append(hashes, commits[i].Hash)
		input.WriteString(commits[i].Hash)
		input.WriteByte('\n')
	}
	out, err := gitInput(ctx, dir, 8*time.Second, 4<<20, []byte(input.String()), "diff-tree", "--stdin", "-r", "--root", "--numstat", "-z", "--no-renames")
	if err == nil {
		var stats []Commit
		stats, err = parseBatchNumstat(out, hashes)
		if err == nil {
			for n, i := range selected {
				commits[i].ChangedPaths = stats[n].ChangedPaths
				commits[i].Additions = stats[n].Additions
				commits[i].Deletions = stats[n].Deletions
				commits[i].BinaryChanges = stats[n].BinaryChanges
			}
			return nil
		}
	}
	if len(selected) > 1 && ctx.Err() == nil {
		mid := len(selected) / 2
		warnings := fillCommitStatsBatch(ctx, dir, commits, selected[:mid])
		return append(warnings, fillCommitStatsBatch(ctx, dir, commits, selected[mid:])...)
	}
	if ctx.Err() != nil {
		warnings := make([]string, 0, len(selected))
		for _, i := range selected {
			warnings = append(warnings, fmt.Sprintf("%s: change volume unavailable", commits[i].Hash[:min(8, len(commits[i].Hash))]))
		}
		return warnings
	}
	i := selected[0]
	// Preserve the previous Git behavior when a single commit cannot be read
	// through diff-tree, including its per-commit output bound.
	if ctx.Err() == nil {
		if stats, showErr := git(ctx, dir, 5*time.Second, 256<<10, "show", "--format=", "--numstat", "-z", "--no-renames", "--root", commits[i].Hash); showErr == nil {
			parseNumstat(stats, &commits[i])
			return nil
		}
	}
	return []string{fmt.Sprintf("%s: change volume unavailable", commits[i].Hash[:min(8, len(commits[i].Hash))])}
}

func parseBatchNumstat(out []byte, hashes []string) ([]Commit, error) {
	stats := make([]Commit, len(hashes))
	current := -1
	for _, part := range bytes.Split(out, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		line := strings.TrimPrefix(string(part), "\n")
		if current+1 < len(hashes) && line == hashes[current+1] {
			current++
			continue
		}
		if current < 0 || !parseNumstatLine(line, &stats[current]) {
			return nil, fmt.Errorf("unexpected diff-tree output")
		}
	}
	if current+1 != len(hashes) {
		return nil, fmt.Errorf("missing diff-tree commits")
	}
	return stats, nil
}

func currentChanges(ctx context.Context, dir string) ([]string, error) {
	out, err := git(ctx, dir, 6*time.Second, 512<<10, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	parts := bytes.Split(out, []byte{0})
	for i := 0; i < len(parts); i++ {
		if len(parts[i]) < 4 {
			continue
		}
		entry := string(parts[i])
		paths[clean(entry[3:])] = true
		if entry[0] == 'R' || entry[1] == 'R' || entry[0] == 'C' || entry[1] == 'C' {
			i++
		}
	}
	result := make([]string, 0, len(paths))
	for p := range paths {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("Git output exceeded report limit")
	}
	return b.Buffer.Write(p)
}

func git(ctx context.Context, dir string, timeout time.Duration, limit int, args ...string) ([]byte, error) {
	return gitInput(ctx, dir, timeout, limit, nil, args...)
}

func gitInput(ctx context.Context, dir string, timeout time.Duration, limit int, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := gitpkg.CommandContext(ctx, args...)
	cmd.Dir = dir
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var out limitedBuffer
	out.limit = limit
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return out.Bytes(), nil
}
