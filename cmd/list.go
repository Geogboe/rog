package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/query"
)

var (
	listLang   []string
	listTag    []string
	listBranch string
	listRoot   string
	listDirty  bool
	listClean  bool
	listAhead  bool
	listBehind bool
	listSort   string
	listLimit  int
	listLong   bool
	listShort  bool
	listOutput string
	listFields string
)

var listCmd = &cobra.Command{
	Use:   "list [search terms...]",
	Short: "List repositories with optional filtering",
	Long: `List repositories from the index with fuzzy search and filtering.

Search terms are fuzzy-matched against:
  - Repository name
  - Description
  - Tags
  - Path
  - Remote URL

Filters are applied using flags for exact matches.

Output modes:
  --short: Minimal output (name, language, path)
  (default): Compact table (name, lang, branch, status, path)
  --long: Detailed output (adds author, remote URL)
  --fields: Custom fields (comma-separated)
  --output, -o: table, json, yaml, or path

Available fields:
  name, lang, host, branch, status, commit, author, root, path, remote, tags, description

Examples:
  rog list                           # List all repositories
  rog list api                       # Fuzzy search for "api"
  rog list --lang go --tag cli       # Go repos tagged with "cli"
  rog list --dirty                   # Repos with uncommitted changes
  rog list --short                   # Minimal output
  rog list --fields name,lang,branch # Custom fields
  rog list -o json                   # JSON format
  rog list --sort last-commit --limit 10  # 10 most recently committed`,
	Run:     runList,
	Aliases: []string{"ls"},
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringSliceVar(&listLang, "lang", nil, "Filter by language")
	listCmd.Flags().StringSliceVar(&listTag, "tag", nil, "Filter by tags (all must match)")
	listCmd.Flags().StringVar(&listBranch, "branch", "", "Filter by branch")
	listCmd.Flags().StringVar(&listRoot, "root", "", "Filter by root")
	listCmd.Flags().BoolVar(&listDirty, "dirty", false, "Show only dirty repos")
	listCmd.Flags().BoolVar(&listClean, "clean", false, "Show only clean repos")
	listCmd.Flags().BoolVar(&listAhead, "ahead", false, "Show only repos ahead of remote")
	listCmd.Flags().BoolVar(&listBehind, "behind", false, "Show only repos behind remote")
	listCmd.Flags().StringVar(&listSort, "sort", "name", "Sort by: name, last-commit, path, last-scan")
	listCmd.Flags().IntVar(&listLimit, "limit", 0, "Limit number of results")
	listCmd.Flags().BoolVarP(&listLong, "long", "l", false, "Show detailed information")
	listCmd.Flags().BoolVarP(&listShort, "short", "s", false, "Show minimal information")
	listCmd.Flags().StringVar(&listFields, "fields", "", "Custom fields to display (comma-separated)")
	listCmd.Flags().StringVarP(&listOutput, "output", "o", "table", "Output format: table, json, yaml, path")
}

func runList(cmd *cobra.Command, args []string) {
	// Validate mutually exclusive flags
	if listShort && listLong {
		exitWithError("Cannot use --short and --long together")
	}
	if listFields != "" && (listShort || listLong) {
		exitWithError("Cannot use --fields with --short or --long")
	}

	if listOutput != "table" && listOutput != "json" && listOutput != "yaml" && listOutput != "path" {
		exitWithError("Invalid output %q (valid: table, json, yaml, path)", listOutput)
	}

	// Determine which fields to display
	var fields []string
	if listFields != "" {
		// Use explicitly specified fields
		fields = parseFields(listFields)
	} else {
		// Try to load default fields from config
		cfg, err := config.Load()
		if err == nil && cfg.List != nil && len(cfg.List.DefaultFields) > 0 {
			fields = cfg.List.DefaultFields
		}
	}

	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	if idx.Count() == 0 {
		if listOutput == "table" {
			fmt.Println("No repositories found. Run 'rog scan' first.")
		} else {
			outputListResults([]*index.Repo{}, fields)
		}
		return
	}

	// Build filter
	filter := &query.Filter{
		SearchTerms: args,
		Languages:   listLang,
		Tags:        listTag,
		Branch:      listBranch,
		Root:        listRoot,
		SortBy:      parseSortField(listSort),
		Limit:       listLimit,
	}

	// Handle dirty/clean flags
	if listDirty {
		dirty := true
		filter.Dirty = &dirty
	}
	if listClean {
		clean := false
		filter.Dirty = &clean
	}

	// Handle ahead/behind flags
	if listAhead {
		ahead := true
		filter.Ahead = &ahead
	}
	if listBehind {
		behind := true
		filter.Behind = &behind
	}

	// Execute query
	results := query.Query(idx, filter)

	if len(results) == 0 {
		if listOutput == "table" {
			fmt.Println("No repositories match the criteria.")
		} else {
			outputListResults([]*index.Repo{}, fields)
		}
		return
	}

	outputListResults(results, fields)
}

func outputListResults(results []*index.Repo, fields []string) {
	switch listOutput {
	case "json":
		outputJSON(results)
	case "yaml":
		outputYAML(results)
	case "path":
		for _, repo := range results {
			fmt.Println(repo.AbsPath)
		}
	default:
		outputTable(results, listShort, listLong, fields)
	}
}

func parseSortField(s string) query.SortField {
	switch s {
	case "last-commit":
		return query.SortByLastCommit
	case "path":
		return query.SortByPath
	case "last-scan":
		return query.SortByLastScan
	default:
		return query.SortByName
	}
}

// parseFields parses a comma-separated list of fields and validates them
func parseFields(fieldsStr string) []string {
	validFields := map[string]bool{
		"name":        true,
		"lang":        true,
		"host":        true,
		"branch":      true,
		"status":      true,
		"commit":      true,
		"author":      true,
		"root":        true,
		"path":        true,
		"remote":      true,
		"tags":        true,
		"description": true,
	}

	fields := strings.Split(fieldsStr, ",")
	result := make([]string, 0, len(fields))

	for _, field := range fields {
		field = strings.TrimSpace(strings.ToLower(field))
		if field == "" {
			continue
		}
		if !validFields[field] {
			exitWithError("Invalid field: %s\nValid fields: name, lang, host, branch, status, commit, author, root, path, remote, tags, description", field)
		}
		result = append(result, field)
	}

	if len(result) == 0 {
		exitWithError("No valid fields specified")
	}

	return result
}

func outputTable(repos []*index.Repo, short bool, long bool, customFields []string) {
	interactive := isInteractiveTerminal(os.Stdout)
	color := interactive && supportsANSIColor()
	width := 0
	if interactive {
		var err error
		width, _, err = term.GetSize(os.Stdout.Fd())
		if err != nil || width <= 0 {
			width = 80
		}
	}
	writeTable(os.Stdout, repos, short, long, customFields, width, color)
}

func writeTable(out io.Writer, repos []*index.Repo, short bool, long bool, customFields []string, width int, color bool) {
	// Determine which fields to display
	var fields []string
	var descMaxLen int
	if len(customFields) > 0 {
		fields = customFields
		descMaxLen = 80 // Custom fields get longer descriptions
	} else if short {
		fields = []string{"name", "lang", "path"}
		descMaxLen = 0 // No description in short mode
	} else if long {
		fields = []string{"name", "lang", "host", "branch", "status", "commit", "author", "description", "root", "path", "remote"}
		descMaxLen = 80 // Long mode: 80 chars for description
	} else {
		fields = []string{"name", "lang", "branch", "status", "path"}
		if width > 0 && width < 90 {
			fields = []string{"name", "status", "path"}
		}
		if width > 0 && width < 50 {
			fields = []string{"name", "path"}
		}
		if width > 0 && width < 24 {
			fields = []string{"name"}
		}
		descMaxLen = 0 // Normal mode: no description
	}

	// Field display names (for headers)
	fieldNames := map[string]string{
		"name":        "NAME",
		"lang":        "LANG",
		"host":        "HOST",
		"branch":      "BRANCH",
		"status":      "STATUS",
		"commit":      "LAST COMMIT",
		"author":      "AUTHOR",
		"root":        "ROOT",
		"path":        "PATH",
		"remote":      "REMOTE",
		"tags":        "TAGS",
		"description": "DESCRIPTION",
	}

	// Print header
	header := make([]string, len(fields))
	for i, field := range fields {
		if displayName, ok := fieldNames[field]; ok {
			header[i] = displayName
		} else {
			header[i] = strings.ToUpper(field)
		}
	}
	// Check if root is in the fields
	hasRoot := false
	for _, field := range fields {
		if field == "root" {
			hasRoot = true
			break
		}
	}

	rows := make([][]string, 0, len(repos))
	widths := make([]int, len(fields))
	for i, heading := range header {
		widths[i] = ansi.StringWidth(heading)
	}
	for _, repo := range repos {
		values := make([]string, len(fields))
		for i, field := range fields {
			value := getFieldValue(repo, field, hasRoot, descMaxLen)
			if len(customFields) == 0 && !short && !long && field == "status" {
				value = formatCompactStatus(repo)
			}
			value = cleanTableCell(value)
			if width > 0 && len(customFields) == 0 && !short && !long {
				value = truncateTableCell(value, field, width, len(fields))
			}
			values[i] = value
			if cellWidth := ansi.StringWidth(value); cellWidth > widths[i] {
				widths[i] = cellWidth
			}
		}
		rows = append(rows, values)
	}

	writeTableRow(out, header, widths)
	for rowIndex, values := range rows {
		if color {
			for i := range values {
				values[i] = colorTableCell(values[i], fields[i], repos[rowIndex])
			}
		}
		writeTableRow(out, values, widths)
	}
	fmt.Fprintf(out, "\nTotal: %d repositories\n", len(repos))
}

func writeTableRow(out io.Writer, cells []string, widths []int) {
	for i, cell := range cells {
		fmt.Fprint(out, cell)
		if i < len(cells)-1 {
			padding := widths[i] - ansi.StringWidth(cell) + 2
			fmt.Fprint(out, strings.Repeat(" ", padding))
		}
	}
	fmt.Fprintln(out)
}

func formatCompactStatus(repo *index.Repo) string {
	status := "clean"
	if repo.StatusUnavailable {
		status = "unknown"
	} else if repo.IsDirty {
		status = "dirty"
	} else if repo.HasUntracked {
		status = "untracked"
	}
	if repo.Ahead > 0 {
		status += fmt.Sprintf(" ↑%d", repo.Ahead)
	}
	if repo.Behind > 0 {
		status += fmt.Sprintf(" ↓%d", repo.Behind)
	}
	return status
}

func cleanTableCell(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '\u009b' {
			return ' '
		}
		return r
	}, value)
}

func truncateTableCell(value, field string, width, columns int) string {
	limit := 0
	if columns == 1 {
		limit = width
	} else if columns == 2 {
		switch field {
		case "name":
			limit = width / 3
		case "path":
			limit = width - width/3 - 4
		}
	} else if columns == 3 {
		switch field {
		case "name":
			limit = 20
		case "status":
			limit = 18
		case "path":
			limit = width - 44
		}
	} else {
		switch field {
		case "name":
			limit = 20
		case "lang":
			limit = 10
		case "branch":
			limit = 16
		case "status":
			limit = 18
		case "path":
			limit = width - 74
		}
	}
	if limit < 8 {
		limit = 8
	}
	return ansi.Truncate(value, limit, "…")
}

func colorTableCell(value, field string, repo *index.Repo) string {
	code := ""
	switch field {
	case "name":
		code = "36"
	case "status":
		if repo.StatusUnavailable {
			code = "31"
		} else if repo.IsDirty || repo.HasUntracked || repo.Behind > 0 {
			code = "33"
		}
	}
	if code == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

// getFieldValue returns the value for a specific field from a repo
func getFieldValue(repo *index.Repo, field string, hasRoot bool, descMaxLen int) string {
	switch field {
	case "name":
		return repo.Name
	case "lang":
		if repo.PrimaryLanguage == "" {
			return "unknown"
		}
		return repo.PrimaryLanguage
	case "host":
		if repo.Host == "" {
			return "-"
		}
		return repo.Host
	case "branch":
		if repo.CurrentBranch == "" {
			return "-"
		}
		return repo.CurrentBranch
	case "status":
		return formatStatus(repo)
	case "commit":
		return formatTime(repo.LastCommitTime)
	case "author":
		author := repo.LastCommitAuthor
		if len(author) > 20 {
			return author[:17] + "..."
		}
		if author == "" {
			return "-"
		}
		return author
	case "root":
		return repo.Root
	case "path":
		relPath := strings.ReplaceAll(repo.RelPath, "\\", "/")
		// When root is shown separately, show relative path
		// When root is not shown, show combined path (like in short mode)
		if hasRoot {
			return relPath
		}
		// Combine root and relpath for short mode
		if repo.RelPath == "" {
			return repo.Root
		}
		return repo.Root + "/" + relPath
	case "remote":
		remote := repo.RemoteURL
		if len(remote) > 40 {
			return remote[:37] + "..."
		}
		if remote == "" {
			return "-"
		}
		return remote
	case "tags":
		if len(repo.Tags) == 0 {
			return "-"
		}
		return strings.Join(repo.Tags, ",")
	case "description":
		desc := repo.Description
		if desc == "" {
			return "-"
		}
		if descMaxLen > 0 && len(desc) > descMaxLen {
			return desc[:descMaxLen-3] + "..."
		}
		return desc
	default:
		return "-"
	}
}

func formatStatus(repo *index.Repo) string {
	var parts []string

	// Remote status
	if repo.Ahead > 0 && repo.Behind > 0 {
		parts = append(parts, fmt.Sprintf("diverged ↑%d ↓%d", repo.Ahead, repo.Behind))
	} else if repo.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("ahead %d", repo.Ahead))
	} else if repo.Behind > 0 {
		parts = append(parts, fmt.Sprintf("behind %d", repo.Behind))
	} else {
		parts = append(parts, "up-to-date")
	}

	// Local status
	if repo.StatusUnavailable {
		parts = append(parts, "status unknown")
	} else if repo.IsDirty {
		parts = append(parts, "dirty")
	} else if repo.HasUntracked {
		parts = append(parts, "untracked")
	} else {
		parts = append(parts, "clean")
	}

	return strings.Join(parts, ", ")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(diff.Hours()/24/7))
	case diff < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(diff.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(diff.Hours()/24/365))
	}
}

func outputJSON(repos []*index.Repo) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(repos); err != nil {
		exitWithError("Failed to encode JSON: %v", err)
	}
}

func outputYAML(repos []*index.Repo) {
	enc := yaml.NewEncoder(os.Stdout)
	if err := enc.Encode(repos); err != nil {
		exitWithError("Failed to encode YAML: %v", err)
	}
}
