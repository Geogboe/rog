// Package workerbundle contains the Linux scanner distributed with Windows rog.
package workerbundle

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

//go:embed linux_amd64.gz linux_amd64.sha256
var files embed.FS

// LinuxAMD64 returns the bundled executable and a build ID derived from its
// content. The checksum is checked before each attempted installation.
func LinuxAMD64() ([]byte, string, error) {
	compressed, err := files.ReadFile("linux_amd64.gz")
	if err != nil {
		return nil, "", err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, 32<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 32<<20 {
		return nil, "", fmt.Errorf("WSL worker exceeds 32 MiB limit")
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	expected, err := files.ReadFile("linux_amd64.sha256")
	if err != nil {
		return nil, "", err
	}
	if actual != strings.TrimSpace(string(expected)) {
		return nil, "", fmt.Errorf("bundled WSL worker checksum mismatch")
	}
	return data, actual, nil
}
