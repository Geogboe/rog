package reportui

import (
	"strings"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/report"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestNarrowViewFitsWidth(t *testing.T) {
	m := model{doc: report.Document{ConfiguredRoots: []string{"a-long-configured-root"}, Complete: true}, width: 28, height: 12, color: true}
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 28 {
			t.Fatalf("line exceeds terminal width: %q", line)
		}
	}
}

func TestCtrlCCancelsAIConfirmation(t *testing.T) {
	m := model{confirm: true}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !next.(model).cancelled || command == nil {
		t.Fatal("Ctrl+C should quit with a cancellation status")
	}
}

func TestAISpinnerAdvancesWhileGenerating(t *testing.T) {
	m := model{busy: true, aiStarted: time.Now(), width: 48, height: 10}
	next, command := m.Update(aiTick(time.Now()))
	if next.(model).aiFrame != 1 || command == nil || !strings.Contains(next.(model).View(), "Generating AI Summary") {
		t.Fatal("AI generation should show an updating elapsed indicator")
	}
}
