package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
	listFormat string
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

Examples:
  rog list                           # List all repositories
  rog list api                       # Fuzzy search for "api"
  rog list --lang go --tag cli       # Go repos tagged with "cli"
  rog list --dirty                   # Repos with uncommitted changes
  rog list --sort last-commit --limit 10  # 10 most recently committed`,
	Run: runList,
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
	listCmd.Flags().BoolVar(&listLong, "long", false, "Show detailed information")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format: table, json, yaml")
}

func runList(cmd *cobra.Command, args []string) {
	// Load index
	idx, err := index.Load()
	if err != nil {
		exitWithError("Failed to load index: %v", err)
	}

	if idx.Count() == 0 {
		fmt.Println("No repositories found. Run 'rog scan' first.")
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
		fmt.Println("No repositories match the criteria.")
		return
	}

	// Output results
	switch listFormat {
	case "json":
		outputJSON(results)
	case "yaml":
		outputYAML(results)
	default:
		outputTable(results, listLong)
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

func outputTable(repos []*index.Repo, long bool) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if long {
		fmt.Fprintln(w, "NAME\tLANG\tHOST\tBRANCH\tSTATUS\tLAST COMMIT\tAUTHOR\tROOT\tPATH\tREMOTE")
	} else {
		fmt.Fprintln(w, "NAME\tLANG\tHOST\tBRANCH\tSTATUS\tLAST COMMIT\tROOT\tPATH")
	}

	for _, repo := range repos {
		name := repo.Name
		lang := repo.PrimaryLanguage
		if lang == "" {
			lang = "unknown"
		}
		host := repo.Host
		if host == "" {
			host = "-"
		}
		branch := repo.CurrentBranch
		if branch == "" {
			branch = "-"
		}

		status := formatStatus(repo)
		lastCommit := formatTime(repo.LastCommitTime)

		if long {
			author := repo.LastCommitAuthor
			if len(author) > 20 {
				author = author[:17] + "..."
			}
			remote := repo.RemoteURL
			if len(remote) > 40 {
				remote = remote[:37] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				name, lang, host, branch, status, lastCommit, author, repo.Root, repo.RelPath, remote)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				name, lang, host, branch, status, lastCommit, repo.Root, repo.RelPath)
		}
	}

	w.Flush()

	fmt.Printf("\nTotal: %d repositories\n", len(repos))
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
	if repo.IsDirty {
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
