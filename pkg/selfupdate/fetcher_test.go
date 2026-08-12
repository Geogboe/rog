package selfupdate

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNamer(v, goos, goarch string) string {
	return "rog-" + v + "-" + goos + "-" + goarch
}

func newTestUpdater(client HTTPDoer) *Updater {
	return &Updater{
		Repo:       "owner/repo",
		BinaryName: "rog",
		AssetNamer: testNamer,
		Client:     client,
	}
}

// mockReleaseServer returns an httptest.Server that serves a fake GitHub release.
// assetName should be the full asset filename (e.g. "rog-0.5.0-linux-amd64.tar.gz").
func mockReleaseServer(t *testing.T, tagName, assetName string) *httptest.Server {
	t.Helper()
	baseURL := "" // set after server creation; workaround via pointer
	var srv *httptest.Server

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := githubRelease{
			TagName: tagName,
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: srv.URL + "/download/" + assetName},
				{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/download/checksums.txt"},
			},
		}
		_ = baseURL // suppress "unused" warning
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchLatestRelease_Success(t *testing.T) {
	assetName := "rog-0.5.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := mockReleaseServer(t, "v0.5.0", assetName)

	u := newTestUpdater(srv.Client())
	u.Client = &prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase}

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	assert.Equal(t, "v0.5.0", rel.Version)
	assert.Equal(t, assetName, rel.AssetName)
	assert.NotEmpty(t, rel.DownloadURL)
	assert.NotEmpty(t, rel.ChecksumURL)
}

func TestFetchLatestRelease_TokenInHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		assetName := "rog-0.5.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
		rel := githubRelease{
			TagName: "v0.5.0",
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: "http://example.com/" + assetName},
				{Name: "checksums.txt", BrowserDownloadURL: "http://example.com/checksums.txt"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	u.Token = "my-secret-token"

	_, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", gotAuth)
}

func TestFetchLatestRelease_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoStableRelease), "expected ErrNoStableRelease, got: %v", err)
}

// TestCheckLatest_AllowPrerelease_UsesReleaseListEndpoint verifies that
// Updater.AllowPrerelease switches CheckLatest to the /releases list endpoint
// (which includes prereleases and drafts) instead of /releases/latest (which
// does not), and that it takes the first (newest) entry in the list.
func TestCheckLatest_AllowPrerelease_UsesReleaseListEndpoint(t *testing.T) {
	assetName := "rog-0.6.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.URL.Path == "/repos/owner/repo/releases/latest" {
			t.Fatal("AllowPrerelease should not hit /releases/latest")
		}
		rels := []githubRelease{
			{
				TagName: "v0.6.0",
				Assets: []githubAsset{
					{Name: assetName, BrowserDownloadURL: "http://example.com/" + assetName},
					{Name: "checksums.txt", BrowserDownloadURL: "http://example.com/checksums.txt"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rels)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	u.AllowPrerelease = true

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	assert.Equal(t, "v0.6.0", rel.Version)
	assert.Equal(t, "/repos/owner/repo/releases", gotPath)
	assert.Equal(t, "per_page=1", gotQuery)
}

// TestCheckLatest_AllowPrerelease_EmptyList verifies a clear error when the
// repository has no releases at all (as opposed to only non-stable ones).
func TestCheckLatest_AllowPrerelease_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]githubRelease{})
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	u.AllowPrerelease = true

	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no releases found")
}

// TestCheckLatest_AllowPrerelease_False_StillUsesLatestEndpoint is a
// regression guard: the zero value (AllowPrerelease unset) must keep hitting
// /releases/latest, preserving existing stable-only behavior for callers who
// don't opt in.
func TestCheckLatest_AllowPrerelease_False_StillUsesLatestEndpoint(t *testing.T) {
	assetName := "rog-0.5.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := mockReleaseServer(t, "v0.5.0", assetName)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	assert.Equal(t, "v0.5.0", rel.Version)
}

func TestFetchLatestRelease_AssetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := githubRelease{
			TagName: "v0.5.0",
			Assets: []githubAsset{
				{Name: "other-tool.tar.gz", BrowserDownloadURL: "http://example.com/other-tool.tar.gz"},
				{Name: "checksums.txt", BrowserDownloadURL: "http://example.com/checksums.txt"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no asset")
}

func TestFetchLatestRelease_NoChecksums(t *testing.T) {
	assetName := "rog-0.5.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := githubRelease{
			TagName: "v0.5.0",
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: "http://example.com/" + assetName},
				// no checksums.txt
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksums.txt")
}

func TestFetchVersionRelease_PinnedVersion(t *testing.T) {
	assetName := "rog-0.4.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "v0.4.0")
		rel := githubRelease{
			TagName: "v0.4.0",
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: "http://example.com/" + assetName},
				{Name: "checksums.txt", BrowserDownloadURL: "http://example.com/checksums.txt"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	rel, err := u.FetchRelease(testCtx(t), "v0.4.0")
	require.NoError(t, err)
	assert.Equal(t, "v0.4.0", rel.Version)
	assert.Equal(t, assetName, rel.AssetName)
}

func TestFetchVersionRelease_NormalizesVersion(t *testing.T) {
	assetName := "rog-0.4.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should always request with "v" prefix
		assert.Contains(t, r.URL.Path, "v0.4.0")
		rel := githubRelease{
			TagName: "v0.4.0",
			Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: "http://example.com/" + assetName},
				{Name: "checksums.txt", BrowserDownloadURL: "http://example.com/checksums.txt"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	}))
	t.Cleanup(srv.Close)

	u := newTestUpdater(&prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase})
	// Pass without "v" prefix
	rel, err := u.FetchRelease(testCtx(t), "0.4.0")
	require.NoError(t, err)
	assert.Equal(t, "v0.4.0", rel.Version)
}
