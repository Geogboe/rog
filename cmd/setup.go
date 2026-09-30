package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/setup"
	"github.com/Geogboe/rog/internal/setupui"
	"github.com/Geogboe/rog/internal/windowsbridge"
	"github.com/Geogboe/rog/internal/wsl"
	"github.com/Geogboe/rog/internal/wslbridge"
)

var setupRollback bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure rog with a guided repository discovery wizard",
	Long:  "Walk through environment selection, repository discovery, Configured Roots, report identity, and a reviewable config transaction.",
	Args:  cobra.NoArgs,
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().BoolVar(&setupRollback, "rollback", false, "Choose and restore a previous setup configuration")
}

func runSetup(cmd *cobra.Command, args []string) error {
	configPath := config.GetConfigPath()
	if setupRollback {
		return runSetupRollback(configPath)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	idx, err := index.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}
	local := setup.LocalSearchRoots()
	external := setupExternalRoots(cfg)
	ctx, cancel := signalSetupContext(cmd.Context())
	defer cancel()
	bridgeStatus := "native filesystem only"
	if runtime.GOOS == "windows" {
		if wsl.IsAvailable() {
			bridgeStatus = "WSL registered; selected distros start only when discovery begins"
		} else {
			bridgeStatus = "WSL not available"
		}
	} else if _, err := exec.LookPath("rog.exe"); err == nil {
		bridgeStatus = "Windows rog.exe available through WSL interop"
	} else {
		bridgeStatus = "Windows worker unavailable; Linux discovery remains available"
	}
	result, err := setupui.Run(ctx, configPath, cfg, local, external, setupui.IndexSummary{Count: idx.Count(), UpdatedAt: idx.UpdatedAt, BridgeStatus: bridgeStatus}, discoverSetup)
	if err != nil {
		if errors.Is(err, setupui.ErrCancelled) {
			return nil
		}
		return err
	}
	if result.Cancelled {
		return nil
	}
	if result.ScanNow {
		fmt.Fprintln(os.Stderr, "Running initial rog scan…")
		runScan(scanCmd, nil)
	} else {
		fmt.Fprintf(os.Stderr, "Setup saved. Run `rog scan` to build the index, then `rog report` to review work.\n")
	}
	return nil
}

func setupExternalRoots(cfg *config.Config) []setup.SearchRoot {
	var roots []setup.SearchRoot
	if runtime.GOOS == "windows" {
		if distros, err := wsl.ListDistros(); err == nil {
			for _, d := range distros {
				name := "wsl-" + d
				roots = append(roots, setup.SearchRoot{Name: name, Path: "/", WSL: true, Distro: d})
			}
		}
		for _, r := range cfg.Roots {
			if r.WSL && !containsSearchRoot(roots, r.Name) {
				roots = append(roots, setup.SearchRoot{Name: r.Name, Path: r.Path, WSL: true, Distro: r.WSLDistro})
			}
		}
	} else {
		for _, r := range cfg.Roots {
			if r.Windows {
				roots = append(roots, setup.SearchRoot{Name: r.Name, Path: r.Path, Windows: true})
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return roots
}

func containsSearchRoot(roots []setup.SearchRoot, name string) bool {
	for _, r := range roots {
		if strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

func discoverSetup(ctx context.Context, roots []setup.SearchRoot, excludes []string, progress func(string, string, int, int)) setup.DiscoveryResult {
	var all setup.DiscoveryResult
	var native, windowsRoots []setup.SearchRoot
	wslRoots := map[string][]config.Root{}
	for _, r := range roots {
		switch {
		case r.Windows:
			windowsRoots = append(windowsRoots, r)
		case r.WSL:
			wslRoots[r.Distro] = append(wslRoots[r.Distro], config.Root{Name: r.Name, Path: r.Path, WSL: true, WSLDistro: r.Distro, MaxDepth: 64})
		default:
			native = append(native, r)
		}
	}
	mergeDiscovery(&all, setup.Discover(ctx, native, excludes, progress))
	if len(windowsRoots) > 0 {
		rs := make([]config.Root, 0, len(windowsRoots))
		for _, r := range windowsRoots {
			rs = append(rs, config.Root{Name: r.Name, Path: r.Path, Windows: true, MaxDepth: 64})
		}
		result, err := (windowsbridge.Bridge{}).Discover(ctx, rs, excludes, progress)
		if err != nil {
			all.Warnings = append(all.Warnings, "Windows discovery: "+err.Error())
		} else {
			mergeDiscovery(&all, result)
		}
	}
	if runtime.GOOS == "windows" {
		for distro, rs := range wslRoots {
			result, err := (wslbridge.Bridge{}).Discover(ctx, rs, excludes, progress)
			if err != nil {
				all.Warnings = append(all.Warnings, fmt.Sprintf("WSL %s discovery: %v", distro, err))
			} else {
				mergeDiscovery(&all, result)
			}
		}
	}
	sort.Slice(all.Candidates, func(i, j int) bool { return all.Candidates[i].Path < all.Candidates[j].Path })
	return all
}

func mergeDiscovery(dst *setup.DiscoveryResult, src setup.DiscoveryResult) {
	dst.Candidates = append(dst.Candidates, src.Candidates...)
	dst.Rejected += src.Rejected
	dst.Warnings = append(dst.Warnings, src.Warnings...)
}

func runSetupRollback(path string) error {
	revisions, err := setup.History(path)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return fmt.Errorf("no setup revisions are available to restore")
	}
	ctx, cancel := signalSetupContext(context.Background())
	defer cancel()
	scanNow, restored, err := setupui.RunRollback(ctx, path, revisions)
	if err != nil {
		return err
	}
	if !restored {
		return nil
	}
	if scanNow {
		runScan(scanCmd, nil)
	} else {
		fmt.Fprintln(os.Stderr, "Rollback saved. Run `rog scan` to refresh the index if needed.")
	}
	return nil
}

func signalSetupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt)
}
