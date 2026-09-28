package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/report"
	"github.com/Geogboe/rog/internal/reportui"
	"github.com/Geogboe/rog/internal/wsl"
	"github.com/Geogboe/rog/internal/wslbridge"
)

var (
	reportSince, reportUntil, reportOutput, reportView, reportFile, reportProgressMode string
	reportEmails                                                                       []string
	reportOpen, reportLLM, reportApproveWorker                                         bool
)

var reportCmd = &cobra.Command{
	Use: "report [html-file-or-directory]", Short: "Report work across Configured Roots",
	Long: "Summarize authored commits on all local branches and current working-tree changes from indexed repositories. New repositories require 'rog scan'. The default range is this week in local time. AI generation is opt-in.",
	Args: cobra.MaximumNArgs(1), RunE: runReport,
}

func init() {
	rootCmd.AddCommand(reportCmd)
	reportCmd.Flags().StringVar(&reportSince, "since", "", "Start date (YYYY-MM-DD) or RFC3339 time, inclusive")
	reportCmd.Flags().StringVar(&reportUntil, "until", "", "End date (YYYY-MM-DD, inclusive) or RFC3339 time, exclusive")
	reportCmd.Flags().StringSliceVar(&reportEmails, "author-email", nil, "Author email override (repeatable)")
	reportCmd.Flags().StringVarP(&reportOutput, "output", "o", "", "Output: markdown, json, html; defaults to terminal tabs or Markdown when redirected")
	reportCmd.Flags().StringVar(&reportView, "view", "", "Markdown view: weekly, dashboard, log, ai; default includes all")
	reportCmd.Flags().StringVar(&reportFile, "file", "", "Write export atomically to this file")
	reportCmd.Flags().StringVar(&reportProgressMode, "progress", "", "Progress mode: auto, rich, plain, off")
	reportCmd.Flags().BoolVar(&reportOpen, "open", false, "Save an HTML report and open it in the browser; optional directory or .html path")
	reportCmd.Flags().BoolVar(&reportLLM, "llm", false, "Explicitly generate an AI Summary using the configured provider")
	reportCmd.Flags().BoolVar(&reportApproveWorker, "approve-wsl-worker-install", false, "Approve installing the matching report worker in configured WSL distros")
}

type reportExitError struct{ code int }

func (e *reportExitError) Error() string { return "report incomplete" }
func ExitCode(err error) int {
	var e *reportExitError
	if errors.As(err, &e) {
		return e.code
	}
	return 1
}

func runReport(cmd *cobra.Command, args []string) error {
	start := time.Now()
	outputMode, outputPath, err := resolveReportDestination(args, start)
	if err != nil {
		return err
	}
	since, until := report.CurrentWeek(start)
	if reportSince != "" {
		since, err = parseReportDate(reportSince, false)
		if err != nil {
			return err
		}
	}
	if reportUntil != "" {
		until, err = parseReportDate(reportUntil, true)
		if err != nil {
			return err
		}
	}
	if !until.After(since) {
		return fmt.Errorf("--until must be later than --since")
	}
	if reportView != "" && reportView != "weekly" && reportView != "dashboard" && reportView != "log" && reportView != "ai" {
		return fmt.Errorf("invalid --view %q (weekly, dashboard, log, ai)", reportView)
	}
	if reportOutput != "" && reportOutput != "markdown" && reportOutput != "json" && reportOutput != "html" {
		return fmt.Errorf("invalid --output %q (markdown, json, html)", reportOutput)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	idx, err := index.Load()
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}
	zone, _ := start.In(time.Local).Zone()
	d := report.Document{SchemaVersion: report.SchemaVersion, GeneratedAt: start, IndexUpdatedAt: idx.UpdatedAt, Since: since, Until: until, Timezone: zone, Projects: []report.Project{}, Complete: true}
	roots := map[string]config.Root{}
	for _, root := range cfg.Roots {
		roots[root.Name] = root
		d.ConfiguredRoots = append(d.ConfiguredRoots, root.Name)
	}
	sort.Strings(d.ConfiguredRoots)
	if len(cfg.Roots) == 0 {
		d.Warnings = append(d.Warnings, "No Configured Roots. Run 'rog init' first.")
		d.Complete = false
	}
	var local []*index.Repo
	wslRepos := map[string][]*index.Repo{}
	stale := 0
	for _, repo := range idx.List() {
		root, ok := roots[repo.Root]
		if !ok || !repoInConfiguredRoot(repo, root) {
			stale++
			continue
		}
		if repo.IsWSL && runtime.GOOS == "windows" {
			wslRepos[repo.WSLDistro] = append(wslRepos[repo.WSLDistro], repo)
		} else {
			local = append(local, repo)
		}
	}
	if stale > 0 {
		d.Warnings = append(d.Warnings, fmt.Sprintf("%d indexed repositories are outside current Configured Roots; run 'rog scan' to refresh the index", stale))
	}
	if idx.Count() == 0 {
		d.Warnings = append(d.Warnings, "No indexed repositories. Run 'rog scan' first.")
		d.Complete = false
	}
	emails := reportEmails
	if len(emails) == 0 && cfg.Report != nil {
		emails = cfg.Report.AuthorEmails
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	progressMode, err := resolveProgressMode(reportProgressMode, "")
	if err != nil {
		return err
	}
	tracker := newReportProgress(progressMode)
	defer tracker.Close()
	if len(local) > 0 {
		tracker.Phase("Local Git", len(local))
	}
	progress := func(done, total int, recent string) {
		tracker.Advance(done, total, recent)
	}
	projects, warnings := report.CollectLocal(ctx, local, report.Options{Since: since, Until: until, AuthorEmails: emails, IncludePatches: reportLLM && cfg.LLM != nil, OnProgress: progress})
	d.Projects = append(d.Projects, projects...)
	d.Warnings = append(d.Warnings, warnings...)
	for distro, repos := range wslRepos {
		var distroRoots []config.Root
		for _, root := range cfg.Roots {
			if root.WSL && (root.WSLDistro == "" || root.WSLDistro == distro) {
				distroRoots = append(distroRoots, root)
			}
		}
		bridge := wslbridge.Bridge{Approve: func(name, path string) bool {
			if reportApproveWorker {
				return true
			}
			tracker.Pause()
			defer tracker.Resume()
			return approveWSLWorker(name, path)
		}}
		tracker.Phase("WSL "+distro, len(repos))
		remoteProjects, remoteWarnings, err := bridge.Report(ctx, distro, distroRoots, repos, report.Options{Since: since, Until: until, AuthorEmails: emails, IncludePatches: reportLLM && cfg.LLM != nil, OnProgress: func(done, total int, _ string) {
			tracker.Advance(done, total, "")
		}})
		if err != nil {
			d.Warnings = append(d.Warnings, fmt.Sprintf("WSL %s: %v", distro, err))
			continue
		}
		d.Projects = append(d.Projects, remoteProjects...)
		for _, w := range remoteWarnings {
			d.Warnings = append(d.Warnings, fmt.Sprintf("WSL %s: %s", distro, w))
		}
	}
	for _, p := range d.Projects {
		for _, w := range p.Warnings {
			d.Warnings = append(d.Warnings, fmt.Sprintf("%s: %s", p.Name, w))
		}
	}
	if len(d.Warnings) > 0 {
		d.Complete = false
	}
	report.SortProjects(d.Projects)
	d.CollectionMS = time.Since(start).Milliseconds()
	if ctx.Err() != nil {
		return &reportExitError{code: 130}
	}
	generate := func(callCtx context.Context) (string, error) {
		if cfg.LLM == nil {
			return "", fmt.Errorf("configure llm.endpoint and llm.model before generating AI Summary")
		}
		if !reportLLM {
			fillReportPatches(callCtx, &d, wslRepos, cfg, emails)
		}
		return report.GenerateAI(callCtx, d, cfg.LLM)
	}
	if reportLLM {
		tracker.Pause()
		if cfg.LLM != nil && cfg.LLM.Endpoint != "" && cfg.LLM.Model != "" {
			fmt.Fprintf(os.Stderr, "AI Summary: about %d bytes of evidence to %s (cap %d bytes; output cap %d tokens; timeout %s)\n", len(report.Evidence(d)), report.ProviderHost(cfg.LLM), report.MaxAIContextBytes, report.MaxAIOutputTokens, report.AITimeout)
			if estimate := report.CostEstimate(cfg.Report); estimate != "" {
				fmt.Fprintln(os.Stderr, estimate)
			}
		}
		tracker.Resume()
		tracker.Phase("AI Summary", 0)
		d.AISummary, err = generate(ctx)
		if err != nil {
			d.Warnings = append(d.Warnings, "AI Summary: "+err.Error())
			d.Complete = false
		}
	}
	tracker.Close()
	mode := outputMode
	if mode == "" && outputPath == "" && reportView == "" && isInteractiveTerminal(os.Stdout) {
		var action func(context.Context) (string, error)
		provider := "Configure llm.endpoint and llm.model"
		if cfg.LLM != nil && cfg.LLM.Endpoint != "" && cfg.LLM.Model != "" {
			action = generate
			provider = fmt.Sprintf("%s (~%d bytes metadata, up to %d bytes with patches)", report.ProviderHost(cfg.LLM), len(report.Evidence(d)), report.MaxAIContextBytes)
		}
		d, err = reportui.Run(ctx, d, provider, action)
		if err != nil {
			if errors.Is(err, reportui.ErrCancelled) {
				return &reportExitError{code: 130}
			}
			return err
		}
		if !d.Complete {
			return &reportExitError{code: 2}
		}
		return nil
	}
	if mode == "" {
		mode = "markdown"
	}
	var output []byte
	switch mode {
	case "json":
		output, err = json.MarshalIndent(d, "", "  ")
		output = append(output, '\n')
	case "html":
		var html string
		html, err = report.HTML(d)
		output = []byte(html)
	default:
		output = []byte(report.Markdown(d, reportView))
	}
	if err != nil {
		return err
	}
	if outputPath != "" {
		if err := writeReportFile(outputPath, output); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Report saved:", outputPath)
	} else {
		if _, err := os.Stdout.Write(output); err != nil {
			return err
		}
	}
	if reportOpen {
		fmt.Fprintln(os.Stderr, "Opening report:", outputPath)
		if err := openReportFile(outputPath); err != nil {
			return err
		}
	}
	if !d.Complete {
		fmt.Fprintf(os.Stderr, "Report incomplete (%d warnings). ", len(d.Warnings))
		switch {
		case containsWarning(d.Warnings, "installation declined"):
			fmt.Fprintln(os.Stderr, "Review the WSL worker cache path in the report, then retry interactively or use --approve-wsl-worker-install.")
		case containsWarning(d.Warnings, "No indexed repositories"):
			fmt.Fprintln(os.Stderr, "Run 'rog scan' to index Configured Roots.")
		default:
			fmt.Fprintln(os.Stderr, "Inspect the report warnings; use -o json for the full list.")
		}
		return &reportExitError{code: 2}
	}
	return nil
}

func resolveReportDestination(args []string, now time.Time) (string, string, error) {
	if len(args) > 1 {
		return "", "", fmt.Errorf("expected at most one HTML file or directory")
	}
	if !reportOpen {
		if len(args) > 0 {
			return "", "", fmt.Errorf("%q requires --open; reports otherwise cover all Configured Roots", args[0])
		}
		return reportOutput, reportFile, nil
	}
	if reportOutput != "" && reportOutput != "html" {
		return "", "", fmt.Errorf("--open requires HTML output; remove -o %s or use -o html", reportOutput)
	}
	if len(args) > 0 && reportFile != "" {
		return "", "", fmt.Errorf("choose either --file or the path after --open")
	}
	if reportFile != "" {
		return "html", reportFile, nil
	}
	destination := os.TempDir()
	if len(args) > 0 {
		destination = args[0]
	}
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		name := fmt.Sprintf("rog-report-%s-%d.html", now.Format("2006-01-02-150405"), os.Getpid())
		return "html", filepath.Join(destination, name), nil
	}
	if len(args) > 0 && strings.EqualFold(filepath.Ext(destination), ".html") {
		return "html", destination, nil
	}
	return "", "", fmt.Errorf("--open path %q must be an existing directory or an .html file", destination)
}

func containsWarning(warnings []string, needle string) bool {
	for _, w := range warnings {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}

func repoInConfiguredRoot(repo *index.Repo, root config.Root) bool {
	if repo.IsWSL && runtime.GOOS == "windows" {
		if !root.WSL || (root.WSLDistro != "" && !strings.EqualFold(root.WSLDistro, repo.WSLDistro)) {
			return false
		}
		expected := wsl.TranslatePathToWindows(repo.WSLDistro, path.Join(root.Path, filepath.ToSlash(repo.RelPath)))
		return strings.EqualFold(expected, repo.AbsPath)
	}
	if root.WSL && runtime.GOOS == "windows" {
		return false
	}
	rel, err := filepath.Rel(root.Path, repo.AbsPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fillReportPatches(ctx context.Context, d *report.Document, wslRepos map[string][]*index.Repo, cfg *config.Config, emails []string) {
	for i := range d.Projects {
		if d.Projects[i].SourcePath != "" && len(d.Projects[i].AIPatches) == 0 {
			d.Projects[i].AIPatches = report.PatchExcerpts(ctx, d.Projects[i], 6000)
		}
	}
	for distro, repos := range wslRepos {
		var roots []config.Root
		for _, root := range cfg.Roots {
			if root.WSL && (root.WSLDistro == "" || root.WSLDistro == distro) {
				roots = append(roots, root)
			}
		}
		bridge := wslbridge.Bridge{Approve: func(name, path string) bool {
			if reportApproveWorker {
				return true
			}
			return approveWSLWorker(name, path)
		}}
		projects, _, err := bridge.Report(ctx, distro, roots, repos, report.Options{Since: d.Since, Until: d.Until, AuthorEmails: emails, IncludePatches: true})
		if err != nil {
			d.Warnings = append(d.Warnings, "AI patch context unavailable for WSL "+distro)
			continue
		}
		for _, p := range projects {
			for i := range d.Projects {
				if len(p.Paths) > 0 && len(d.Projects[i].Paths) > 0 && p.Paths[0] == d.Projects[i].Paths[0] {
					d.Projects[i].AIPatches = p.AIPatches
					break
				}
			}
		}
	}
}

func parseReportDate(input string, end bool) (time.Time, error) {
	if t, err := time.ParseInLocation("2006-01-02", input, time.Local); err == nil {
		if end {
			return t.AddDate(0, 0, 1), nil
		}
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid report date %q: use YYYY-MM-DD or RFC3339", input)
}

func writeReportFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rog-report-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func openReportFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", abs)
	} else if strings.Contains(strings.ToLower(os.Getenv("WSL_DISTRO_NAME")), "ubuntu") {
		c = exec.Command("wslview", abs)
	} else {
		c = exec.Command("xdg-open", abs)
	}
	return c.Start()
}
