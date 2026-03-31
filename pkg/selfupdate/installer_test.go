package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicReplace_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "new-binary")
	destPath := filepath.Join(tmpDir, "current-binary")

	require.NoError(t, os.WriteFile(srcPath, []byte("new version"), 0755))
	require.NoError(t, os.WriteFile(destPath, []byte("old version"), 0755))

	err := atomicReplace(srcPath, destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("new version"), got)

	// Source should no longer exist (was moved)
	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err), "source file should have been moved")
}

func TestAtomicReplace_DestNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "new-binary")
	destPath := filepath.Join(tmpDir, "nonexistent-dir", "current-binary")

	require.NoError(t, os.WriteFile(srcPath, []byte("new version"), 0755))

	// Should fail because the destination directory doesn't exist
	err := atomicReplace(srcPath, destPath)
	require.Error(t, err)
}

func TestAtomicReplace_PreservesContent(t *testing.T) {
	content := []byte{0x7f, 0x45, 0x4c, 0x46} // ELF magic bytes
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "binary")
	destPath := filepath.Join(tmpDir, "installed")

	require.NoError(t, os.WriteFile(srcPath, content, 0755))
	require.NoError(t, os.WriteFile(destPath, []byte("placeholder"), 0755))

	err := atomicReplace(srcPath, destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}
