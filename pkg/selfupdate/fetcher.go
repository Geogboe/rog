package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

const githubAPIBase = "https://api.github.com"

// githubRelease is the JSON structure from the GitHub Releases API.
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []githubAsset  `json:"assets"`
}

// githubAsset is an individual asset within a GitHub release.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// fetchLatestRelease fetches the latest release from GitHub.
func (u *Updater) fetchLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, u.Repo)
	return u.fetchReleaseFromURL(ctx, url)
}

// fetchVersionRelease fetches a specific tagged release from GitHub.
// version must include the leading "v" (e.g. "v0.5.0").
func (u *Updater) fetchVersionRelease(ctx context.Context, version string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPIBase, u.Repo, version)
	return u.fetchReleaseFromURL(ctx, url)
}

// fetchReleaseFromURL fetches and parses a GitHub release from the given API URL,
// then resolves the asset and checksum URLs for the current platform.
func (u *Updater) fetchReleaseFromURL(ctx context.Context, apiURL string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}

	resp, err := u.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("release not found at %s", apiURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var ghRel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRel); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}

	return u.resolveRelease(&ghRel)
}

// resolveRelease matches the current platform's asset within a GitHub release.
func (u *Updater) resolveRelease(ghRel *githubRelease) (*Release, error) {
	version := ghRel.TagName
	versionNum := normalizeVersion(version)
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	baseName := u.AssetNamer(versionNum, goos, goarch)
	assetName := baseName + assetExtension()

	var downloadURL, checksumURL string
	for _, asset := range ghRel.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = asset.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		available := make([]string, 0, len(ghRel.Assets))
		for _, a := range ghRel.Assets {
			available = append(available, a.Name)
		}
		return nil, fmt.Errorf(
			"no asset %q found in release %s (available: %s)",
			assetName, version, strings.Join(available, ", "),
		)
	}
	if checksumURL == "" {
		return nil, fmt.Errorf("no checksums.txt found in release %s", version)
	}

	return &Release{
		Version:     version,
		AssetName:   assetName,
		DownloadURL: downloadURL,
		ChecksumURL: checksumURL,
	}, nil
}
