// Package wslbridge runs the bundled Linux scanner as a short-lived WSL child.
package wslbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/metadata"
	"github.com/Geogboe/rog/internal/scanner"
	"github.com/Geogboe/rog/internal/workerbundle"
	"github.com/Geogboe/rog/internal/workerproto"
	"github.com/Geogboe/rog/internal/wsl"
)

type Approval func(distro, cachePath string) bool

type Bridge struct{ Approve Approval }

type startupError struct{ cause error }

func (e *startupError) Error() string {
	return fmt.Sprintf("WSL process exited before worker output: %v", e.cause)
}
func (e *startupError) Unwrap() error { return e.cause }

// Scan runs once for all roots in a distro and returns only discovered repos.
func (b Bridge) Scan(ctx context.Context, roots []config.Root, excludes []string, existing []*index.Repo, meta *metadata.GlobalMeta, full, remote, dryRun bool, onProgress func(string, string, int, int, int)) (scanner.WSLResult, error) {
	if runtime.GOOS != "windows" {
		return scanner.WSLResult{}, fmt.Errorf("WSL bridge requires Windows")
	}
	if len(roots) == 0 {
		return scanner.WSLResult{}, nil
	}
	distro := roots[0].WSLDistro
	binary, checksum, err := workerbundle.LinuxAMD64()
	if err != nil {
		return scanner.WSLResult{}, err
	}
	id := checksum[:16]
	req := workerproto.Request{Version: workerproto.Version, Config: config.Config{GlobalExcludes: excludes, Roots: roots}, GlobalMeta: meta, Full: full, Remote: remote, DryRun: dryRun}
	for _, repo := range existing {
		if repo == nil || !repo.IsWSL || repo.WSLDistro != distro {
			continue
		}
		// The worker only needs identity and user metadata. The Windows index
		// retains the full cached record for repos reused by a fast scan.
		for _, root := range roots {
			if repo.Root != root.Name {
				continue
			}
			linuxPath := path.Join(root.Path, filepath.ToSlash(repo.RelPath))
			req.Existing = append(req.Existing, &index.Repo{AbsPath: linuxPath, Root: repo.Root, RelPath: repo.RelPath,
				Description: repo.Description, Tags: repo.Tags, PrimaryLanguage: repo.PrimaryLanguage})
			break
		}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return scanner.WSLResult{}, err
	}
	run := func() (workerproto.Response, string, error) {
		const script = `cache="${XDG_CACHE_HOME:-$HOME/.cache}/rog/workers/$1"; worker="$cache/rog-worker"; if [ ! -x "$worker" ]; then printf 'ROG_WORKER_MISSING:%s\n' "$worker" >&2; exit 72; fi; printf 'ROG_WORKER_READY\n'; exec "$worker"`
		cmd := wsl.ExecInDistroContext(ctx, distro, "sh", "-c", script, "rog-worker", id)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return workerproto.Response{}, "", err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return workerproto.Response{}, "", err
		}
		if err := cmd.Start(); err != nil {
			return workerproto.Response{}, "", err
		}
		lines := bufio.NewScanner(stdout)
		lines.Buffer(make([]byte, 64*1024), 32<<20)
		if !lines.Scan() {
			_ = stdin.Close()
			waitErr := cmd.Wait()
			if waitErr == nil {
				waitErr = fmt.Errorf("WSL worker exited before ready")
			}
			return workerproto.Response{}, stderr.String(), waitErr
		}
		if lines.Text() != "ROG_WORKER_READY" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return workerproto.Response{}, stderr.String(), fmt.Errorf("unexpected WSL worker handshake")
		}
		if _, err := stdin.Write(payload); err != nil {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return workerproto.Response{}, stderr.String(), fmt.Errorf("send WSL scan request: %w", err)
		}
		if err := stdin.Close(); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return workerproto.Response{}, stderr.String(), err
		}
		var response workerproto.Response
		gotResult := false
		events := 0
		var decodeErr error
		for lines.Scan() {
			events++
			var event workerproto.Event
			if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
				decodeErr = fmt.Errorf("decode WSL worker event: %w", err)
				break
			}
			switch event.Type {
			case "progress":
				if event.Progress != nil && onProgress != nil {
					p := event.Progress
					onProgress(p.Root, p.Repo, p.Found, p.Refreshed, p.Reused)
				}
			case "result":
				if event.Result == nil {
					decodeErr = fmt.Errorf("empty WSL worker result")
					break
				}
				response = *event.Result
				gotResult = true
			default:
				decodeErr = fmt.Errorf("unknown WSL worker event %q", event.Type)
			}
			if decodeErr != nil {
				break
			}
		}
		if decodeErr == nil {
			decodeErr = lines.Err()
		}
		if decodeErr != nil {
			_ = cmd.Process.Kill()
		}
		waitErr := cmd.Wait()
		if decodeErr != nil {
			return response, stderr.String(), decodeErr
		}
		if waitErr != nil {
			if events == 0 && stderr.Len() == 0 {
				return response, "", &startupError{waitErr}
			}
			return response, stderr.String(), fmt.Errorf("%w (result=%v, events=%d)", waitErr, gotResult, events)
		}
		if !gotResult {
			return response, stderr.String(), fmt.Errorf("WSL worker exited without a result")
		}
		if response.Version != workerproto.Version {
			return response, stderr.String(), fmt.Errorf("WSL worker protocol %d, need %d", response.Version, workerproto.Version)
		}
		return response, stderr.String(), nil
	}

	response, stderr, err := run()
	var startup *startupError
	if errors.As(err, &startup) && ctx.Err() == nil {
		// WSL occasionally exits before starting the child under host load.
		// Retry only when it emitted no worker output or diagnostic.
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return scanner.WSLResult{}, ctx.Err()
		case <-timer.C:
		}
		response, stderr, err = run()
	}
	if err != nil {
		cachePath := ""
		for _, line := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(line, "ROG_WORKER_MISSING:") {
				cachePath = strings.TrimPrefix(line, "ROG_WORKER_MISSING:")
				break
			}
		}
		if cachePath == "" {
			return scanner.WSLResult{}, fmt.Errorf("WSL worker in %s: %w: %s", distro, err, strings.TrimSpace(stderr))
		}
		if b.Approve == nil || !b.Approve(distro, cachePath) {
			return scanner.WSLResult{}, fmt.Errorf("WSL worker installation declined for %s at %s", distro, cachePath)
		}
		if err := install(ctx, distro, id, checksum, binary); err != nil {
			return scanner.WSLResult{}, err
		}
		response, stderr, err = run()
		if err != nil {
			return scanner.WSLResult{}, fmt.Errorf("WSL worker in %s after install: %w: %s", distro, err, strings.TrimSpace(stderr))
		}
	}
	for _, repo := range response.Repos {
		if repo == nil {
			continue
		}
		repo.AbsPath = wsl.TranslatePathToWindows(distro, repo.AbsPath)
		repo.ID = "" // Index.Upsert generates the Windows-path identity.
		repo.IsWSL = true
		repo.WSLDistro = distro
	}
	for i, p := range response.Found {
		response.Found[i] = wsl.TranslatePathToWindows(distro, p)
	}
	return scanner.WSLResult{Repos: response.Repos, Found: response.Found, Candidates: response.Candidates, Reused: response.Reused, Refreshed: response.Refreshed, StatusUnavailable: response.StatusUnavailable, StatusTimeouts: response.StatusTimeouts,
		Discovery: time.Duration(response.DiscoveryNanos), Git: time.Duration(response.GitNanos), Metadata: time.Duration(response.MetadataNanos), CompleteRoots: response.CompleteRoots, Warnings: response.Warnings}, nil
}

const installScript = `set -eu; cache="${XDG_CACHE_HOME:-$HOME/.cache}/rog/workers/$1"; mkdir -p "$cache"; tmp=$(mktemp "$cache/.worker.XXXXXX"); trap 'rm -f "$tmp"' EXIT; cat > "$tmp"; actual=$(sha256sum "$tmp"); actual=${actual%% *}; [ "$actual" = "$2" ]; chmod 700 "$tmp"; mv -f "$tmp" "$cache/rog-worker"`

func install(ctx context.Context, distro, id, checksum string, binary []byte) error {
	cmd := wsl.ExecInDistroContext(ctx, distro, "sh", "-c", installScript, "rog-worker", id, checksum)
	cmd.Stdin = bytes.NewReader(binary)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install WSL worker in %s: %w: %s", distro, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
