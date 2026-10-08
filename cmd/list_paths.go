package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/wsl"
)

// listPathContext describes the shell environment owning a path.
type listPathContext struct{ cwd, home, distro, platform string }

func currentListPathContext() listPathContext {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return listPathContext{cwd, home, os.Getenv("WSL_DISTRO_NAME"), runtime.GOOS}
}

func (c listPathContext) display(repo *index.Repo) string {
	return c.displayWithForeign(repo, nil)
}

func (c listPathContext) displayWithForeign(repo *index.Repo, remote *listPathContext) string {
	p := repo.AbsPath
	if strings.IndexFunc(p, unicode.IsControl) >= 0 {
		return "[path contains control characters]"
	}
	if p == "" {
		return "-"
	}
	foreign := repo.IsWSL && (c.platform == "windows" || repo.WSLDistro != c.distro)
	if repo.IsWSL {
		// Windows indexes may store a WSL UNC path; display the distro's own path.
		normalized := strings.ReplaceAll(p, `\`, "/")
		for _, prefix := range []string{"//wsl.localhost/", "//wsl$/"} {
			if strings.HasPrefix(strings.ToLower(normalized), prefix) {
				rest := normalized[len(prefix):]
				if i := strings.IndexByte(rest, '/'); i >= 0 {
					p = rest[i:]
				}
			}
		}
	}
	if foreign {
		if remote != nil {
			p = shortListPath(p, remote.cwd, remote.home, "linux")
		}
		return distroLabel(repo.WSLDistro) + ":" + quoteListPath(p, "linux")
	}
	return quoteListPath(shortListPath(p, c.cwd, c.home, c.platform), c.platform)
}

func distroLabel(distro string) string {
	if distro == "" {
		return "WSL"
	}
	return distro
}

func shortListPath(p, cwd, home, platform string) string {
	// filepath semantics must match the filesystem owning the displayed path.
	if platform == "windows" {
		p, cwd, home = strings.ReplaceAll(p, `\`, "/"), strings.ReplaceAll(cwd, `\`, "/"), strings.ReplaceAll(home, `\`, "/")
	}
	equal := func(a, b string) bool {
		if platform == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	within := func(base string) (string, bool) {
		base = strings.TrimRight(base, "/\\")
		if base == "" {
			return "", false
		}
		if equal(p, base) {
			return "", true
		}
		if len(p) > len(base) && equal(p[:len(base)], base) && (p[len(base)] == '/' || platform == "windows" && p[len(base)] == '\\') {
			return p[len(base)+1:], true
		}
		return "", false
	}
	if suffix, ok := within(home); ok {
		if suffix == "" {
			return "~"
		}
		return "~/" + suffix
	}
	var rel string
	if platform == "windows" {
		// Do not relativize between drives or UNC shares.
		volume := func(s string) string {
			if strings.HasPrefix(s, "//") {
				parts := strings.Split(strings.TrimPrefix(s, "//"), "/")
				if len(parts) >= 2 {
					return "//" + parts[0] + "/" + parts[1]
				}
			}
			if len(s) >= 2 && s[1] == ':' {
				return s[:2]
			}
			return ""
		}
		if cwd != "" && volume(p) != "" && equal(volume(p), volume(cwd)) {
			a, b := strings.Split(path.Clean(cwd), "/"), strings.Split(path.Clean(p), "/")
			i := 0
			for i < len(a) && i < len(b) && equal(a[i], b[i]) {
				i++
			}
			pieces := make([]string, 0)
			for j := i; j < len(a); j++ {
				pieces = append(pieces, "..")
			}
			pieces = append(pieces, b[i:]...)
			rel = strings.Join(pieces, "/")
			if rel == "" {
				rel = "."
			}
		}
	} else if cwd != "" {
		rel, _ = filepath.Rel(cwd, p)
	}
	parents := 0
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			parents++
		}
	}
	if rel != "" && parents <= 2 && len(rel) < len(p) {
		if rel != "." && !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		return rel
	}
	return p
}

// Quote shell metacharacters, leaving home expansion outside POSIX quotes.
func quoteListPath(p, platform string) string {
	safe := true
	for _, r := range p {
		if r == '\\' && platform != "windows" {
			safe = false
			break
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("/\\._-:~", r)) {
			safe = false
			break
		}
	}
	if safe {
		return p
	}
	if platform == "windows" {
		return "'" + strings.ReplaceAll(p, "'", "''") + "'"
	}
	if strings.HasPrefix(p, "~/") {
		return "~/" + "'" + strings.ReplaceAll(p[2:], "'", "'\\''") + "'"
	}
	return "'" + strings.ReplaceAll(p, "'", "'\\''") + "'"
}

// Cache successes and failures for this listing only. Nothing is written to disk.
type listPathFormatter struct {
	local    listPathContext
	lookup   func(context.Context, string) (listPathContext, error)
	foreign  map[string]*listPathContext
	deadline time.Time
}

func newListPathFormatter() *listPathFormatter {
	return &listPathFormatter{local: currentListPathContext(), lookup: lookupListDistroContext, foreign: map[string]*listPathContext{}}
}

func (f *listPathFormatter) display(repo *index.Repo) string {
	if !repo.IsWSL || (f.local.platform != "windows" && repo.WSLDistro == f.local.distro) || repo.WSLDistro == "" {
		return f.local.display(repo)
	}
	remote, known := f.foreign[repo.WSLDistro]
	if !known {
		// Bound all distro lookups together, including a failed or slow startup.
		if f.deadline.IsZero() {
			f.deadline = time.Now().Add(2 * time.Second)
		}
		if time.Now().Before(f.deadline) {
			ctx, cancel := context.WithDeadline(context.Background(), f.deadline)
			resolved, err := f.lookup(ctx, repo.WSLDistro)
			cancel()
			if err == nil {
				remote = &resolved
			}
		}
		f.foreign[repo.WSLDistro] = remote
	}
	return f.local.displayWithForeign(repo, remote)
}

func lookupListDistroContext(ctx context.Context, distro string) (listPathContext, error) {
	// --exec does not load the user's shell profile. WSL supplies the distro's
	// default user's home and translates the inherited working directory itself.
	command := wsl.ExecInDistroContext(ctx, distro, "sh", "-c", `printf '%s\000%s\000' "$HOME" "$PWD"`)
	command.WaitDelay = 100 * time.Millisecond
	out, err := command.Output()
	if err != nil {
		return listPathContext{}, err
	}
	return parseListDistroContext(string(out))
}

func parseListDistroContext(output string) (listPathContext, error) {
	parts := strings.Split(output, "\x00")
	if len(parts) != 3 || parts[2] != "" {
		return listPathContext{}, fmt.Errorf("invalid WSL path context")
	}
	for _, p := range parts[:2] {
		if !strings.HasPrefix(p, "/") || strings.IndexFunc(p, unicode.IsControl) >= 0 {
			return listPathContext{}, fmt.Errorf("invalid WSL directory")
		}
	}
	return listPathContext{home: parts[0], cwd: parts[1], platform: "linux"}, nil
}
