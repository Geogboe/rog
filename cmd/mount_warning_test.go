package cmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/Geogboe/rog/internal/config"
)

func TestWindowsMountForPath(t *testing.T) {
	mounts := "1 0 0:1 / / rw - ext4 /dev/root rw\n2 1 0:2 / /mnt/c ro - 9p C: ro,aname=drvfs;path=C:\\;uid=1000\n3 1 0:3 / /mnt/other rw - ext4 /dev/other rw\n"
	if got := windowsMountForPath(mounts, "/mnt/c/projects/repo"); got != "/mnt/c" {
		t.Fatalf("mount = %q", got)
	}
	if got := windowsMountForPath(mounts, "/mnt/other/repo"); got != "" {
		t.Fatalf("unrelated mount = %q", got)
	}
}

func TestMountWarningSuppression(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux mountinfo test")
	}
	var out bytes.Buffer
	warnMountedWindowsRoots(&config.Config{Roots: []config.Root{{Name: "mounted", Path: "/mnt/c"}}}, &out)
	if !strings.Contains(out.String(), "mounted") {
		t.Skip("test host has no WSL DrvFs mount")
	}
	out.Reset()
	warnMountedWindowsRoots(&config.Config{Scan: &config.ScanConfig{SuppressMountWarnings: true}, Roots: []config.Root{{Name: "mounted", Path: "/mnt/c"}}}, &out)
	if out.Len() != 0 {
		t.Fatalf("suppressed warning: %s", out.String())
	}
}
