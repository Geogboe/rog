package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// download fetches url and writes the response body to destPath.
// The file is created with mode 0600 and replaced atomically via a temp file.
func download(ctx context.Context, client HTTPDoer, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}

	// Write to a temp file first, then rename (atomic on POSIX; fine on Windows too
	// since we're writing a new file, not replacing a running executable).
	tmp, err := os.CreateTemp("", "selfupdate-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // best-effort cleanup if rename fails

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		return fmt.Errorf("move to destination: %w", err)
	}
	return nil
}
