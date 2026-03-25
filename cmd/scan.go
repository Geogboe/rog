package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/llm"
	"github.com/Geogboe/rog/internal/scanner"
)

var (
	scanRemote      bool
	scanLLM         bool
	scanRefreshMeta bool
	scanDryRun      bool
	scanProgress    string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for repositories and update the index",
	Long: `Scan configured roots for Git repositories and update the index.

By default, this performs local-only operations:
  - Discovers git repositories
  - Extracts git metadata (branch, commits, status)
  - Detects primary language
  - Reads metadata files (.rogmeta.yml)

Flags:
  --dry-run: Show scan metrics without processing (for debugging performance)
  --progress: Control scan progress rendering (auto, off, plain, rich)
  --remote: Fetch remote status (ahead/behind) - requires network
  --llm: Use LLM to generate descriptions/tags for repos missing them
  --refresh-meta: Allow LLM to update previously generated metadata`,
	Run: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "Show scan metrics without processing (for debugging performance)")
	scanCmd.Flags().StringVar(&scanProgress, "progress", "", "Progress mode: auto, off, plain, rich")
	scanCmd.Flags().BoolVar(&scanRemote, "remote", false, "Check remote status (ahead/behind)")
	scanCmd.Flags().BoolVar(&scanLLM, "llm", false, "Use LLM to enrich metadata (use with --refresh-meta to update existing LLM metadata)")

	// --refresh-meta is only applicable with --llm, so mark it as hidden
	scanCmd.Flags().BoolVar(&scanRefreshMeta, "refresh-meta", false, "Refresh LLM-generated metadata (requires --llm)")
	scanCmd.Flags().MarkHidden("refresh-meta")
}

func runScan(cmd *cobra.Command, args []string) {
	start := time.Now()

	// Validate flag combinations
	if scanRefreshMeta && !scanLLM {
		exitWithError("--refresh-meta requires --llm")
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		exitWithError("Failed to load config: %v", err)
	}

	if len(cfg.Roots) == 0 {
		exitWithError("No roots configured. Run 'rog init' first.")
	}

	cfgProgress := ""
	if cfg.Scan != nil {
		cfgProgress = cfg.Scan.Progress
	}

	progressMode, err := resolveProgressMode(scanProgress, cfgProgress)
	if err != nil {
		exitWithError("%v", err)
	}

	renderer := newProgressRenderer(progressMode, isInteractiveTerminal(os.Stdout))

	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	// Create scanner
	scan := scanner.New(cfg, idx).WithRemoteCheck(scanRemote).WithDryRun(scanDryRun)
	fmt.Fprint(os.Stdout, renderer.Start(scanProgressSnapshot{
		Phase:      scanPhaseScan,
		RootsTotal: len(cfg.Roots),
	}))

	var progressWg sync.WaitGroup
	stopProgress := make(chan struct{})
	if renderer.Mode() == progressModeRich {
		progressWg.Add(1)
		go func() {
			defer progressWg.Done()
			ticker := time.NewTicker(150 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					metrics := scan.SnapshotMetrics()
					fmt.Fprint(os.Stdout, renderer.Update(scanProgressSnapshot{
						Phase:          scanPhaseScan,
						RootsTotal:     metrics.RootsTotal,
						RootsCompleted: metrics.RootsCompleted,
						ReposFound:     metrics.ReposFound,
						Duration:       time.Since(start),
					}))
				case <-stopProgress:
					return
				}
			}
		}()
	}

	// Scan repositories
	if err := scan.Scan(); err != nil {
		if renderer.Mode() == progressModeRich {
			close(stopProgress)
			progressWg.Wait()
			fmt.Fprint(os.Stdout, "\r"+clearLine())
		}
		exitWithError("Scan failed: %v", err)
	}
	if renderer.Mode() == progressModeRich {
		close(stopProgress)
		progressWg.Wait()
	}

	// Show metrics if dry-run
	if scanDryRun {
		metrics := scan.GetMetrics()
		fmt.Print(renderDryRunMetrics(metrics))
		return
	}

	// Remove stale entries
	removed := idx.RemoveStale()

	// LLM enrichment if requested
	if scanLLM {
		if cfg.LLM == nil || cfg.LLM.Endpoint == "" {
			exitWithError("LLM not configured. Add LLM settings to config.yml")
		}

		fmt.Fprintln(os.Stdout, "Enriching repositories with LLM...")
		if err := enrichWithLLM(cfg, idx, scanRefreshMeta); err != nil {
			log.Printf("Warning: LLM enrichment failed: %v", err)
		}
	}

	// Save index
	if err := idx.Save(); err != nil {
		exitWithError("Failed to save index: %v", err)
	}

	duration := time.Since(start)
	fmt.Fprint(os.Stdout, renderer.Finish(scanProgressSnapshot{
		Phase:        scanPhaseDone,
		RootsTotal:   len(cfg.Roots),
		ReposFound:   idx.Count(),
		StaleRemoved: removed,
		Duration:     duration,
	}))
}

func enrichWithLLM(cfg *config.Config, idx *index.Index, refresh bool) error {
	client := llm.NewClient(cfg.LLM)
	if client == nil {
		return fmt.Errorf("failed to create LLM client")
	}

	repos := idx.List()
	enriched := 0
	skipped := 0

	for i, repo := range repos {
		// Skip if already has manual or global metadata
		if repo.DescriptionSource == "manual" || repo.DescriptionSource == "global" {
			skipped++
			continue
		}

		// Skip if already has LLM metadata unless refresh is requested
		if !refresh && repo.DescriptionSource == "llm" && repo.Description != "" {
			skipped++
			continue
		}

		// Skip if already has description and tags from any source, unless refresh
		if !refresh && repo.Description != "" && len(repo.Tags) > 0 {
			skipped++
			continue
		}

		fmt.Printf("[%d/%d] Enriching %s...\n", i+1, len(repos), repo.Name)

		result, err := client.Enrich(repo)
		if err != nil {
			log.Printf("Warning: failed to enrich %s: %v", repo.Name, err)
			continue
		}

		// Only update if we got meaningful results
		if result.Description != "" {
			repo.Description = result.Description
			repo.DescriptionSource = "llm"
		}
		if len(result.Tags) > 0 {
			repo.Tags = result.Tags
			repo.TagsSource = "llm"
		}

		idx.Upsert(repo)
		enriched++

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\nEnriched %d repositories, skipped %d\n", enriched, skipped)
	return nil
}

func renderDryRunMetrics(metrics *scanner.ScanMetrics) string {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	var b strings.Builder

	fmt.Fprintf(&b, "Scan Metrics (Dry Run)\n")
	fmt.Fprintf(&b, "Duration:             %v\n", duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "Total Directories:    %d\n", metrics.TotalDirs)
	fmt.Fprintf(&b, "Directories Scanned:  %d\n", metrics.DirsScanned)
	fmt.Fprintf(&b, "Directories Excluded: %d\n", metrics.DirsExcluded)
	fmt.Fprintf(&b, "Directories Skipped:  %d (max depth)\n", metrics.DirsSkipped)
	fmt.Fprintf(&b, "Repositories Found:   %d\n", metrics.ReposFound)
	fmt.Fprintf(&b, "\nStatistics:\n")
	fmt.Fprintf(&b, "  Deepest Path: %s (depth %d)\n", metrics.DeepestPath, metrics.DeepestDepth)
	fmt.Fprintf(&b, "  Largest Dir:  %s (%d subdirs)\n", metrics.LargestDir, metrics.LargestDirSize)

	if duration > 0 && metrics.DirsScanned > 0 {
		fmt.Fprintf(&b, "\nPerformance:\n")
		fmt.Fprintf(&b, "  %.0f dirs/sec\n", float64(metrics.DirsScanned)/duration.Seconds())
		fmt.Fprintf(&b, "  %.0f repos/sec\n", float64(metrics.ReposFound)/duration.Seconds())
		fmt.Fprintf(&b, "  %.2f ms per dir\n", duration.Seconds()*1000.0/float64(metrics.DirsScanned))
	}

	if metrics.TotalDirs > 0 && metrics.DirsScanned > 0 && metrics.DirsExcluded > 0 {
		excludePercent := float64(metrics.DirsExcluded) / float64(metrics.TotalDirs) * 100
		fmt.Fprintf(&b, "\nExcluded %.1f%% of directories.\n", excludePercent)
	}

	return b.String()
}
