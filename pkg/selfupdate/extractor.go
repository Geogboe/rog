package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// entryNotFoundError reports that an archive has no entry with the requested
// name, as opposed to the archive being unreadable.
type entryNotFoundError struct{ msg string }

func (e *entryNotFoundError) Error() string { return e.msg }

// extractOptional extracts name like extractBinary, but reports a name that is
// not in the archive as found=false instead of an error. Any other failure is
// still an error.
func extractOptional(archivePath, name, destPath string) (found bool, err error) {
	err = extractBinary(archivePath, name, destPath)
	var missing *entryNotFoundError
	if errors.As(err, &missing) {
		return false, nil
	}
	return err == nil, err
}

// extractBinary extracts the file named binaryName from the archive at archivePath
// and writes it to destPath. The archive format is determined from the filename extension.
func extractBinary(archivePath, binaryName, destPath string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return extractFromTarGz(archivePath, binaryName, destPath)
	case strings.HasSuffix(lower, ".zip"):
		return extractFromZip(archivePath, binaryName, destPath)
	default:
		return fmt.Errorf("unsupported archive format: %s", filepath.Ext(archivePath))
	}
}

// extractFromTarGz extracts binaryName from a .tar.gz archive.
func extractFromTarGz(archivePath, binaryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Match by base name to handle subdirectories in the archive.
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		return writeExtracted(tr, destPath, hdr.FileInfo().Mode())
	}
	return &entryNotFoundError{fmt.Sprintf("binary %q not found in archive", binaryName)}
}

// extractFromZip extracts binaryName from a .zip archive.
func extractFromZip(archivePath, binaryName, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) != binaryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		defer rc.Close()
		return writeExtracted(rc, destPath, f.Mode())
	}
	return &entryNotFoundError{fmt.Sprintf("binary %q not found in zip archive", binaryName)}
}

// writeExtracted writes data from r into destPath with the given mode.
func writeExtracted(r io.Reader, destPath string, mode os.FileMode) error {
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}
	return nil
}
