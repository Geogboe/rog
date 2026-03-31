package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyChecksum_Valid(t *testing.T) {
	content := []byte("binary content")
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "rog-0.5.0-linux-amd64.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, content, 0644))

	h := sha256.Sum256(content)
	checksum := hex.EncodeToString(h[:])

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	checksumContent := fmt.Sprintf("%s  rog-0.5.0-linux-amd64.tar.gz\n", checksum)
	require.NoError(t, os.WriteFile(checksumPath, []byte(checksumContent), 0644))

	err := verifyChecksum(archivePath, checksumPath)
	require.NoError(t, err)
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	content := []byte("binary content")
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "rog-0.5.0-linux-amd64.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, content, 0644))

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	checksumContent := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  rog-0.5.0-linux-amd64.tar.gz\n"
	require.NoError(t, os.WriteFile(checksumPath, []byte(checksumContent), 0644))

	err := verifyChecksum(archivePath, checksumPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestVerifyChecksum_MissingEntry(t *testing.T) {
	content := []byte("binary content")
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "rog-0.5.0-linux-amd64.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, content, 0644))

	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	// File exists but doesn't mention our archive
	checksumContent := "deadbeef  some-other-file.tar.gz\n"
	require.NoError(t, os.WriteFile(checksumPath, []byte(checksumContent), 0644))

	err := verifyChecksum(archivePath, checksumPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no checksum entry")
}

func TestVerifyChecksum_CaseInsensitive(t *testing.T) {
	content := []byte("binary content")
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "rog-0.5.0-linux-amd64.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, content, 0644))

	h := sha256.Sum256(content)
	// Use uppercase hex
	checksum := hex.EncodeToString(h[:])
	upperChecksum := fmt.Sprintf("%X", h[:])

	_ = checksum
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	checksumContent := fmt.Sprintf("%s  rog-0.5.0-linux-amd64.tar.gz\n", upperChecksum)
	require.NoError(t, os.WriteFile(checksumPath, []byte(checksumContent), 0644))

	err := verifyChecksum(archivePath, checksumPath)
	require.NoError(t, err)
}
