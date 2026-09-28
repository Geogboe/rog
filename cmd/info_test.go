package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Geogboe/rog/internal/index"
)

func TestInfoColorPreservesPlainContent(t *testing.T) {
	repo := &index.Repo{Name: "sample", AbsPath: "/projects/sample", Root: "projects", CurrentBranch: "main", IsDirty: true}
	var plain, colored bytes.Buffer
	writeInfo(&plain, repo, false)
	writeInfo(&colored, repo, true)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("plain output contains ANSI escapes: %q", plain.String())
	}
	if !strings.Contains(colored.String(), "\x1b[1;36msample") || !strings.Contains(colored.String(), "\x1b[33mdirty") {
		t.Fatalf("colored output lacks name or status color: %q", colored.String())
	}
	if ansi.Strip(colored.String()) != plain.String() {
		t.Fatalf("color changed info content:\nplain: %q\ncolor: %q", plain.String(), ansi.Strip(colored.String()))
	}
}
