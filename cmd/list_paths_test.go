package cmd

import (
	"bytes"
	"strings"
	"testing"

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
	writeGroupedTable(&out, repos, false, false, nil, 80, false)
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
