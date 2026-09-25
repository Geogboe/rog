package cmd

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"os"
	"runtime"
	"strings"
	"time"
)

type progressMode string

const (
	progressModeAuto  progressMode = "auto"
	progressModeOff   progressMode = "off"
	progressModePlain progressMode = "plain"
	progressModeRich  progressMode = "rich"
)

type scanPhase string

const (
	scanPhaseScan   scanPhase = "scan"
	scanPhaseEnrich scanPhase = "enrich"
	scanPhaseDone   scanPhase = "done"
)

type scanProgressSnapshot struct {
	Phase             scanPhase
	RootsTotal        int
	RootsCompleted    int
	RootsSucceeded    int
	ReposFound        int
	Indexed           int
	ReposReused       int
	ReposRefreshed    int
	StatusUnavailable int
	CurrentRoot       string
	CurrentRepo       string
	StaleRemoved      int
	Incomplete        bool
	Duration          time.Duration
}

type progressRenderer interface {
	Mode() progressMode
	Start(scanProgressSnapshot) string
	Update(scanProgressSnapshot) string
	Finish(scanProgressSnapshot) string
}

type offProgressRenderer struct{}

func (offProgressRenderer) Mode() progressMode { return progressModeOff }
func (offProgressRenderer) Start(scanProgressSnapshot) string {
	return ""
}
func (offProgressRenderer) Update(scanProgressSnapshot) string {
	return ""
}
func (offProgressRenderer) Finish(snapshot scanProgressSnapshot) string {
	status := "Scan completed"
	if snapshot.Incomplete {
		status = "Scan incomplete"
	}
	return fmt.Sprintf("%s in %s (%d/%d roots complete, %d indexed repositories, %d Git markers", status, formatProgressDuration(snapshot.Duration), snapshot.RootsSucceeded, snapshot.RootsTotal, snapshot.Indexed, snapshot.ReposFound) +
		fmt.Sprintf(", %d stale removed)\n", snapshot.StaleRemoved)
}

type plainProgressRenderer struct{}

func (plainProgressRenderer) Mode() progressMode { return progressModePlain }
func (plainProgressRenderer) Start(snapshot scanProgressSnapshot) string {
	switch snapshot.Phase {
	case scanPhaseEnrich:
		return "Enriching repositories with LLM...\n"
	default:
		return fmt.Sprintf("Scanning %d roots...\n", snapshot.RootsTotal)
	}
}
func (plainProgressRenderer) Update(snapshot scanProgressSnapshot) string {
	switch snapshot.Phase {
	case scanPhaseEnrich:
		return fmt.Sprintf("LLM progress: %d Git markers in %s\n", snapshot.ReposFound, formatProgressDuration(snapshot.Duration))
	default:
		return fmt.Sprintf("Scan progress: %d/%d roots, %d Git markers, %s elapsed\n",
			snapshot.RootsCompleted,
			snapshot.RootsTotal,
			snapshot.ReposFound,
			formatProgressDuration(snapshot.Duration),
		)
	}
}
func (plainProgressRenderer) Finish(snapshot scanProgressSnapshot) string {
	status := "Scan completed"
	if snapshot.Incomplete {
		status = "Scan incomplete"
	}
	return fmt.Sprintf("%s in %s\nRoots completed: %d/%d\nIndexed repositories: %d\nGit markers found: %d\nStale removed: %d\n",
		status,
		formatProgressDuration(snapshot.Duration),
		snapshot.RootsSucceeded,
		snapshot.RootsTotal,
		snapshot.Indexed,
		snapshot.ReposFound,
		snapshot.StaleRemoved,
	)
}

type richProgressRenderer struct {
	color bool
}

func (r richProgressRenderer) Mode() progressMode { return progressModeRich }

func (r richProgressRenderer) Start(snapshot scanProgressSnapshot) string {
	return r.renderLine("scan", snapshot)
}

func (r richProgressRenderer) Update(snapshot scanProgressSnapshot) string {
	return r.renderLine("scan", snapshot)
}

func (r richProgressRenderer) Finish(snapshot scanProgressSnapshot) string {
	status := "Scan completed"
	if snapshot.Incomplete {
		status = "Scan incomplete"
	}
	return "\r" + clearLine() + fmt.Sprintf("%s %s in %s (%d/%d roots complete, %d indexed repositories, %d Git markers, %d stale removed)\r\n",
		r.label("done"), status,
		formatProgressDuration(snapshot.Duration),
		snapshot.RootsSucceeded,
		snapshot.RootsTotal,
		snapshot.Indexed,
		snapshot.ReposFound,
		snapshot.StaleRemoved,
	)
}

func (r richProgressRenderer) renderLine(label string, snapshot scanProgressSnapshot) string {
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width < 20 {
		width = 120
	}
	return r.renderLineWidth(label, snapshot, width)
}

func (r richProgressRenderer) renderLineWidth(label string, snapshot scanProgressSnapshot, width int) string {
	name := snapshot.CurrentRoot
	if snapshot.CurrentRepo != "" {
		if name != "" {
			name += "/"
		}
		name += snapshot.CurrentRepo
	}
	var body string
	if width < 72 {
		body = fmt.Sprintf("%s %d/%d  %d markers  %s", r.label(label), snapshot.RootsCompleted, snapshot.RootsTotal, snapshot.ReposFound, formatProgressDuration(snapshot.Duration))
	} else {
		body = fmt.Sprintf("%s %d/%d roots  %d markers  %d refreshed  %d reused  %s", r.label(label), snapshot.RootsCompleted, snapshot.RootsTotal, snapshot.ReposFound, snapshot.ReposRefreshed, snapshot.ReposReused, formatProgressDuration(snapshot.Duration))
	}
	body += formatCurrentRepo(name)
	if width > 1 {
		body = ansi.Truncate(body, width-1, "…")
	}
	return "\r" + clearLine() + body
}

func formatCurrentRepo(name string) string {
	if name == "" {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" {
		return ""
	}
	const maxRunes = 32
	runes := []rune(name)
	if len(runes) > maxRunes {
		name = string(runes[:maxRunes-1]) + "…"
	}
	return "  " + name
}

func (r richProgressRenderer) label(text string) string {
	if !r.color {
		return "[" + text + "]"
	}

	const (
		reset = "\x1b[0m"
		cyan  = "\x1b[36m"
		green = "\x1b[32m"
	)

	if text == "done" {
		return green + "[done]" + reset
	}
	return cyan + "[scan]" + reset
}

func resolveProgressMode(flagMode, cfgMode string) (progressMode, error) {
	value := strings.TrimSpace(flagMode)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("ROG_PROGRESS"))
	}
	if value == "" {
		value = strings.TrimSpace(cfgMode)
	}
	if value == "" {
		value = string(progressModeAuto)
	}

	mode := progressMode(strings.ToLower(value))
	switch mode {
	case progressModeAuto, progressModeOff, progressModePlain, progressModeRich:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid progress mode %q (valid: auto, off, plain, rich)", value)
	}
}

func newProgressRenderer(mode progressMode, interactive bool) progressRenderer {
	ansi := interactive && supportsANSIControls()

	switch resolvedProgressMode(mode, interactive, ansi) {
	case progressModeOff:
		return offProgressRenderer{}
	case progressModeRich:
		return richProgressRenderer{color: ansi && supportsANSIColor()}
	default:
		return plainProgressRenderer{}
	}
}

func resolvedProgressMode(mode progressMode, interactive bool, ansi bool) progressMode {
	switch mode {
	case progressModeOff:
		return progressModeOff
	case progressModeRich:
		if interactive && ansi {
			return progressModeRich
		}
		return progressModePlain
	case progressModeAuto:
		if interactive && ansi {
			return progressModeRich
		}
		return progressModePlain
	default:
		return progressModePlain
	}
}

func isInteractiveTerminal(file *os.File) bool {
	if file == nil {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func supportsANSIColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	return supportsANSIControls()
}

func supportsANSIControls() bool {

	if runtime.GOOS != "windows" {
		term := strings.ToLower(os.Getenv("TERM"))
		return term != "" && term != "dumb"
	}

	if os.Getenv("WT_SESSION") != "" || os.Getenv("ANSICON") != "" {
		return true
	}
	if strings.EqualFold(os.Getenv("ConEmuANSI"), "ON") {
		return true
	}

	term := strings.ToLower(os.Getenv("TERM"))
	return strings.Contains(term, "xterm") || strings.Contains(term, "ansi")
}

func formatProgressDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0s"
	}
	return duration.Round(time.Millisecond).String()
}

func clearLine() string {
	return "\x1b[2K"
}
