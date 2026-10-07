package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildArchive returns an archive for this platform holding every entry, keyed
// by name, under a top-level directory like GoReleaser's.
func buildArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if testGOOS() == "windows" {
		zw := zip.NewWriter(&buf)
		for name, data := range entries {
			w, err := zw.Create("pkg/" + name)
			require.NoError(t, err)
			_, err = w.Write(data)
			require.NoError(t, err)
		}
		require.NoError(t, zw.Close())
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "pkg/" + name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// installFrom serves archive as release 0.5.0 and installs it over exePath.
func installFrom(t *testing.T, archive []byte, sidecars []string, exePath string) error {
	t.Helper()
	assetName := fmt.Sprintf("rog-0.5.0-%s-%s%s", testGOOS(), testGOARCH(), assetExtension())
	checksums := fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v0.5.0", Assets: []githubAsset{
				{Name: assetName, BrowserDownloadURL: srv.URL + "/download/" + assetName},
				{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/download/checksums.txt"},
			}})
		case "/download/" + assetName:
			_, _ = w.Write(archive)
		case "/download/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	u := &Updater{
		Repo: "owner/repo", BinaryName: "rog", AssetNamer: testNamer, Sidecars: sidecars,
		Client: &prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase},
	}
	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	return u.Install(testCtx(t), rel, exePath)
}

func exeName() string {
	if testGOOS() == "windows" {
		return "rog.exe"
	}
	return "rog"
}

func TestUpdater_Install_ReplacesSidecarsBesideTheBinary(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, exeName())
	require.NoError(t, os.WriteFile(exePath, []byte("old binary"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.dll"), []byte("old dll"), 0o644))

	archive := buildArchive(t, map[string][]byte{exeName(): []byte("new binary"), "helper.dll": []byte("new dll"), "README.md": []byte("docs")})
	require.NoError(t, installFrom(t, archive, []string{"helper.dll"}, exePath))

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("new binary"), got)
	got, err = os.ReadFile(filepath.Join(dir, "helper.dll"))
	require.NoError(t, err)
	assert.Equal(t, []byte("new dll"), got)
	_, err = os.Stat(filepath.Join(dir, "README.md"))
	assert.True(t, os.IsNotExist(err), "only the configured sidecars are installed")
}

// An archive from before the file was shipped still updates the binary and
// leaves an existing copy of the sidecar alone.
func TestUpdater_Install_SidecarMissingFromTheArchiveIsSkipped(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, exeName())
	require.NoError(t, os.WriteFile(exePath, []byte("old binary"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.dll"), []byte("kept dll"), 0o644))

	archive := buildArchive(t, map[string][]byte{exeName(): []byte("new binary")})
	require.NoError(t, installFrom(t, archive, []string{"helper.dll"}, exePath))

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("new binary"), got)
	got, err = os.ReadFile(filepath.Join(dir, "helper.dll"))
	require.NoError(t, err)
	assert.Equal(t, []byte("kept dll"), got)
}

func TestUpdater_Install_WithoutSidecarsLeavesOthersAlone(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, exeName())
	require.NoError(t, os.WriteFile(exePath, []byte("old binary"), 0o755))
	archive := buildArchive(t, map[string][]byte{exeName(): []byte("new binary"), "helper.dll": []byte("new dll")})
	require.NoError(t, installFrom(t, archive, nil, exePath))
	_, err := os.Stat(filepath.Join(dir, "helper.dll"))
	assert.True(t, os.IsNotExist(err))
}

func TestUpdater_Validate_RejectsUnsafeSidecarNames(t *testing.T) {
	for _, name := range []string{"", "../evil.dll", "sub/helper.dll", `sub\helper.dll`, ".", "..", "rog"} {
		u := &Updater{Repo: "owner/repo", BinaryName: "rog", AssetNamer: testNamer, Sidecars: []string{name}}
		_, err := u.CheckLatest(testCtx(t))
		require.Error(t, err, "sidecar %q", name)
		assert.Contains(t, err.Error(), "sidecar", "sidecar %q", name)
	}
}
