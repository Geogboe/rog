// Package selfupdate provides a generic, zero-dependency self-update mechanism
// for Go binaries distributed via GitHub Releases.
//
// It handles fetching the latest release, downloading the platform-specific
// asset, verifying its SHA256 checksum, extracting the binary, and atomically
// replacing the running executable.
//
// Example usage:
//
//	u := &selfupdate.Updater{
//	    Repo:  "owner/repo",
//	    Token: os.Getenv("GITHUB_TOKEN"),
//	    AssetNamer: func(v, goos, goarch string) string {
//	        return "mytool-" + v + "-" + goos + "-" + goarch
//	    },
//	}
//
//	rel, err := u.CheckLatest(ctx)
//	if err != nil { ... }
//
//	exePath, _ := os.Executable()
//	if err := u.Install(ctx, rel, exePath); err != nil { ... }
package selfupdate

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// HTTPDoer is the interface satisfied by *http.Client.
// Inject a custom implementation for testing or proxy configuration.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// AssetNamer maps (version, goos, goarch) to an asset base filename without extension.
// version is the release version without the leading "v" prefix (e.g. "0.5.0").
// The package appends ".tar.gz" (non-Windows) or ".zip" (Windows) automatically.
//
// Example for a GoReleaser project named "mytool":
//
//	func(v, goos, goarch string) string { return "mytool-" + v + "-" + goos + "-" + goarch }
type AssetNamer func(version, goos, goarch string) string

// Release describes a GitHub release resolved for the current platform.
type Release struct {
	// Version is the full release tag, e.g. "v0.5.0".
	Version string
	// AssetName is the resolved asset filename, e.g. "rog-0.5.0-linux-amd64.tar.gz".
	AssetName string
	// DownloadURL is the direct URL to download the asset.
	DownloadURL string
	// ChecksumURL is the URL for the checksums.txt file for this release.
	ChecksumURL string
}

// Updater performs self-update operations against a GitHub repository.
//
// Zero-value is not usable; at minimum Repo, BinaryName, and AssetNamer must be set.
type Updater struct {
	// Repo is the GitHub repository in "owner/repo" format (required).
	Repo string
	// BinaryName is the base name of the binary to extract from the archive (required).
	// Do not include the ".exe" suffix — it is added automatically on Windows.
	// Example: "rog"
	BinaryName string
	// Token is an optional GitHub API token to avoid rate limits.
	Token string
	// Client is an optional HTTP client. If nil, http.DefaultClient is used.
	// Set a custom client to configure proxy, timeouts, or TLS settings.
	Client HTTPDoer
	// AssetNamer determines the asset base filename for the current platform (required).
	AssetNamer AssetNamer
}

// httpClient returns the configured HTTP client or http.DefaultClient.
func (u *Updater) httpClient() HTTPDoer {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

// CheckLatest fetches the latest release from GitHub without downloading it.
// Returns the release metadata resolved for the current OS and architecture.
func (u *Updater) CheckLatest(ctx context.Context) (*Release, error) {
	if err := u.validate(); err != nil {
		return nil, err
	}
	return u.fetchLatestRelease(ctx)
}

// FetchRelease fetches a specific version's release from GitHub without downloading it.
// version may include or omit the leading "v" (both "v0.5.0" and "0.5.0" are accepted).
func (u *Updater) FetchRelease(ctx context.Context, version string) (*Release, error) {
	if err := u.validate(); err != nil {
		return nil, err
	}
	return u.fetchVersionRelease(ctx, withV(version))
}

// Install downloads, verifies, and atomically replaces exePath with the binary
// from the given release. It returns an error if any step fails.
//
// On Unix, the replacement is performed with os.Rename (atomic on same filesystem).
// On Windows, the running executable is first renamed to exePath+".old", then the
// new binary is moved into place. The .old file is removed best-effort.
func (u *Updater) Install(ctx context.Context, rel *Release, exePath string) error {
	if err := u.validate(); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "selfupdate-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	archivePath := filepath.Join(tmpDir, rel.AssetName)

	if err := download(ctx, u.httpClient(), rel.ChecksumURL, checksumPath); err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	if err := download(ctx, u.httpClient(), rel.DownloadURL, archivePath); err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	if err := verifyChecksum(archivePath, checksumPath); err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}

	binaryName := u.BinaryName
	if runtime.GOOS == "windows" && filepath.Ext(binaryName) == "" {
		binaryName += ".exe"
	}
	extractedPath := filepath.Join(tmpDir, binaryName)
	if err := extractBinary(archivePath, binaryName, extractedPath); err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	if err := atomicReplace(extractedPath, exePath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	return nil
}

// validate checks that the Updater is configured with required fields.
func (u *Updater) validate() error {
	if u.Repo == "" {
		return fmt.Errorf("selfupdate: Repo must be set")
	}
	if u.BinaryName == "" {
		return fmt.Errorf("selfupdate: BinaryName must be set")
	}
	if u.AssetNamer == nil {
		return fmt.Errorf("selfupdate: AssetNamer must be set")
	}
	return nil
}

// assetExtension returns the platform-specific archive extension.
func assetExtension() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// normalizeVersion strips the leading "v" from a version string.
func normalizeVersion(v string) string {
	if len(v) > 0 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

// withV ensures a version string has a leading "v".
func withV(v string) string {
	if len(v) > 0 && v[0] != 'v' {
		return "v" + v
	}
	return v
}
