package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/pkg/selfupdate"
)

// updateOptions holds the resolved configuration for a single update invocation.
type updateOptions struct {
	pinnedVersion string
	proxyURL      string
	token         string
	checkOnly     bool
}

// updaterIface is the narrow interface used by runUpdate, enabling injection in tests.
type updaterIface interface {
	// CheckLatest returns the latest release version tag (e.g. "v0.5.0").
	CheckLatest() (string, error)
	// Install downloads and installs the given version to exePath.
	Install(version, exePath string) error
}

// updateNewUpdater is the factory used to create an updaterIface.
// It is a package-level variable so tests can replace it.
var updateNewUpdater = defaultUpdateNewUpdater

// defaultUpdateNewUpdater builds a real *selfupdate.Updater wrapped in rogUpdater.
func defaultUpdateNewUpdater(opts updateOptions) updaterIface {
	transport := &http.Transport{
		Proxy: proxyFunc(opts.proxyURL),
	}
	client := &http.Client{Transport: transport}

	u := &selfupdate.Updater{
		Repo:       "Geogboe/rog",
		BinaryName: "rog",
		Token:      opts.token,
		Client:     client,
		AssetNamer: func(v, goos, goarch string) string {
			return "rog-" + v + "-" + goos + "-" + goarch
		},
	}
	return &rogUpdater{u: u, pinnedVersion: opts.pinnedVersion}
}

// rogUpdater wraps selfupdate.Updater to implement updaterIface.
type rogUpdater struct {
	u             *selfupdate.Updater
	pinnedVersion string
}

func (r *rogUpdater) CheckLatest() (string, error) {
	ctx := context.Background()
	if r.pinnedVersion != "" {
		rel, err := r.u.FetchRelease(ctx, r.pinnedVersion)
		if err != nil {
			return "", err
		}
		return rel.Version, nil
	}
	rel, err := r.u.CheckLatest(ctx)
	if err != nil {
		return "", err
	}
	return rel.Version, nil
}

func (r *rogUpdater) Install(targetVersion, exePath string) error {
	ctx := context.Background()
	var rel *selfupdate.Release
	var err error
	if r.pinnedVersion != "" {
		rel, err = r.u.FetchRelease(ctx, targetVersion)
	} else {
		rel, err = r.u.CheckLatest(ctx)
	}
	if err != nil {
		return err
	}
	return r.u.Install(ctx, rel, exePath)
}

// proxyFunc returns an http.Transport-compatible proxy function.
// If proxyURL is non-empty it is used; otherwise http.ProxyFromEnvironment is used.
func proxyFunc(proxyURL string) func(*http.Request) (*url.URL, error) {
	if proxyURL == "" {
		return http.ProxyFromEnvironment
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return http.ProxyFromEnvironment
	}
	return http.ProxyURL(parsed)
}

// --- Cobra command ---

var (
	updateCheckOnly     bool
	updatePinnedVersion string
	updateProxy         string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update rog to the latest release",
	Long: `Update the rog binary in-place to the latest (or a pinned) release from GitHub.

Environment variables:
  ROG_GITHUB_TOKEN   GitHub API token to avoid rate limits`,
	Args:  cobra.NoArgs,
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "Check for updates without installing")
	updateCmd.Flags().StringVar(&updatePinnedVersion, "version", "", "Install a specific version (e.g. v0.5.0)")
	updateCmd.Flags().StringVar(&updateProxy, "proxy", "", "HTTP proxy URL (overrides HTTPS_PROXY env var)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	opts := updateOptions{
		pinnedVersion: updatePinnedVersion,
		proxyURL:      updateProxy,
		token:         os.Getenv("ROG_GITHUB_TOKEN"),
		checkOnly:     updateCheckOnly,
	}

	current := resolveVersion()
	updater := updateNewUpdater(opts)

	fmt.Println("==> Checking for updates...")
	fmt.Printf("    Current version: %s\n", current)

	latest, err := updater.CheckLatest()
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}
	fmt.Printf("    Latest version:  %s\n", latest)

	// Normalize both versions for comparison (strip leading "v").
	if strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest, "v") {
		fmt.Printf("✓ Already up to date (%s)\n", latest)
		return nil
	}

	if opts.checkOnly {
		fmt.Printf("update available — run 'rog update' to install\n")
		return nil
	}

	// Determine the path of the running executable.
	exePath, err := resolveExePath()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}

	fmt.Printf("==> Downloading rog %s...\n", latest)
	if err := updater.Install(latest, exePath); err != nil {
		if isPermissionError(err) {
			return fmt.Errorf(
				"install failed (permission denied): %w\n"+
					"  The binary at %s is not writable by the current user.\n"+
					"  Run as the user who owns that file.",
				err, exePath,
			)
		}
		return fmt.Errorf("install: %w", err)
	}

	fmt.Printf("✓ rog updated to %s\n", latest)

	// Warn if the install directory is not in PATH.
	installDir := filepath.Dir(exePath)
	if msg := selfupdate.PathWarningMessage(installDir); msg != "" {
		fmt.Print("\n" + msg)
	}

	return nil
}

// resolveExePath returns the path of the running executable.
// In tests, ROG_TEST_EXE_PATH can override this.
func resolveExePath() (string, error) {
	if p := os.Getenv("ROG_TEST_EXE_PATH"); p != "" {
		return p, nil
	}
	return os.Executable()
}

// isPermissionError reports whether err contains a permission-denied indication.
func isPermissionError(err error) bool {
	return err != nil && (os.IsPermission(err) ||
		strings.Contains(strings.ToLower(err.Error()), "permission denied") ||
		strings.Contains(strings.ToLower(err.Error()), "access is denied"))
}
