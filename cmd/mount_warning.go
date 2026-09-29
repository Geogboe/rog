package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Geogboe/rog/internal/config"
)

func warnMountedWindowsRoots(cfg *config.Config, out io.Writer) {
	if runtime.GOOS != "linux" || cfg.Scan != nil && cfg.Scan.SuppressMountWarnings {
		return
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return
	}
	for _, root := range cfg.Roots {
		if root.Windows || root.WSL {
			continue
		}
		resolved, err := filepath.EvalSymlinks(root.Path)
		if err != nil {
			continue
		}
		if mount := windowsMountForPath(string(data), resolved); mount != "" {
			fmt.Fprintf(out, "Warning: Configured Root %q walks Windows mount %s; Git reads may be slow. Use a Windows root with windows: true to scan on Windows, or set scan.suppress_mount_warnings: true.\n", root.Name, mount)
		}
	}
}

func windowsMountForPath(mountinfo, target string) string {
	best := ""
	lines := bufio.NewScanner(strings.NewReader(mountinfo))
	for lines.Scan() {
		parts := strings.SplitN(lines.Text(), " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 3 {
			continue
		}
		if right[0] != "drvfs" && !(right[0] == "9p" && strings.Contains(right[2], "aname=drvfs")) {
			continue
		}
		mount := strings.ReplaceAll(strings.ReplaceAll(left[4], `\040`, " "), `\134`, `\`)
		if (target == mount || strings.HasPrefix(target, strings.TrimRight(mount, "/")+"/")) && len(mount) > len(best) {
			best = mount
		}
	}
	return best
}
