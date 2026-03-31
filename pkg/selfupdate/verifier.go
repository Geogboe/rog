package selfupdate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// verifyChecksum reads checksums.txt and verifies that the SHA256 of archivePath
// matches the recorded checksum for that filename.
//
// checksums.txt format (same as GoReleaser / sha256sum output):
//
//	<hex-sha256>  <filename>
func verifyChecksum(archivePath, checksumsPath string) error {
	filename := filepath.Base(archivePath)

	expected, err := findChecksum(checksumsPath, filename)
	if err != nil {
		return err
	}

	actual, err := sha256File(archivePath)
	if err != nil {
		return fmt.Errorf("hash %s: %w", filename, err)
	}

	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  got:      %s", filename, expected, actual)
	}
	return nil
}

// findChecksum searches checksums.txt for the entry matching filename and returns its hex SHA256.
func findChecksum(checksumsPath, filename string) (string, error) {
	f, err := os.Open(checksumsPath)
	if err != nil {
		return "", fmt.Errorf("open checksums file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "<hash>  <filename>" (two spaces, as produced by sha256sum / GoReleaser)
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return strings.ToLower(parts[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums file: %w", err)
	}
	return "", fmt.Errorf("no checksum entry for %q in checksums.txt", filename)
}

// sha256File computes the hex-encoded SHA256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
