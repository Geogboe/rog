// rog-worker scans Linux roots for a Windows rog process. It only reads user
// repositories and writes a response to stdout; it never writes an index.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/scanner"
	"github.com/Geogboe/rog/internal/workerproto"
)

func main() {
	var req workerproto.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fail(fmt.Errorf("decode request: %w", err))
	}
	if req.Version != workerproto.Version {
		fail(fmt.Errorf("protocol version %d, need %d", req.Version, workerproto.Version))
	}
	idx := index.New()
	for _, repo := range req.Existing {
		if repo != nil {
			// Preserve a zero scan timestamp on compact cached placeholders.
			idx.Repos[repo.AbsPath] = repo
		}
	}
	scan := scanner.New(&req.Config, idx).WithRemoteCheck(req.Remote).WithDryRun(req.DryRun).WithReuseExisting(!req.Full && !req.Remote).WithGlobalMeta(req.GlobalMeta)
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	encoder := json.NewEncoder(os.Stdout)
	var outputMu sync.Mutex
	done := make(chan struct{})
	var progress sync.WaitGroup
	progress.Add(1)
	go func() {
		defer progress.Done()
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				metrics := scan.SnapshotMetrics()
				event := workerproto.Event{Type: "progress", Progress: &workerproto.Progress{Root: metrics.CurrentRoot, Repo: metrics.CurrentRepo, Found: metrics.ReposFound, Refreshed: metrics.ReposRefreshed, Reused: metrics.ReposReused}}
				outputMu.Lock()
				if err := encoder.Encode(event); err != nil {
					cancel()
				}
				outputMu.Unlock()
			}
		}
	}()
	scanErr := scan.ScanContext(ctx)
	close(done)
	progress.Wait()
	var incomplete *scanner.IncompleteError
	if scanErr != nil && !errors.As(scanErr, &incomplete) {
		fail(scanErr)
	}
	var warnings []string
	if incomplete != nil {
		for _, err := range incomplete.Errors {
			warnings = append(warnings, err.Error())
		}
	}
	completeMap := scan.CompletedRoots()
	completeRoots := make([]string, 0, len(completeMap))
	for name := range completeMap {
		completeRoots = append(completeRoots, name)
	}
	sort.Strings(completeRoots)
	found := scan.FoundPaths()
	paths := make([]string, 0, len(found))
	repos := make([]*index.Repo, 0, len(found))
	for p := range found {
		paths = append(paths, p)
		if repo, ok := idx.Get(p); ok && !repo.LastScanAt.IsZero() {
			repos = append(repos, repo)
		}
	}
	sort.Strings(paths)
	metrics := scan.SnapshotMetrics()
	response := workerproto.Response{Version: workerproto.Version, Found: paths, Candidates: metrics.ReposFound, Repos: repos, CompleteRoots: completeRoots, Warnings: warnings,
		Reused: metrics.ReposReused, Refreshed: metrics.ReposRefreshed, StatusUnavailable: metrics.StatusUnavailable, StatusTimeouts: metrics.StatusTimeouts,
		DiscoveryNanos: int64(metrics.DiscoveryDuration), GitNanos: int64(metrics.GitDuration), MetadataNanos: int64(metrics.MetadataDuration)}
	if err := encoder.Encode(workerproto.Event{Type: "result", Result: &response}); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "rog WSL worker:", err); os.Exit(1) }
