// Package workerrun serves scan and report requests on the filesystem owner.
package workerrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/report"
	"github.com/Geogboe/rog/internal/scanner"
	"github.com/Geogboe/rog/internal/setup"
	"github.com/Geogboe/rog/internal/workerproto"
)

// Run writes worker events without reading or writing the user's index.
func Run(ctx context.Context, req workerproto.Request, out io.Writer) error {
	if req.Version != workerproto.Version {
		return fmt.Errorf("protocol version %d, need %d", req.Version, workerproto.Version)
	}
	if req.Operation == "report" {
		return runReport(ctx, req, out)
	}
	if req.Operation == "discover_locations" {
		enc := json.NewEncoder(out)
		var mu sync.Mutex
		result := setup.Discover(ctx, req.DiscoveryRoots, req.DiscoveryExcludes, func(root, name string, done, total int) {
			mu.Lock()
			defer mu.Unlock()
			_ = enc.Encode(workerproto.Event{Type: "discovery_progress", DiscoveryProgress: &workerproto.DiscoveryProgress{Root: root, Name: name, Completed: done, Total: total}})
		})
		mu.Lock()
		defer mu.Unlock()
		return enc.Encode(workerproto.Event{Type: "discovery_result", Result: &workerproto.Response{Version: workerproto.Version, Discovery: &result}})
	}
	if req.Operation != "" && req.Operation != "scan" {
		return fmt.Errorf("unknown worker operation %q", req.Operation)
	}
	idx := index.New()
	for _, repo := range req.Existing {
		if repo != nil {
			// Preserve a zero scan timestamp on compact cached placeholders.
			idx.Repos[repo.AbsPath] = repo
		}
	}
	scan := scanner.New(&req.Config, idx).WithRemoteCheck(req.Remote).WithDryRun(req.DryRun).WithReuseExisting(!req.Full && !req.Remote).WithGlobalMeta(req.GlobalMeta)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	encoder := json.NewEncoder(out)
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
		return scanErr
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
		return err
	}
	return nil
}

func runReport(ctx context.Context, req workerproto.Request, out io.Writer) error {
	if req.Report == nil {
		return fmt.Errorf("missing report request")
	}
	encoder := json.NewEncoder(out)
	var progressErr error
	projects, warnings := report.CollectLocal(ctx, req.Existing, report.Options{Since: req.Report.Since, Until: req.Report.Until, AuthorEmails: req.Report.AuthorEmails, IncludePatches: req.Report.IncludePatches, OnProgress: func(done, total int, _ string) {
		if progressErr == nil && (done%10 == 0 || done == total) {
			progressErr = encoder.Encode(workerproto.Event{Type: "report_progress", ReportProgress: &workerproto.ReportProgress{Completed: done, Total: total}})
		}
	}})
	if progressErr != nil {
		return progressErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for i := range projects {
		if err := encoder.Encode(workerproto.Event{Type: "project", Project: &projects[i]}); err != nil {
			return err
		}
		if len(projects[i].AIPatches) > 0 {
			if err := encoder.Encode(workerproto.Event{Type: "patches", Patches: &workerproto.PatchEvent{Index: i, Excerpts: projects[i].AIPatches}}); err != nil {
				return err
			}
		}
	}
	if err := encoder.Encode(workerproto.Event{Type: "result", Result: &workerproto.Response{Version: workerproto.Version, Warnings: warnings}}); err != nil {
		return err
	}
	return nil
}
