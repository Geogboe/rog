package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTarGz(t *testing.T, dir, archiveName, binaryName string, content []byte) string {
	t.Helper()
	archivePath := filepath.Join(dir, archiveName)
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name: binaryName,
		Mode: 0755,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return archivePath
}

func makeZip(t *testing.T, dir, archiveName, binaryName string, content []byte) string {
	t.Helper()
	archivePath := filepath.Join(dir, archiveName)
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	zw := zip.NewWriter(f)
	fw, err := zw.Create(binaryName)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return archivePath
}

func TestExtractBinary_TarGz(t *testing.T) {
	content := []byte("binary-content")
	tmpDir := t.TempDir()
	archivePath := makeTarGz(t, tmpDir, "rog-0.5.0-linux-amd64.tar.gz", "rog", content)

	destPath := filepath.Join(tmpDir, "rog-extracted")
	err := extractBinary(archivePath, "rog", destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestExtractBinary_Zip(t *testing.T) {
	content := []byte("binary-content-windows")
	tmpDir := t.TempDir()
	archivePath := makeZip(t, tmpDir, "rog-0.5.0-windows-amd64.zip", "rog.exe", content)

	destPath := filepath.Join(tmpDir, "rog-extracted.exe")
	err := extractBinary(archivePath, "rog.exe", destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestExtractBinary_TarGz_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := makeTarGz(t, tmpDir, "archive.tar.gz", "other-file", []byte("data"))

	destPath := filepath.Join(tmpDir, "rog")
	err := extractBinary(archivePath, "rog", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExtractBinary_Zip_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := makeZip(t, tmpDir, "archive.zip", "other.exe", []byte("data"))

	destPath := filepath.Join(tmpDir, "rog.exe")
	err := extractBinary(archivePath, "rog.exe", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExtractBinary_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.bz2")
	require.NoError(t, os.WriteFile(archivePath, []byte("data"), 0644))

	err := extractBinary(archivePath, "rog", filepath.Join(tmpDir, "rog"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive format")
}

func TestExtractBinary_TarGz_NestedDirectory(t *testing.T) {
	content := []byte("binary-content")
	tmpDir := t.TempDir()

	// Create archive with binary inside a subdirectory
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	f, err := os.Create(archivePath)
	require.NoError(t, err)

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: "rog-0.5.0-linux-amd64/rog", // nested path
		Mode: 0755,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	f.Close()

	destPath := filepath.Join(tmpDir, "rog")
	err = extractBinary(archivePath, "rog", destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}
