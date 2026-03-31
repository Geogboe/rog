package selfupdate

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownload_Success(t *testing.T) {
	content := []byte("hello, world!")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	t.Cleanup(srv.Close)

	destPath := filepath.Join(t.TempDir(), "output.bin")
	err := download(testCtx(t), srv.Client(), srv.URL+"/file", destPath)
	require.NoError(t, err)

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDownload_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	destPath := filepath.Join(t.TempDir(), "output.bin")
	err := download(testCtx(t), srv.Client(), srv.URL+"/missing", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestDownload_CreatesFileAtDestination(t *testing.T) {
	content := []byte("binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "sub", "output.bin")
	// Ensure parent directory exists
	require.NoError(t, os.MkdirAll(filepath.Dir(destPath), 0755))

	err := download(testCtx(t), srv.Client(), srv.URL+"/file", destPath)
	require.NoError(t, err)

	info, err := os.Stat(destPath)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), info.Size())
}
