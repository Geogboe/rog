package selfupdate

import (
	"archive/tar"
	"archive/zip"
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
func TestUpdater_Validate_MissingRepo(t *testing.T) {
	u := &Updater{BinaryName: "rog", AssetNamer: testNamer}
	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Repo must be set")
}

func TestUpdater_Validate_MissingAssetNamer(t *testing.T) {
	u := &Updater{Repo: "owner/repo", BinaryName: "rog"}
	_, err := u.CheckLatest(testCtx(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AssetNamer must be set")
}

func TestUpdater_CheckLatest_ReturnsRelease(t *testing.T) {
	assetName := "rog-0.5.0-" + testGOOS() + "-" + testGOARCH() + assetExtension()
	srv := mockReleaseServer(t, "v0.5.0", assetName)

	u := &Updater{
		Repo:       "owner/repo",
		BinaryName: "rog",
		AssetNamer: testNamer,
		Client:     &prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase},
	}

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)
	assert.Equal(t, "v0.5.0", rel.Version)
	assert.Equal(t, assetName, rel.AssetName)
	assert.NotEmpty(t, rel.DownloadURL)
	assert.NotEmpty(t, rel.ChecksumURL)
}

// TestUpdater_Install_FullPipeline runs the full download→verify→extract→replace pipeline
// against an in-memory test server, exercising all components end-to-end.
func TestUpdater_Install_FullPipeline(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho hello")
	goos := testGOOS()
	goarch := testGOARCH()
	version := "0.5.0"
	assetName := fmt.Sprintf("rog-%s-%s-%s%s", version, goos, goarch, assetExtension())
	bin := "rog"
	if goos == "windows" {
		bin = "rog.exe"
	}

	// Build archive in the format for this platform.
	var archiveData []byte
	if goos == "windows" {
		archiveData = buildZip(t, bin, binaryContent)
	} else {
		archiveData = buildTarGz(t, bin, binaryContent)
	}

	// Compute its SHA256 for the checksums file.
	checksumHex := sha256Hex(archiveData)
	checksumsContent := fmt.Sprintf("%s  %s\n", checksumHex, assetName)

	// Use a closure so the handler can reference srv.URL for download URLs.
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			rel := githubRelease{
				TagName: "v" + version,
				Assets: []githubAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/download/" + assetName},
					{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/download/checksums.txt"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
		case "/download/" + assetName:
			w.Write(archiveData)
		case "/download/checksums.txt":
			w.Write([]byte(checksumsContent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	// Set up a fake "current executable" in a temp dir.
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, bin)
	require.NoError(t, os.WriteFile(exePath, []byte("old binary"), 0755))

	u := &Updater{
		Repo:       "owner/repo",
		BinaryName: "rog",
		AssetNamer: testNamer,
		Client:     &prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase},
	}

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)

	err = u.Install(testCtx(t), rel, exePath)
	require.NoError(t, err)

	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, got)
}

func TestUpdater_Install_ChecksumMismatch_Fails(t *testing.T) {
	goos := testGOOS()
	goarch := testGOARCH()
	assetName := fmt.Sprintf("rog-0.5.0-%s-%s%s", goos, goarch, assetExtension())
	bin := "rog"
	if goos == "windows" {
		bin = "rog.exe"
	}
	archiveData := buildTarGz(t, bin, []byte("binary"))
	if goos == "windows" {
		archiveData = buildZip(t, bin, []byte("binary"))
	}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			rel := githubRelease{
				TagName: "v0.5.0",
				Assets: []githubAsset{
					{Name: assetName, BrowserDownloadURL: srv.URL + "/download/" + assetName},
					{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/download/checksums.txt"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
		case "/download/" + assetName:
			w.Write(archiveData)
		case "/download/checksums.txt":
			// Wrong checksum
			w.Write([]byte("deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  " + assetName + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, bin)
	require.NoError(t, os.WriteFile(exePath, []byte("old binary"), 0755))

	u := &Updater{
		Repo:       "owner/repo",
		BinaryName: "rog",
		AssetNamer: testNamer,
		Client:     &prefixedClient{base: srv.Client(), urlPrefix: srv.URL, apiBase: githubAPIBase},
	}

	rel, err := u.CheckLatest(testCtx(t))
	require.NoError(t, err)

	err = u.Install(testCtx(t), rel, exePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")

	// Original binary should be untouched.
	got, err := os.ReadFile(exePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("old binary"), got)
}

// buildTarGz creates a tar.gz archive in memory containing one file named name with content.
func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")

	f, err := os.Create(archivePath)
	require.NoError(t, err)

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	return data
}

// buildZip creates an in-memory zip archive containing one file named name with content.
func buildZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.zip")

	f, err := os.Create(archivePath)
	require.NoError(t, err)

	zw := zip.NewWriter(f)
	fw, err := zw.Create(name)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	return data
}
func sha256Hex(data []byte) string {
	tmp, err := os.CreateTemp("", "sha256-*")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write(data)
	tmp.Close()
	result, err := sha256File(tmp.Name())
	if err != nil {
		panic(err)
	}
	return result
}
