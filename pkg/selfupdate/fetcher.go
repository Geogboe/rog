package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

const githubAPIBase = "https://api.github.com"

// ErrNoStableRelease indicates GitHub's /releases/latest endpoint returned
// 404. That endpoint excludes every release marked prerelease or draft, so
// this is the expected result when a repository has releases but none of
// them are stable — set Updater.AllowPrereleaseAndDraft to fetch the newest
// release regardless of its prerelease/draft flags. A 404 here can also
// have other causes (a mistyped Repo, a private repo with no Token); errors
// wrapping this sentinel retain the underlying error, including the request
// URL, so those cases stay distinguishable and debuggable.
var ErrNoStableRelease = errors.New("no stable release found")

// errReleaseNotFound is the internal sentinel for a 404 from the GitHub API.
// fetchLatestRelease translates it into the more actionable ErrNoStableRelease;
// fetchVersionRelease (a pinned-tag lookup) leaves it as-is, since "no stable
// release" doesn't apply to a request for one specific tag.
var errReleaseNotFound = errors.New("release not found")

// githubRelease is the JSON structure from the GitHub Releases API.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset is an individual asset within a GitHub release.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// fetchLatestRelease fetches the latest stable (non-prerelease, non-draft)
// release from GitHub via the /releases/latest endpoint.
func (u *Updater) fetchLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, u.Repo)
	body, err := u.doReleaseRequest(ctx, url)
	if err != nil {
		if errors.Is(err, errReleaseNotFound) {
			// Wrap both the sentinel (for errors.Is) and the original error
			// (which carries the endpoint URL) so a 404 caused by something
			// other than "every release is prerelease/draft" — a typo'd
			// Repo, a private repo with no auth — stays debuggable.
			return nil, fmt.Errorf("%w: %w", ErrNoStableRelease, err)
		}
		return nil, err
	}

	var ghRel githubRelease
	if err := json.Unmarshal(body, &ghRel); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}
	return u.resolveRelease(&ghRel)
}

// fetchLatestReleaseIncludingPrerelease fetches the single newest release for
// the repository regardless of its prerelease/draft flags, using the release
// list endpoint (sorted newest-first) rather than /releases/latest.
func (u *Updater) fetchLatestReleaseIncludingPrerelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=1", githubAPIBase, u.Repo)
	body, err := u.doReleaseRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var ghRels []githubRelease
	if err := json.Unmarshal(body, &ghRels); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}
	if len(ghRels) == 0 {
		return nil, fmt.Errorf("%s: no releases found", u.Repo)
	}
	return u.resolveRelease(&ghRels[0])
}

// fetchVersionRelease fetches a specific tagged release from GitHub.
// version must include the leading "v" (e.g. "v0.5.0").
func (u *Updater) fetchVersionRelease(ctx context.Context, version string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPIBase, u.Repo, version)
	body, err := u.doReleaseRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var ghRel githubRelease
	if err := json.Unmarshal(body, &ghRel); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}
	return u.resolveRelease(&ghRel)
}

// doReleaseRequest issues an authenticated GET against the GitHub API and
// returns the raw response body for the caller to decode — either a single
// release object or a release list, depending on the endpoint.
func (u *Updater) doReleaseRequest(ctx context.Context, apiURL string) ([]byte, error) {
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
		return nil, fmt.Errorf("%w at %s", errReleaseNotFound, apiURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
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
