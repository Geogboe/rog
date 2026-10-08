package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/index"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestListNavigationPaths(t *testing.T) {
	for _, tt := range []struct {
		name string
		c    listPathContext
		repo index.Repo
		want string
	}{
		{"backslash", listPathContext{"/work", "/home/test", "", "linux"}, index.Repo{AbsPath: `/home/test/a\b`}, `~/'a\b'`},
		{"control", listPathContext{}, index.Repo{AbsPath: "/projects/a\nb"}, "[path contains control characters]"},
		{"home", listPathContext{"/work", "/home/test", "Ubuntu", "linux"}, index.Repo{AbsPath: "/home/test/dev/rog"}, "~/dev/rog"},
		{"home prefix collision", listPathContext{"/work", "/home/test", "", "linux"}, index.Repo{AbsPath: "/home/tester/dev"}, "/home/tester/dev"},
		{"sibling", listPathContext{"/work/one", "/home/test", "", "linux"}, index.Repo{AbsPath: "/work/two"}, "../two"},
		{"current", listPathContext{"/work/one", "/home/test", "", "linux"}, index.Repo{AbsPath: "/work/one"}, "."},
		{"space", listPathContext{"/work", "/home/test", "", "linux"}, index.Repo{AbsPath: "/home/test/my tool"}, "~/'my tool'"},
		{"apostrophe", listPathContext{"/work", "/home/test", "", "linux"}, index.Repo{AbsPath: "/home/test/it's"}, "~/'it'\\''s'"},
		{"windows home", listPathContext{`C:\work`, `C:\Users\test`, "", "windows"}, index.Repo{AbsPath: `c:\users\TEST\dev\rog`}, "~/dev/rog"},
		{"windows sibling", listPathContext{`C:\work\one`, `C:\Users\test`, "", "windows"}, index.Repo{AbsPath: `C:\work\two`}, "../two"},
		{"other drive", listPathContext{`C:\work`, `C:\Users\test`, "", "windows"}, index.Repo{AbsPath: `D:\work\two`}, "D:/work/two"},
		{"powershell spaces", listPathContext{`C:\work`, `C:\Users\test`, "", "windows"}, index.Repo{AbsPath: `C:\Users\test\my tool`}, "'~/my tool'"},
		{"wsl unc", listPathContext{`C:\work`, `C:\Users\test`, "", "windows"}, index.Repo{AbsPath: `\\wsl$\Ubuntu\home\test\rog`, IsWSL: true, WSLDistro: "Ubuntu"}, "Ubuntu:/home/test/rog"},
		{"same distro UNC", listPathContext{"/work", "/home/test", "Ubuntu", "linux"}, index.Repo{AbsPath: `\\wsl$\Ubuntu\home\test\rog`, IsWSL: true, WSLDistro: "Ubuntu"}, "~/rog"},
		{"same distro", listPathContext{"/work", "/home/test", "Ubuntu", "linux"}, index.Repo{AbsPath: "/home/test/rog", IsWSL: true, WSLDistro: "Ubuntu"}, "~/rog"},
		{"different distro", listPathContext{"/work", "/home/test", "Ubuntu", "linux"}, index.Repo{AbsPath: "/home/test/rog", IsWSL: true, WSLDistro: "Debian"}, "Debian:/home/test/rog"},
	} {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, tt.c.display(&tt.repo)) })
	}
}

func TestListGroupingSeparatesEnvironments(t *testing.T) {
	var out bytes.Buffer
	repos := []*index.Repo{{Name: "a", Root: "projects", AbsPath: "/a"}, {Name: "b", Root: "projects", AbsPath: "/b", IsWSL: true, WSLDistro: "Ubuntu"}, {Name: "c", Root: "projects", AbsPath: "/c", IsWSL: true, WSLDistro: "Debian"}}
	formatter := &listPathFormatter{local: listPathContext{platform: "windows"}, foreign: map[string]*listPathContext{}, lookup: func(context.Context, string) (listPathContext, error) {
		return listPathContext{}, errors.New("unavailable")
	}}
	writeGroupedTable(&out, repos, false, false, nil, 80, false, formatter)
	for _, label := range []string{"projects · local", "projects · WSL Ubuntu", "projects · WSL Debian"} {
		require.Contains(t, out.String(), label)
	}
	require.Equal(t, 3, strings.Count(out.String(), "Total: 1 repositories"))
}

func TestListAllModesKeepPathsOnOneLine(t *testing.T) {
	for _, mode := range []struct {
		short, long bool
		fields      []string
	}{{false, false, nil}, {true, false, nil}, {false, true, nil}, {false, false, []string{"name", "path"}}} {
		var out bytes.Buffer
		writeTable(&out, []*index.Repo{{Name: "tool", AbsPath: "/projects/" + strings.Repeat("long-directory/", 8) + "tool"}}, mode.short, mode.long, mode.fields, 80, false)
		lines := strings.Split(out.String(), "\n")
		require.Len(t, lines, 5)
		require.Contains(t, lines[1], "…")
		require.LessOrEqual(t, ansi.StringWidth(lines[1]), 80)
	}
}

func TestForeignWSLPathsUseTheirOwnShellContext(t *testing.T) {
	calls := map[string]int{}
	formatter := &listPathFormatter{local: listPathContext{cwd: `C:\work`, home: `C:\Users\test`, platform: "windows"}, foreign: map[string]*listPathContext{}, lookup: func(ctx context.Context, distro string) (listPathContext, error) {
		calls[distro]++
		require.NoError(t, ctx.Err())
		if distro == "Missing" {
			return listPathContext{}, errors.New("unavailable")
		}
		home := "/home/ubuntu-user"
		if distro == "Debian" {
			home = "/srv/debian-user"
		}
		return listPathContext{home: home, cwd: "/srv/work/one", platform: "linux"}, nil
	}}
	for _, tt := range []struct{ distro, path, want string }{
		{"Ubuntu", `\\wsl$\Ubuntu\home\ubuntu-user\projects\tool`, "Ubuntu:~/projects/tool"},
		{"Ubuntu", "/srv/work/two", "Ubuntu:../two"},
		{"Ubuntu", "/home/ubuntu-user/my tool", "Ubuntu:~/'my tool'"},
		{"Ubuntu", "/home/ubuntu-user-other/tool", "Ubuntu:/home/ubuntu-user-other/tool"},
		{"Debian", "/srv/debian-user/tool", "Debian:~/tool"},
		{"Missing", "/home/ubuntu-user/tool", "Missing:/home/ubuntu-user/tool"},
		{"Missing", "/srv/work/two", "Missing:/srv/work/two"},
	} {
		require.Equal(t, tt.want, formatter.display(&index.Repo{AbsPath: tt.path, IsWSL: true, WSLDistro: tt.distro}))
	}
	require.Equal(t, map[string]int{"Ubuntu": 1, "Debian": 1, "Missing": 1}, calls)
	// Native rows and machine output never resolve a distribution.
	require.Equal(t, "~/tool", formatter.display(&index.Repo{AbsPath: `C:\Users\test\tool`}))
}

func TestWSLContextLookupIsBoundedAndValidated(t *testing.T) {
	for _, output := range []string{"", "/home/a\x00relative\x00", "/home/a\ninvalid\x00/work\x00", "/home/a\x00/work\x00extra"} {
		_, err := parseListDistroContext(output)
		require.Error(t, err)
	}
	ctx, err := parseListDistroContext("/srv/user home\x00/mount/work\x00")
	require.NoError(t, err)
	require.Equal(t, "/srv/user home", ctx.home)
	require.Equal(t, "/mount/work", ctx.cwd)
	formatter := &listPathFormatter{local: listPathContext{platform: "windows"}, foreign: map[string]*listPathContext{}, deadline: time.Now().Add(-time.Second), lookup: func(context.Context, string) (listPathContext, error) {
		t.Fatal("expired lookup must not start WSL")
		return listPathContext{}, nil
	}}
	require.Equal(t, "Ubuntu:/home/a/tool", formatter.display(&index.Repo{AbsPath: "/home/a/tool", IsWSL: true, WSLDistro: "Ubuntu"}))
}
