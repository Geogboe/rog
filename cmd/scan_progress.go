package cmd

import (
	"fmt"
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
	Phase          scanPhase
	RootsTotal     int
	RootsCompleted int
	ReposFound     int
	StaleRemoved   int
	Duration       time.Duration
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
	return fmt.Sprintf("Scan completed in %s (%d roots, %d repositories", formatProgressDuration(snapshot.Duration), snapshot.RootsTotal, snapshot.ReposFound) +
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
		return fmt.Sprintf("LLM progress: %d repositories in %s\n", snapshot.ReposFound, formatProgressDuration(snapshot.Duration))
	default:
		return fmt.Sprintf("Scan progress: %d/%d roots, %d repositories, %s elapsed\n",
			snapshot.RootsCompleted,
			snapshot.RootsTotal,
			snapshot.ReposFound,
			formatProgressDuration(snapshot.Duration),
		)
	}
}
func (plainProgressRenderer) Finish(snapshot scanProgressSnapshot) string {
	return fmt.Sprintf("Scan completed in %s\nRoots scanned: %d\nRepositories found: %d\nStale removed: %d\n",
		formatProgressDuration(snapshot.Duration),
		snapshot.RootsTotal,
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
	return "\r" + clearLine() + fmt.Sprintf("%s Scan completed in %s\nRoots scanned: %d\nRepositories found: %d\nStale removed: %d\n",
		r.label("done"),
		formatProgressDuration(snapshot.Duration),
		snapshot.RootsTotal,
		snapshot.ReposFound,
		snapshot.StaleRemoved,
	)
}

func (r richProgressRenderer) renderLine(label string, snapshot scanProgressSnapshot) string {
	return fmt.Sprintf("\r%s %d/%d roots  %d repos  %s",
		clearLine()+r.label(label),
		snapshot.RootsCompleted,
		snapshot.RootsTotal,
		snapshot.ReposFound,
		formatProgressDuration(snapshot.Duration),
	)
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
