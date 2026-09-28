package wslbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/report"
	"github.com/Geogboe/rog/internal/workerbundle"
	"github.com/Geogboe/rog/internal/workerproto"
	"github.com/Geogboe/rog/internal/wsl"
)

// Report runs one Linux worker for the indexed repositories of one distro.
func (b Bridge) Report(ctx context.Context, distro string, roots []config.Root, existing []*index.Repo, opts report.Options) ([]report.Project, []string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil, fmt.Errorf("WSL report bridge requires Windows")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	binary, checksum, err := workerbundle.LinuxAMD64()
	if err != nil {
		return nil, nil, err
	}
	id := checksum[:16]
	req := workerproto.Request{Version: workerproto.Version, Operation: "report", Report: &workerproto.ReportRequest{Since: opts.Since, Until: opts.Until, AuthorEmails: opts.AuthorEmails, IncludePatches: opts.IncludePatches}}
	for _, repo := range existing {
		for _, root := range roots {
			if root.Name != repo.Root {
				continue
			}
			linuxPath := path.Join(root.Path, filepath.ToSlash(repo.RelPath))
			req.Existing = append(req.Existing, &index.Repo{AbsPath: linuxPath, Name: repo.Name, Root: repo.Root, RelPath: repo.RelPath, PrimaryLanguage: repo.PrimaryLanguage})
			break
		}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	invoke := func() ([]report.Project, []string, string, error) {
		const script = `cache="${XDG_CACHE_HOME:-$HOME/.cache}/rog/workers/$1"; worker="$cache/rog-worker"; if [ ! -x "$worker" ]; then printf 'ROG_WORKER_MISSING:%s\n' "$worker" >&2; exit 72; fi; printf 'ROG_WORKER_READY\n'; exec "$worker"`
		cmd := wsl.ExecInDistroContext(ctx, distro, "sh", "-c", script, "rog-worker", id)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, "", err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, "", err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, "", err
		}
		lines := bufio.NewScanner(stdout)
		lines.Buffer(make([]byte, 64*1024), 8<<20)
		ready := make(chan bool, 1)
		go func() { ready <- lines.Scan() }()
		var handshook bool
		select {
		case handshook = <-ready:
		case <-time.After(20 * time.Second):
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), fmt.Errorf("WSL report worker startup timed out after 20s")
		case <-ctx.Done():
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), ctx.Err()
		}
		if !handshook {
			stdin.Close()
			e := cmd.Wait()
			if e == nil {
				e = fmt.Errorf("worker exited before ready")
			}
			return nil, nil, stderr.String(), e
		}
		if lines.Text() != "ROG_WORKER_READY" {
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), fmt.Errorf("unexpected WSL report worker handshake")
		}
		if _, err := stdin.Write(payload); err != nil {
			stdin.Close()
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), err
		}
		if err := stdin.Close(); err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), err
		}
		var projects []report.Project
		var warnings []string
		done := false
		for lines.Scan() {
			var event workerproto.Event
			if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
				cmd.Process.Kill()
				cmd.Wait()
				return nil, nil, stderr.String(), fmt.Errorf("decode WSL report event: %w", err)
			}
			switch event.Type {
			case "project":
				if event.Project == nil {
					cmd.Process.Kill()
					cmd.Wait()
					return nil, nil, stderr.String(), fmt.Errorf("empty WSL report project")
				}
				projects = append(projects, *event.Project)
			case "result":
				if event.Result == nil || event.Result.Version != workerproto.Version {
					cmd.Process.Kill()
					cmd.Wait()
					return nil, nil, stderr.String(), fmt.Errorf("incompatible WSL report worker result")
				}
				warnings = append(warnings, event.Result.Warnings...)
				done = true
			case "patches":
				if event.Patches == nil || event.Patches.Index < 0 || event.Patches.Index >= len(projects) {
					cmd.Process.Kill()
					cmd.Wait()
					return nil, nil, stderr.String(), fmt.Errorf("invalid WSL report patches")
				}
				projects[event.Patches.Index].AIPatches = event.Patches.Excerpts
			case "report_progress":
				if event.ReportProgress != nil && opts.OnProgress != nil {
					opts.OnProgress(event.ReportProgress.Completed, event.ReportProgress.Total, distro)
				}
			default:
				cmd.Process.Kill()
				cmd.Wait()
				return nil, nil, stderr.String(), fmt.Errorf("unexpected WSL report event %q", event.Type)
			}
		}
		if err := lines.Err(); err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			return nil, nil, stderr.String(), fmt.Errorf("read WSL report: %w", err)
		}
		if err := cmd.Wait(); err != nil {
			if ctx.Err() != nil {
				return nil, nil, stderr.String(), ctx.Err()
			}
			return nil, nil, stderr.String(), err
		}
		if !done {
			return nil, nil, stderr.String(), fmt.Errorf("WSL report worker exited without result")
		}
		return projects, warnings, stderr.String(), nil
	}
	projects, warnings, stderr, err := invoke()
	if err != nil {
		cachePath := ""
		for _, line := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(line, "ROG_WORKER_MISSING:") {
				cachePath = strings.TrimPrefix(line, "ROG_WORKER_MISSING:")
				break
			}
		}
		if cachePath == "" {
			return nil, nil, fmt.Errorf("WSL report worker in %s: %w", distro, err)
		}
		if b.Approve == nil || !b.Approve(distro, cachePath) {
			return nil, nil, fmt.Errorf("WSL worker installation declined for %s at %s", distro, cachePath)
		}
		if err := install(ctx, distro, id, checksum, binary); err != nil {
			return nil, nil, err
		}
		projects, warnings, _, err = invoke()
		if err != nil {
			return nil, nil, fmt.Errorf("WSL report worker in %s after install: %w", distro, err)
		}
	}
	for i := range projects {
		for j, p := range projects[i].Paths {
			projects[i].Paths[j] = wsl.TranslatePathToWindows(distro, p)
		}
		for j := range projects[i].Worktrees {
			projects[i].Worktrees[j].Path = wsl.TranslatePathToWindows(distro, projects[i].Worktrees[j].Path)
		}
		projects[i].SourcePath = ""
	}
	return projects, warnings, nil
}
