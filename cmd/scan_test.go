package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestResolveProgressModePrecedence(t *testing.T) {
	t.Setenv("ROG_PROGRESS", "plain")

	cfgMode := "off"
	flagMode := "rich"

	mode, err := resolveProgressMode(flagMode, cfgMode)
	if err != nil {
		t.Fatalf("resolveProgressMode() error = %v", err)
	}
	if mode != progressModeRich {
		t.Fatalf("resolveProgressMode() = %q, want %q", mode, progressModeRich)
	}
}

func TestResolveProgressModeInvalidEnv(t *testing.T) {
	t.Setenv("ROG_PROGRESS", "loud")

	_, err := resolveProgressMode("", "")
	if err == nil {
		t.Fatal("expected invalid env value to fail")
	}
}

func TestPlainProgressRendererIsASCIISafe(t *testing.T) {
	renderer := newProgressRenderer(progressModePlain, false)

	start := renderer.Start(scanProgressSnapshot{
		Phase:      scanPhaseScan,
		RootsTotal: 2,
	})
	update := renderer.Update(scanProgressSnapshot{
		Phase:          scanPhaseScan,
		RootsTotal:     2,
		RootsCompleted: 1,
		ReposFound:     12,
	})
	done := renderer.Finish(scanProgressSnapshot{
		Phase:      scanPhaseDone,
		RootsTotal: 2,
		ReposFound: 12,
	})

	for _, chunk := range []string{start, update, done} {
		for _, r := range chunk {
			if r > unicodeMaxASCII {
				t.Fatalf("plain renderer emitted non-ASCII rune %q in %q", r, chunk)
			}
		}
	}
}

func TestOffProgressRendererSuppressesIntermediateOutput(t *testing.T) {
	renderer := newProgressRenderer(progressModeOff, false)

	if got := renderer.Start(scanProgressSnapshot{Phase: scanPhaseScan}); got != "" {
		t.Fatalf("Start() = %q, want empty string", got)
	}
	if got := renderer.Update(scanProgressSnapshot{Phase: scanPhaseScan}); got != "" {
		t.Fatalf("Update() = %q, want empty string", got)
	}

	done := renderer.Finish(scanProgressSnapshot{Phase: scanPhaseDone, ReposFound: 7})
	if !strings.Contains(done, "7") {
		t.Fatalf("Finish() = %q, want final summary to include repo count", done)
	}
}

func TestAutoProgressFallsBackToPlainWhenNotTTY(t *testing.T) {
	t.Setenv("ROG_PROGRESS", "")

	cfgMode := "auto"

	mode, err := resolveProgressMode("", cfgMode)
	if err != nil {
		t.Fatalf("resolveProgressMode() error = %v", err)
	}

	renderer := newProgressRenderer(mode, isInteractiveTerminal(os.Stdout))
	if renderer.Mode() != progressModePlain {
		t.Fatalf("renderer mode = %q, want %q", renderer.Mode(), progressModePlain)
	}
}

const unicodeMaxASCII = 127

func TestRichProgressFinishUsesTerminalSafeLineEnding(t *testing.T) {
	renderer := richProgressRenderer{}
	output := renderer.Finish(scanProgressSnapshot{RootsTotal: 3, ReposFound: 753})
	if !strings.Contains(output, "[done] Scan completed") {
		t.Fatalf("missing final summary: %q", output)
	}
	if !strings.HasSuffix(output, "\r\n") || strings.Contains(strings.ReplaceAll(output, "\r\n", ""), "\n") {
		t.Fatalf("rich summary has a bare newline: %q", output)
	}
}
