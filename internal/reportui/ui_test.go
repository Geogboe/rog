package reportui

import (
	"strings"
	"testing"

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
