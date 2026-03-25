package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func normalizeScanPath(path string) string {
	if path == "" {
		return ""
	}

	path = filepath.Clean(path)

	if runtime.GOOS != "windows" {
		return path
	}

	volume := filepath.VolumeName(path)
	if len(volume) == 2 && volume[1] == ':' {
		volume = strings.ToUpper(volume[:1]) + ":"
		path = volume + path[len(filepath.VolumeName(path)):]
	}

	// Keep drive roots and UNC share roots intact, but trim redundant separators elsewhere.
	if len(path) > len(volume)+1 {
		path = strings.TrimRight(path, `\/`)
	}

	return path
}

func pathWithinRoot(rootPath, fullPath string) (string, bool) {
	rootPath = normalizeScanPath(rootPath)
	fullPath = normalizeScanPath(fullPath)

	if rootPath == "" || fullPath == "" {
		return "", false
	}

	if runtime.GOOS == "windows" && !sameVolume(rootPath, fullPath) {
		return "", false
	}

	relPath, err := filepath.Rel(rootPath, fullPath)
	if err != nil {
		return "", false
	}

	if relPath == "." {
		return "", true
	}

	relPath = filepath.Clean(relPath)
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", false
	}

	return relPath, true
}

func sameVolume(a, b string) bool {
	av := filepath.VolumeName(a)
	bv := filepath.VolumeName(b)

	if runtime.GOOS == "windows" {
		return strings.EqualFold(av, bv)
	}

	return av == bv
}
