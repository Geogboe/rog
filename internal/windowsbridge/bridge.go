// Package windowsbridge runs the installed Windows rog executable once per WSL operation.
package windowsbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/metadata"
	"github.com/Geogboe/rog/internal/report"
	"github.com/Geogboe/rog/internal/scanner"
	"github.com/Geogboe/rog/internal/setup"
	"github.com/Geogboe/rog/internal/workerproto"
)

type Bridge struct{ Executable string }

func (b Bridge) executable() (string, error) {
	if b.Executable != "" {
		return b.Executable, nil
	}
	executable, err := exec.LookPath("rog.exe")
	if err != nil {
		return "", fmt.Errorf("rog.exe is unavailable in WSL PATH; install matching Windows rog and enable WSL interop: %w", err)
	}
	return executable, nil
}

func (b Bridge) invoke(ctx context.Context, req workerproto.Request, onEvent func(workerproto.Event) error) error {
	executable, err := b.executable()
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, executable, "__windows-worker")
	command.WaitDelay = time.Second
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Windows rog worker: %w", err)
	}
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			_ = stdout.Close()
			_ = command.Process.Kill()
		case <-stopWatch:
		}
	}()
	defer func() { _ = stdin.Close() }()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 64*1024), 32<<20)
	ready := make(chan bool, 1)
	go func() { ready <- lines.Scan() }()
	select {
	case ok := <-ready:
		if !ok {
			_ = command.Wait()
			return fmt.Errorf("Windows rog worker exited before ready: %s", strings.TrimSpace(stderr.String()))
		}
	case <-time.After(20 * time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("Windows rog worker startup timed out")
	case <-ctx.Done():
		_ = command.Process.Kill()
		_ = command.Wait()
		return ctx.Err()
	}
	if lines.Text() != fmt.Sprintf("ROG_WINDOWS_WORKER_READY %d", workerproto.Version) {
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("Windows rog worker version mismatch; install matching rog.exe and retry")
	}
	if err := json.NewEncoder(stdin).Encode(req); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("send Windows worker request: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	resultSeen := false
	for lines.Scan() {
		var event workerproto.Event
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return fmt.Errorf("decode Windows worker event: %w", err)
		}
		if event.Type == "result" || event.Type == "discovery_result" {
			resultSeen = true
		}
		if err := onEvent(event); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		_ = command.Wait()
		return err
	}
	if err := lines.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("read Windows worker output: %w", err)
	}
	if err := command.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Windows worker failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if !resultSeen {
		return fmt.Errorf("Windows worker exited without a result")
	}
	return nil
}

func windowsJoin(root, rel string) string {
	root = strings.TrimRight(strings.ReplaceAll(root, "/", `\`), `\`)
	rel = windowsRelative(rel)
	if rel == "" || rel == "." {
		return root
	}
	return root + `\` + rel
}

func windowsRelative(rel string) string { return strings.ReplaceAll(rel, "/", `\`) }

func windowsRel(root, full string) (string, bool) {
	base := strings.TrimRight(strings.ReplaceAll(root, "/", `\`), `\`)
	full = strings.ReplaceAll(full, "/", `\`)
	if strings.EqualFold(full, base) {
		return "", true
	}
	if len(full) <= len(base) || !strings.EqualFold(full[:len(base)], base) || full[len(base)] != '\\' {
		return "", false
	}
	return strings.ReplaceAll(full[len(base)+1:], `\`, "/"), true
}

func rootPaths(ctx context.Context, roots []config.Root) (map[string]string, error) {
	paths := make(map[string]string, len(roots))
	for _, root := range roots {
		command := exec.CommandContext(ctx, "wslpath", "-u", root.Path)
		output, err := command.Output()
		if err != nil {
			return nil, fmt.Errorf("translate Windows root %s with wslpath: %w", root.Name, err)
		}
		translated := strings.TrimSpace(string(output))
		if !path.IsAbs(translated) {
			return nil, fmt.Errorf("wslpath returned nonabsolute path for root %s", root.Name)
		}
		paths[root.Name] = path.Clean(translated)
	}
	return paths, nil
}

func mapPath(full string, roots []config.Root, paths map[string]string) (string, string, string, bool) {
	best := -1
	var name, rel string
	for _, root := range roots {
		candidate, ok := windowsRel(root.Path, full)
		if ok && len(root.Path) > best {
			best, name, rel = len(root.Path), root.Name, candidate
		}
	}
	if best < 0 {
		return "", "", "", false
	}
	return path.Join(paths[name], rel), name, rel, true
}

// Discover scans Windows paths using the filesystem-owning Windows process.
func (b Bridge) Discover(ctx context.Context, roots []config.Root, excludes []string, progress func(string, string, int, int)) (setup.DiscoveryResult, error) {
	var result setup.DiscoveryResult
	search := make([]setup.SearchRoot, 0, len(roots))
	for _, root := range roots {
		search = append(search, setup.SearchRoot{Name: root.Name, Path: root.Path, Windows: true})
	}
	req := workerproto.Request{Version: workerproto.Version, Operation: "discover", DiscoveryRoots: search, DiscoveryExcludes: excludes}
	err := b.invoke(ctx, req, func(event workerproto.Event) error {
		switch event.Type {
		case "discovery_progress":
			if p := event.DiscoveryProgress; p != nil && progress != nil {
				progress(p.Root, p.Name, p.Completed, p.Total)
			}
		case "discovery_result":
			if event.Result == nil || event.Result.Discovery == nil {
				return fmt.Errorf("empty Windows discovery result")
			}
			if event.Result.Version != workerproto.Version {
				return fmt.Errorf("Windows discovery protocol %d, need %d", event.Result.Version, workerproto.Version)
			}
			result = *event.Result.Discovery
		}
		return nil
	})
	return result, err
}

// Scan asks Windows to perform all filesystem and Git reads for its roots.
func (b Bridge) Scan(ctx context.Context, roots []config.Root, excludes []string, existing []*index.Repo, meta *metadata.GlobalMeta, full, remote, dryRun bool, onProgress func(string, string, int, int, int)) (scanner.WSLResult, error) {
	paths, err := rootPaths(ctx, roots)
	if err != nil {
		return scanner.WSLResult{}, err
	}
	req := workerproto.Request{Version: workerproto.Version, Operation: "scan", Config: config.Config{GlobalExcludes: excludes, Roots: roots}, GlobalMeta: meta, Full: full, Remote: remote, DryRun: dryRun}
	existingByPath := make(map[string]*index.Repo)
	for _, repo := range existing {
		if repo == nil || repo.IsWSL {
			continue
		}
		var rootPath string
		for _, root := range roots {
			if root.Name == repo.Root {
				rootPath = root.Path
				break
			}
		}
		if rootPath == "" || repo.AbsPath != path.Join(paths[repo.Root], repo.RelPath) {
			continue
		}
		native := repo.WindowsPath
		if native == "" {
			native = windowsJoin(rootPath, repo.RelPath)
		}
		existingByPath[repo.AbsPath] = repo
		req.Existing = append(req.Existing, &index.Repo{AbsPath: native, Root: repo.Root, RelPath: windowsRelative(repo.RelPath), Description: repo.Description, Tags: repo.Tags, PrimaryLanguage: repo.PrimaryLanguage})
	}
	var response workerproto.Response
	err = b.invoke(ctx, req, func(event workerproto.Event) error {
		switch event.Type {
		case "progress":
			if event.Progress != nil && onProgress != nil {
				p := event.Progress
				onProgress(p.Root, p.Repo, p.Found, p.Refreshed, p.Reused)
			}
		case "result":
			if event.Result == nil || event.Result.Version != workerproto.Version {
				return fmt.Errorf("incompatible Windows worker result")
			}
			response = *event.Result
		default:
			return fmt.Errorf("unexpected Windows worker event %q", event.Type)
		}
		return nil
	})
	if err != nil {
		return scanner.WSLResult{}, err
	}
	returned := make(map[string]struct{})
	for _, repo := range response.Repos {
		if repo == nil {
			continue
		}
		wslPath, _, rel, ok := mapPath(repo.AbsPath, roots, paths)
		if !ok {
			return scanner.WSLResult{}, fmt.Errorf("Windows worker returned path outside configured roots")
		}
		repo.WindowsPath = repo.AbsPath
		repo.AbsPath = wslPath
		repo.RelPath = rel
		repo.IsWindows = true
		repo.ID = ""
		if previous := existingByPath[wslPath]; previous != nil {
			if repo.Description == previous.Description && repo.DescriptionSource == "" {
				repo.DescriptionSource = previous.DescriptionSource
			}
			if len(repo.Tags) > 0 && repo.TagsSource == "" {
				repo.TagsSource = previous.TagsSource
			}
		}
		returned[wslPath] = struct{}{}
	}
	for i, full := range response.Found {
		wslPath, _, _, ok := mapPath(full, roots, paths)
		if !ok {
			return scanner.WSLResult{}, fmt.Errorf("Windows worker returned marker outside configured roots")
		}
		response.Found[i] = wslPath
		if previous := existingByPath[wslPath]; previous != nil && !previous.IsWindows && !dryRun {
			if _, ok := returned[wslPath]; !ok {
				migrated := *previous
				migrated.IsWindows = true
				migrated.WindowsPath = full
				response.Repos = append(response.Repos, &migrated)
				returned[wslPath] = struct{}{}
			}
		}
	}
	return scanner.WSLResult{Repos: response.Repos, Found: response.Found, Candidates: response.Candidates, Reused: response.Reused, Refreshed: response.Refreshed, StatusUnavailable: response.StatusUnavailable, StatusTimeouts: response.StatusTimeouts, Discovery: time.Duration(response.DiscoveryNanos), Git: time.Duration(response.GitNanos), Metadata: time.Duration(response.MetadataNanos), CompleteRoots: response.CompleteRoots, Warnings: response.Warnings}, nil
}

// Report delegates Git reads to Windows and translates display paths for WSL.
func (b Bridge) Report(ctx context.Context, roots []config.Root, existing []*index.Repo, opts report.Options) ([]report.Project, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	paths, err := rootPaths(ctx, roots)
	if err != nil {
		return nil, nil, err
	}
	req := workerproto.Request{Version: workerproto.Version, Operation: "report", Report: &workerproto.ReportRequest{Since: opts.Since, Until: opts.Until, AuthorEmails: opts.AuthorEmails, IncludePatches: opts.IncludePatches}}
	for _, repo := range existing {
		if !repo.IsWindows || repo.WindowsPath == "" {
			continue
		}
		if _, ok := paths[repo.Root]; !ok {
			continue
		}
		req.Existing = append(req.Existing, &index.Repo{AbsPath: repo.WindowsPath, Name: repo.Name, Root: repo.Root, RelPath: windowsRelative(repo.RelPath), PrimaryLanguage: repo.PrimaryLanguage})
	}
	var projects []report.Project
	var warnings []string
	err = b.invoke(ctx, req, func(event workerproto.Event) error {
		switch event.Type {
		case "report_progress":
			if event.ReportProgress != nil && opts.OnProgress != nil {
				opts.OnProgress(event.ReportProgress.Completed, event.ReportProgress.Total, "Windows")
			}
		case "project":
			if event.Project == nil {
				return fmt.Errorf("empty Windows report project")
			}
			projects = append(projects, *event.Project)
		case "patches":
			if event.Patches == nil || event.Patches.Index < 0 || event.Patches.Index >= len(projects) {
				return fmt.Errorf("invalid Windows report patches")
			}
			projects[event.Patches.Index].AIPatches = event.Patches.Excerpts
		case "result":
			if event.Result == nil || event.Result.Version != workerproto.Version {
				return fmt.Errorf("incompatible Windows report result")
			}
			warnings = event.Result.Warnings
		default:
			return fmt.Errorf("unexpected Windows report event %q", event.Type)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	for i := range projects {
		for j, full := range projects[i].Paths {
			mapped, _, _, ok := mapPath(full, roots, paths)
			if !ok {
				return nil, nil, fmt.Errorf("Windows report path outside configured roots")
			}
			projects[i].Paths[j] = mapped
		}
		for j, tree := range projects[i].Worktrees {
			mapped, _, _, ok := mapPath(tree.Path, roots, paths)
			if !ok {
				return nil, nil, fmt.Errorf("Windows report worktree outside configured roots")
			}
			projects[i].Worktrees[j].Path = mapped
		}
		projects[i].SourcePath = ""
	}
	return projects, warnings, nil
}
