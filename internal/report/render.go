package report

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"
)

func Markdown(d Document, view string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Rog work report\n\n%s to %s (%s)\n\n", d.Since.Format("2006-01-02 15:04"), d.Until.Format("2006-01-02 15:04"), d.Timezone)
	fmt.Fprintf(&b, "Configured Roots: %s  \nIndex updated: %s  \nCollection: %d ms  \nCompleteness: %s\n\n", strings.Join(d.ConfiguredRoots, ", "), d.IndexUpdatedAt.Format(time.RFC3339), d.CollectionMS, completeness(d.Complete))
	if len(d.Warnings) > 0 {
		fmt.Fprintf(&b, "## Warnings (%d)\n\n", len(d.Warnings))
		for _, w := range warningSamples(d.Warnings, 5) {
			fmt.Fprintf(&b, "- %s\n", clean(w))
		}
		if len(d.Warnings) > 5 {
			fmt.Fprintf(&b, "- … %d more warnings; use `rog report -o json` for the full list.\n", len(d.Warnings)-5)
		}
		fmt.Fprintln(&b)
	}
	if d.CoverageExplanation != "" {
		fmt.Fprintf(&b, "## Coverage explanation\n\n%s\n\n", clean(d.CoverageExplanation))
	}
	if view == "" || view == "weekly" {
		b.WriteString("## Weekly\n\n")
		for _, p := range d.WeeklyProjects() {
			fmt.Fprintf(&b, "### %s\n\n", clean(p.Name))
			fmt.Fprintf(&b, "%d authored %s · %d active %s · %d changed %s · +%d/−%d lines", p.Metrics.Commits, plural(p.Metrics.Commits, "commit"), p.Metrics.ActiveDays, plural(p.Metrics.ActiveDays, "day"), p.Metrics.ChangedPaths, plural(p.Metrics.ChangedPaths, "path"), p.Metrics.Additions, p.Metrics.Deletions)
			if !p.Metrics.FirstActivity.IsZero() {
				fmt.Fprintf(&b, " · activity span %s–%s", p.Metrics.FirstActivity.In(time.Local).Format("Jan 2"), p.Metrics.LastActivity.In(time.Local).Format("Jan 2"))
			}
			b.WriteString("\n\n")
			for i, c := range p.Commits {
				if i >= 5 {
					fmt.Fprintf(&b, "- … and %d more commits\n", len(p.Commits)-i)
					break
				}
				fmt.Fprintf(&b, "- %s (%s)\n", clean(c.Subject), short(c.Hash))
			}
			fmt.Fprintln(&b)
		}
		if len(d.WeeklyProjects()) == 0 {
			b.WriteString("No matching authored commits in this period. New repositories require `rog scan`.\n\n")
		}
		current := d.CurrentProjects()
		if len(current) > 0 {
			fmt.Fprintf(&b, "### Current working-tree changes (undated)\n\n%d repositories have current edits; these are not attributed to this reporting period.\n\n", len(current))
			for i, p := range current {
				if i >= 10 {
					fmt.Fprintf(&b, "- … and %d more repositories\n", len(current)-i)
					break
				}
				fmt.Fprintf(&b, "- %s: %d changed paths\n", clean(p.Name), p.Metrics.CurrentChangedPaths)
			}
			b.WriteByte('\n')
		}
	}
	if view == "" || view == "dashboard" {
		b.WriteString("## Dashboard\n\n")
		b.WriteString("Projects ranked by active days, authored commits, then unique changed paths. Commits must match the date range and configured author emails; current edits are included separately without a date. Activity span and line counts are not hours worked.\n\n")
		fmt.Fprintf(&b, "Coverage: %d indexed repositories → %d grouped projects · %d with matching commits · %d with current edits · %d unavailable. Showing up to 50 active projects (%d total); JSON contains all.\n\n", d.Coverage.IndexedRepositories, d.Coverage.GroupedProjects, d.Coverage.ProjectsWithCommits, d.Coverage.ProjectsWithCurrentChanges, d.Coverage.UnavailableProjects, d.ActiveCount())
		fmt.Fprintln(&b, "| Project | Active days | Commits | Changed paths | Lines +/− | Current paths |\n|---|---:|---:|---:|---:|---:|")
		for _, p := range d.DashboardProjects() {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | +%d/−%d | %d |\n", mdCell(p.Name), p.Metrics.ActiveDays, p.Metrics.Commits, p.Metrics.ChangedPaths, p.Metrics.Additions, p.Metrics.Deletions, p.Metrics.CurrentChangedPaths)
		}
		fmt.Fprintln(&b)
	}
	if view == "" || view == "log" {
		b.WriteString("## Log\n\n")
		for _, p := range d.Projects {
			if len(p.Commits) == 0 && p.Metrics.CurrentChangedPaths == 0 && len(p.Warnings) == 0 {
				continue
			}
			fmt.Fprintf(&b, "### %s\n\n", clean(p.Name))
			for _, c := range p.Commits {
				marker := ""
				if c.Merge {
					marker = " [merge]"
				}
				fmt.Fprintf(&b, "- %s %s %s%s (+%d/−%d", c.AuthorTime.In(time.Local).Format("2006-01-02"), short(c.Hash), clean(c.Subject), marker, c.Additions, c.Deletions)
				if c.BinaryChanges > 0 {
					fmt.Fprintf(&b, ", %d binary paths", c.BinaryChanges)
				}
				b.WriteString(")\n")
			}
			for _, w := range p.Worktrees {
				if len(w.ChangedPaths) > 0 {
					fmt.Fprintf(&b, "- Current working-tree changes at `%s`: %s\n", strings.ReplaceAll(clean(w.Path), "`", "'"), strings.Join(w.ChangedPaths, ", "))
				}
			}
			for _, w := range p.Warnings {
				fmt.Fprintf(&b, "- Warning: %s\n", clean(w))
			}
			fmt.Fprintln(&b)
		}
	}
	if view == "" || view == "ai" {
		if d.AISummary != "" {
			b.WriteString("## AI Summary\n\n")
			fmt.Fprintln(&b, clean(d.AISummary))
		} else if view == "ai" {
			b.WriteString("## AI Summary\n\nGenerate explicitly with `rog report --llm`.\n")
		}
	}
	return b.String()
}

func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(clean(s))
}
func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
func plural(n int, s string) string {
	if n == 1 {
		return s
	}
	return s + "s"
}
func warningSamples(w []string, n int) []string { return w[:min(n, len(w))] }
func completeness(ok bool) string {
	if ok {
		return "complete"
	}
	return "incomplete"
}

// HTML renders a single offline file. Browser tabs are screen-only; print
// intentionally includes the weekly view and AI summary, when generated.
func HTML(d Document) (string, error) {
	const page = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Rog work report</title><style>
:root{font-family:system-ui,sans-serif;color-scheme:light dark}body{max-width:1100px;margin:auto;padding:2rem;line-height:1.5;background:#101820;color:#e8edf1}h1{color:#76d6e3}h2{border-bottom:1px solid #45535f;padding-bottom:.4rem}.muted{color:#adbac4}.tabs{display:flex;gap:.5rem;flex-wrap:wrap;margin:1.5rem 0}.tabs button{background:#23323d;color:#e8edf1;border:1px solid #45535f;border-radius:.5rem;padding:.6rem 1rem;cursor:pointer}.tabs button[aria-selected=true]{background:#126678;border-color:#76d6e3}.panel{display:none}.panel.active{display:block}.card{background:#1b2933;border:1px solid #354653;border-radius:.7rem;padding:1rem 1.25rem;margin:1rem 0}.metric{color:#76d6e3;font-weight:600}table{border-collapse:collapse;width:100%}th,td{text-align:left;border-bottom:1px solid #45535f;padding:.5rem}th{color:#76d6e3}.scroll{overflow-x:auto}code{overflow-wrap:anywhere}.warning{border-left:4px solid #e5a94c;padding:.5rem 1rem;background:#3a3023}a{color:#76d6e3}li{margin:.3rem 0}#ai .card{white-space:pre-wrap}
@media(max-width:600px){body{padding:1rem}table,tbody,tr,td{display:block}thead{display:none}tr{border:1px solid #45535f;border-radius:.6rem;padding:.6rem;margin:.7rem 0}td{display:flex;justify-content:space-between;gap:1rem;border:0;padding:.2rem}td::before{content:attr(data-label);color:#76d6e3;font-weight:600}}
@media print{body{background:white;color:black;max-width:none;padding:0}.tabs,.screen-only{display:none!important}.panel{display:none!important}#weekly,#ai.has-ai{display:block!important}.card{background:white;color:black;border:1px solid #aaa;break-inside:avoid}.metric,h1{color:#14566a}h2{border-color:#aaa}a{color:black}}
</style></head><body><header><h1>Rog work report</h1><p class="muted">{{.Since.Format "2006-01-02 15:04"}} to {{.Until.Format "2006-01-02 15:04"}} · {{.Timezone}}</p><p>Configured Roots: {{join .ConfiguredRoots ", "}}<br>Index updated: {{local .IndexUpdatedAt}} · Collection: {{.CollectionMS}} ms · {{if .Complete}}Complete{{else}}Incomplete{{end}}</p></header>
{{if .Warnings}}<div class="warning"><strong>{{len .Warnings}} warnings; JSON contains the full list.</strong><ul>{{range sampleWarnings .Warnings}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
{{if .CoverageExplanation}}<div class="card"><strong>Coverage explanation</strong><p>{{.CoverageExplanation}}</p></div>{{end}}
<nav class="tabs" aria-label="Report views"><button data-tab="weekly" aria-selected="true">Weekly</button><button data-tab="dashboard" aria-selected="false">Dashboard</button><button data-tab="log" aria-selected="false">Log</button><button data-tab="ai" aria-selected="false">AI Summary</button></nav>
<section id="weekly" class="panel active"><h2>Weekly</h2>{{range .WeeklyProjects}}<article class="card"><h3>{{.Name}}</h3><p class="metric">{{.Metrics.Commits}} {{plural .Metrics.Commits "commit"}} · {{.Metrics.ActiveDays}} active {{plural .Metrics.ActiveDays "day"}} · {{.Metrics.ChangedPaths}} changed {{plural .Metrics.ChangedPaths "path"}} · +{{.Metrics.Additions}}/−{{.Metrics.Deletions}} lines{{if not .Metrics.FirstActivity.IsZero}} · activity span {{.Metrics.FirstActivity.Format "Jan 2"}}–{{.Metrics.LastActivity.Format "Jan 2"}}{{end}}</p><ul>{{range slice5 .Commits}}<li>{{.Subject}} <code>{{short .Hash}}</code></li>{{end}}</ul></article>{{else}}<p>No matching authored commits in this period. New repositories require <code>rog scan</code>.</p>{{end}}{{if .CurrentProjects}}<h3>Current working-tree changes (undated)</h3><p class="muted">{{len .CurrentProjects}} repositories have current edits. These are not attributed to this reporting period.</p><ul>{{range slice10 .CurrentProjects}}<li>{{.Name}}: {{.Metrics.CurrentChangedPaths}} changed paths</li>{{end}}</ul>{{end}}</section>
<section id="dashboard" class="panel"><h2>Dashboard</h2><p class="muted">Ranked by active days, commits, then changed paths. Commits must match the date range and author email; current edits are undated. These numbers do not measure hours worked. Coverage: {{.Coverage.IndexedRepositories}} indexed repositories · {{.Coverage.GroupedProjects}} grouped projects · {{.Coverage.ProjectsWithCommits}} with matching commits · {{.Coverage.ProjectsWithCurrentChanges}} with current edits · {{.Coverage.UnavailableProjects}} unavailable. Showing up to 50 of {{.ActiveCount}} active projects; JSON includes all.</p><div class="scroll"><table><thead><tr><th>Project</th><th>Days</th><th>Commits</th><th>Paths</th><th>Lines +/−</th><th>Current paths</th></tr></thead><tbody>{{range .DashboardProjects}}<tr><td data-label="Project">{{.Name}}</td><td data-label="Days">{{.Metrics.ActiveDays}}</td><td data-label="Commits">{{.Metrics.Commits}}</td><td data-label="Paths">{{.Metrics.ChangedPaths}}</td><td data-label="Lines +/−">+{{.Metrics.Additions}}/−{{.Metrics.Deletions}}</td><td data-label="Current paths">{{.Metrics.CurrentChangedPaths}}</td></tr>{{end}}</tbody></table></div></section>
<section id="log" class="panel"><h2>Log</h2>{{range .Projects}}{{if or .Commits .Metrics.CurrentChangedPaths .Warnings}}<article class="card"><h3>{{.Name}}</h3><ul>{{range .Commits}}<li>{{.AuthorTime.Format "2006-01-02"}} · <code>{{short .Hash}}</code> · {{.Subject}}{{if .Merge}} [merge]{{end}}{{if .BinaryChanges}} [{{.BinaryChanges}} binary paths]{{end}}</li>{{end}}{{range .Worktrees}}{{if .ChangedPaths}}<li>Current working-tree changes: {{join .ChangedPaths ", "}}</li>{{end}}{{end}}{{range .Warnings}}<li>Warning: {{.}}</li>{{end}}</ul></article>{{end}}{{end}}</section>
<section id="ai" class="panel{{if .AISummary}} has-ai{{end}}"><h2>AI Summary</h2>{{if .AISummary}}<div class="card">{{.AISummary}}</div>{{else}}<p>Generate explicitly with <code>rog report --llm</code>.</p>{{end}}</section>
<script>for(const b of document.querySelectorAll('[data-tab]'))b.addEventListener('click',()=>{for(const x of document.querySelectorAll('[data-tab]'))x.setAttribute('aria-selected',x===b?'true':'false');for(const p of document.querySelectorAll('.panel'))p.classList.toggle('active',p.id===b.dataset.tab)});</script></body></html>`
	t, err := template.New("report").Funcs(template.FuncMap{"join": strings.Join, "short": short, "plural": plural, "local": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return t.In(time.Local).Format("2006-01-02 15:04 MST")
	}, "slice5": func(cs []Commit) []Commit { return cs[:min(5, len(cs))] }, "slice10": func(ps []Project) []Project { return ps[:min(10, len(ps))] }, "sampleWarnings": func(w []string) []string { return warningSamples(w, 3) }}).Parse(page)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	err = t.Execute(&b, d)
	return b.String(), err
}

func AllHashes(d Document) map[string]bool {
	hashes := map[string]bool{}
	for _, p := range d.Projects {
		for _, c := range p.Commits {
			hashes[c.Hash] = true
			hashes[short(c.Hash)] = true
		}
	}
	return hashes
}
func SortStrings(v []string) { sort.Strings(v) }
