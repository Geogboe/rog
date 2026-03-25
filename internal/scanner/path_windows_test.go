package scanner

import (
	"runtime"
	"testing"
)

func TestNormalizeScanPathWindowsDriveCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path normalization")
	}

	got := normalizeScanPath(`d:\Projects\Repo\`)
	want := `D:\Projects\Repo`

	if got != want {
		t.Fatalf("normalizeScanPath() = %q, want %q", got, want)
	}
}

func TestPathWithinRootWindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path matching")
	}

	root := `D:\Projects`
	path := `d:\projects\rog`

	rel, ok := pathWithinRoot(root, path)
	if !ok {
		t.Fatalf("expected %q to be within %q", path, root)
	}
	if rel != `rog` {
		t.Fatalf("pathWithinRoot() rel = %q, want %q", rel, `rog`)
	}
}

func TestPathWithinRootWindowsDifferentDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path matching")
	}

	root := `C:\Projects`
	path := `D:\Projects\rog`

	rel, ok := pathWithinRoot(root, path)
	if ok {
		t.Fatalf("expected %q to be outside %q, rel=%q", path, root, rel)
	}
}
