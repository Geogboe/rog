package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// reportProgress keeps collection feedback on stderr so exported reports stay
// clean. A ticker makes long Git calls and WSL startup visibly alive.
type reportProgress struct {
	mu       sync.Mutex
	mode     progressMode
	phase    string
	recent   string
	done     int
	total    int
	started  time.Time
	lastLog  time.Time
	frame    int
	closed   bool
	paused   bool
	stop     chan struct{}
	finished chan struct{}
}

func newReportProgress(mode progressMode) *reportProgress {
	interactive := term.IsTerminal(os.Stderr.Fd())
	resolved := resolvedProgressMode(mode, interactive, interactive && supportsANSIControls())
	p := &reportProgress{mode: resolved, started: time.Now(), stop: make(chan struct{}), finished: make(chan struct{})}
	if p.mode == progressModeRich {
		go p.run()
	} else {
		close(p.finished)
	}
	return p
}

func (p *reportProgress) run() {
	defer close(p.finished)
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.phase != "" && !p.closed && !p.paused {
				p.frame++
				width, _, err := term.GetSize(os.Stderr.Fd())
				if err != nil || width < 20 {
					width = 80
				}
				fmt.Fprint(os.Stderr, "\r\x1b[2K", renderReportProgress(p.phase, p.done, p.total, p.recent, time.Since(p.started), p.frame, width, supportsANSIColor()))
			}
			p.mu.Unlock()
		}
	}
}

func (p *reportProgress) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.paused {
		return
	}
	p.paused = true
	if p.mode == progressModeRich && p.phase != "" {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
}

func (p *reportProgress) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
}

func (p *reportProgress) Phase(phase string, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.mode == progressModeOff {
		return
	}
	p.phase, p.done, p.total, p.recent = phase, 0, total, ""
	if p.mode == progressModePlain {
		fmt.Fprintf(os.Stderr, "[report] %s: %d indexed repositories\n", phase, total)
		p.lastLog = time.Now()
	}
}

func (p *reportProgress) Advance(done, total int, recent string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.mode == progressModeOff {
		return
	}
	p.done, p.total, p.recent = done, total, recent
	if p.mode == progressModePlain && (done == total || done%25 == 0 || time.Since(p.lastLog) >= 10*time.Second) {
		fmt.Fprintf(os.Stderr, "[report] %s %d/%d projects, %s elapsed\n", p.phase, done, total, formatProgressDuration(time.Since(p.started)))
		p.lastLog = time.Now()
	}
}

func (p *reportProgress) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.stop)
	rich := p.mode == progressModeRich && p.phase != ""
	p.mu.Unlock()
	<-p.finished
	if rich {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
}

func renderReportProgress(phase string, done, total int, recent string, elapsed time.Duration, frame, width int, color bool) string {
	spinners := []string{"◐", "◓", "◑", "◒"}
	label := "[report]"
	if color {
		label = "\x1b[36m[report]\x1b[0m"
	}
	body := fmt.Sprintf("%s %s %s", label, spinners[frame%len(spinners)], phase)
	if total > 0 {
		body += fmt.Sprintf(" %d/%d", done, total)
		if width >= 80 {
			filled := min(10, max(0, done*10/total))
			body += " [" + strings.Repeat("■", filled) + strings.Repeat("·", 10-filled) + "]"
		}
	}
	body += " · " + formatProgressDuration(elapsed)
	if width >= 72 && recent != "" {
		body += " · recent:" + formatCurrentRepo(recent)
	}
	if width > 1 {
		body = ansi.Truncate(body, width-1, "…")
	}
	return body
}
