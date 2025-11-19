package cmd

import (
	"fmt"
	"log"
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
  --remote: Fetch remote status (ahead/behind) - requires network
  --llm: Use LLM to generate descriptions/tags for repos missing them
  --refresh-meta: Allow LLM to update previously generated metadata`,
	Run: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "Show scan metrics without processing (for debugging performance)")
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

	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	fmt.Printf("Scanning %d roots...\n", len(cfg.Roots))

	// Create scanner
	scan := scanner.New(cfg, idx).WithRemoteCheck(scanRemote).WithDryRun(scanDryRun)

	// Scan repositories
	if err := scan.Scan(); err != nil {
		exitWithError("Scan failed: %v", err)
	}

	// Show metrics if dry-run
	if scanDryRun {
		metrics := scan.GetMetrics()
		duration := metrics.EndTime.Sub(metrics.StartTime)

		fmt.Printf("\n📊 Scan Metrics (Dry Run)\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("⏱️  Duration:           %v\n", duration.Round(time.Millisecond))
		fmt.Printf("📁 Total Directories:   %d\n", metrics.TotalDirs)
		fmt.Printf("✓  Directories Scanned: %d\n", metrics.DirsScanned)
		fmt.Printf("✗  Directories Excluded: %d\n", metrics.DirsExcluded)
		fmt.Printf("⚠️  Directories Skipped:  %d (max depth)\n", metrics.DirsSkipped)
		fmt.Printf("🔍 Repositories Found:   %d\n", metrics.ReposFound)
		fmt.Printf("\n📈 Statistics:\n")
		fmt.Printf("   Deepest Path:  %s (depth %d)\n", metrics.DeepestPath, metrics.DeepestDepth)
		fmt.Printf("   Largest Dir:   %s (%d subdirs)\n", metrics.LargestDir, metrics.LargestDirSize)
		fmt.Printf("\n⚡ Performance:\n")
		fmt.Printf("   %.0f dirs/sec\n", float64(metrics.DirsScanned)/duration.Seconds())
		fmt.Printf("   %.0f repos/sec\n", float64(metrics.ReposFound)/duration.Seconds())
		fmt.Printf("   %.2f ms per dir\n", duration.Seconds()*1000.0/float64(metrics.DirsScanned))

		if metrics.DirsExcluded > 0 {
			excludePercent := float64(metrics.DirsExcluded) / float64(metrics.TotalDirs) * 100
			fmt.Printf("\n💡 %.1f%% of directories were excluded (saved %.1fs)\n",
				excludePercent,
				float64(metrics.DirsExcluded)*duration.Seconds()/float64(metrics.DirsScanned))
		}

		return
	}

	// Remove stale entries
	removed := idx.RemoveStale()
	if removed > 0 {
		fmt.Printf("Removed %d stale repositories\n", removed)
	}

	fmt.Printf("Found %d repositories\n", idx.Count())

	// LLM enrichment if requested
	if scanLLM {
		if cfg.LLM == nil || cfg.LLM.Endpoint == "" {
			exitWithError("LLM not configured. Add LLM settings to config.yml")
		}

		fmt.Println("\nEnriching repositories with LLM...")
		if err := enrichWithLLM(cfg, idx, scanRefreshMeta); err != nil {
			log.Printf("Warning: LLM enrichment failed: %v", err)
		}
	}

	// Save index
	if err := idx.Save(); err != nil {
		exitWithError("Failed to save index: %v", err)
	}

	duration := time.Since(start)
	fmt.Printf("\n✓ Scan completed in %v\n", duration.Round(time.Millisecond))
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
