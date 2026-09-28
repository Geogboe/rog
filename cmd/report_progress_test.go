package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestReportProgressFitsNarrowTerminal(t *testing.T) {
	for _, width := range []int{28, 48, 80, 90} {
		line := renderReportProgress("WSL Ubuntu", 12, 20, "\x1b[31mvery-long-repository-name\nnext", 3*time.Second, 2, width, true)
		if ansi.StringWidth(line) > width-1 || strings.Contains(line, "\n") {
			t.Fatalf("progress does not fit width %d: %q", width, line)
		}
	}
	if line := renderReportProgress("Local Git", 12, 20, "demo", 3*time.Second, 2, 80, false); !strings.Contains(line, "recent:") {
		t.Fatal("regular terminal width should show the recent repository")
	}
}
